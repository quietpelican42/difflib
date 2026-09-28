package difflib

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// This file replays testdata/cases.json — real output captured from
// CPython 3.14's stdlib difflib module by testdata/gen_cases.py — through
// this port, and requires exact agreement. See that script's module
// docstring for how each case group was produced and what it covers.

type smCase struct {
	Kind           string                `json:"kind"` // "chars" or "lines"
	A              json.RawMessage       `json:"a"`
	B              json.RawMessage       `json:"b"`
	IsJunk         string                `json:"isjunk"`
	AutoJunk       bool                  `json:"autojunk"`
	Ratio          float64               `json:"ratio"`
	QuickRatio     float64               `json:"quick_ratio"`
	RealQuickRatio float64               `json:"real_quick_ratio"`
	MatchingBlocks [][3]int              `json:"matching_blocks"`
	OpCodes        [][5]any              `json:"opcodes"`
	GroupedOpCodes map[string][][][5]any `json:"grouped_opcodes"`
	BJunk          []string              `json:"bjunk"`
	BPopular       []string              `json:"bpopular"`
}

type closeCase struct {
	Word          string   `json:"word"`
	Possibilities []string `json:"possibilities"`
	N             int      `json:"n"`
	Cutoff        float64  `json:"cutoff"`
	Result        []string `json:"result"`
	Error         bool     `json:"error"`
}

type linesCase struct {
	A      []string `json:"a"`
	B      []string `json:"b"`
	Result []string `json:"result"`
	Tag    string   `json:"tag"`
}

type restoreCase struct {
	Delta  []string `json:"delta"`
	Which  int      `json:"which"`
	Result []string `json:"result"`
	Error  bool     `json:"error"`
	Tag    string   `json:"tag"`
}

type diffCase struct {
	A        []string `json:"a"`
	B        []string `json:"b"`
	FromFile string   `json:"fromfile"`
	ToFile   string   `json:"tofile"`
	FromDate string   `json:"fromdate"`
	ToDate   string   `json:"todate"`
	N        int      `json:"n"`
	LineTerm string   `json:"lineterm"`
	Result   []string `json:"result"`
	Tag      string   `json:"tag"`
}

type lineJunkCase struct {
	Line   string `json:"line"`
	Result bool   `json:"result"`
}

type charJunkCase struct {
	Ch     string `json:"ch"`
	Result bool   `json:"result"`
}

type allCases struct {
	SequenceMatcher []smCase       `json:"sequence_matcher"`
	CloseMatches    []closeCase    `json:"close_matches"`
	DifferCompare   []linesCase    `json:"differ_compare"`
	Ndiff           []linesCase    `json:"ndiff"`
	Restore         []restoreCase  `json:"restore"`
	UnifiedDiff     []diffCase     `json:"unified_diff"`
	ContextDiff     []diffCase     `json:"context_diff"`
	IsLineJunk      []lineJunkCase `json:"is_line_junk"`
	IsCharacterJunk []charJunkCase `json:"is_character_junk"`
}

func loadCases(t *testing.T) allCases {
	t.Helper()
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatalf("reading testdata/cases.json (run testdata/gen_cases.py with Python 3.14 first): %v", err)
	}
	var c allCases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("decoding testdata/cases.json: %v", err)
	}
	return c
}

var charJunkByName = map[string]func(rune) bool{
	"none":       nil,
	"space":      func(r rune) bool { return r == ' ' },
	"space_or_b": func(r rune) bool { return r == ' ' || r == 'b' },
	"vowel":      func(r rune) bool { return strings.ContainsRune("aeiou", r) },
}

var lineJunkByName = map[string]func(string) bool{
	"none":         nil,
	"is_line_junk": IsLineJunk,
	"blank_only":   func(s string) bool { return strings.TrimSpace(s) == "" },
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func opCodesEqual(t *testing.T, tag string, got []OpCode, want [][5]any) bool {
	t.Helper()
	if len(got) != len(want) {
		return false
	}
	for i, g := range got {
		w := want[i]
		wtag, _ := w[0].(string)
		wi1 := int(w[1].(float64))
		wi2 := int(w[2].(float64))
		wj1 := int(w[3].(float64))
		wj2 := int(w[4].(float64))
		if g.Tag != wtag || g.I1 != wi1 || g.I2 != wi2 || g.J1 != wj1 || g.J2 != wj2 {
			return false
		}
	}
	return true
}

func matchingBlocksEqual(got []Match, want [][3]int) bool {
	if len(got) != len(want) {
		return false
	}
	for i, g := range got {
		if g.A != want[i][0] || g.B != want[i][1] || g.Size != want[i][2] {
			return false
		}
	}
	return true
}

func stringSetEqual(gotSet map[string]struct{}, want []string) bool {
	if len(gotSet) != len(want) {
		return false
	}
	for _, w := range want {
		if _, ok := gotSet[w]; !ok {
			return false
		}
	}
	return true
}

func TestDifferentialSequenceMatcher(t *testing.T) {
	cases := loadCases(t).SequenceMatcher
	if len(cases) == 0 {
		t.Skip("no sequence_matcher cases in testdata/cases.json")
	}
	for idx, c := range cases {
		c := c
		t.Run("", func(t *testing.T) {
			switch c.Kind {
			case "chars":
				var aStr, bStr string
				if err := json.Unmarshal(c.A, &aStr); err != nil {
					t.Fatalf("case %d: decoding a: %v", idx, err)
				}
				if err := json.Unmarshal(c.B, &bStr); err != nil {
					t.Fatalf("case %d: decoding b: %v", idx, err)
				}
				junk, ok := charJunkByName[c.IsJunk]
				if !ok {
					t.Fatalf("case %d: unknown isjunk %q", idx, c.IsJunk)
				}
				aRunes, bRunes := []rune(aStr), []rune(bStr)
				sm := NewSequenceMatcherAutoJunk[rune](junk, aRunes, bRunes, c.AutoJunk)
				checkSM(t, idx, sm, c, func(r rune) string { return string(r) })
			case "lines":
				var aLines, bLines []string
				if err := json.Unmarshal(c.A, &aLines); err != nil {
					t.Fatalf("case %d: decoding a: %v", idx, err)
				}
				if err := json.Unmarshal(c.B, &bLines); err != nil {
					t.Fatalf("case %d: decoding b: %v", idx, err)
				}
				junk, ok := lineJunkByName[c.IsJunk]
				if !ok {
					t.Fatalf("case %d: unknown isjunk %q", idx, c.IsJunk)
				}
				sm := NewSequenceMatcherAutoJunk[string](junk, aLines, bLines, c.AutoJunk)
				checkSM(t, idx, sm, c, func(s string) string { return s })
			default:
				t.Fatalf("case %d: unknown kind %q", idx, c.Kind)
			}
		})
	}
}

// checkSM is generic over the SequenceMatcher's element type so both the
// "chars" ([]rune) and "lines" ([]string) case kinds can share one
// assertion routine. toStr renders an element the same way Python's
// sorted(set) → JSON did (a 1-rune string, or the line itself).
func checkSM[T comparable](t *testing.T, idx int, sm *SequenceMatcher[T], c smCase, toStr func(T) string) {
	t.Helper()

	if r := sm.Ratio(); !almostEqual(r, c.Ratio) {
		t.Errorf("case %d: Ratio() = %v, want %v", idx, r, c.Ratio)
	}
	if r := sm.QuickRatio(); !almostEqual(r, c.QuickRatio) {
		t.Errorf("case %d: QuickRatio() = %v, want %v", idx, r, c.QuickRatio)
	}
	if r := sm.RealQuickRatio(); !almostEqual(r, c.RealQuickRatio) {
		t.Errorf("case %d: RealQuickRatio() = %v, want %v", idx, r, c.RealQuickRatio)
	}
	if got := sm.GetMatchingBlocks(); !matchingBlocksEqual(got, c.MatchingBlocks) {
		t.Errorf("case %d: GetMatchingBlocks() = %v, want %v", idx, got, c.MatchingBlocks)
	}
	if got := sm.GetOpCodes(); !opCodesEqual(t, "opcodes", got, c.OpCodes) {
		t.Errorf("case %d: GetOpCodes() = %+v, want %v", idx, got, c.OpCodes)
	}
	for nStr, wantGroups := range c.GroupedOpCodes {
		n, err := strconv.Atoi(nStr)
		if err != nil {
			t.Fatalf("case %d: bad n key %q", idx, nStr)
		}
		gotGroups := sm.GetGroupedOpCodes(n)
		if len(gotGroups) != len(wantGroups) {
			t.Errorf("case %d n=%d: %d groups, want %d", idx, n, len(gotGroups), len(wantGroups))
			continue
		}
		for gi, g := range gotGroups {
			if !opCodesEqual(t, "grouped", g, wantGroups[gi]) {
				t.Errorf("case %d n=%d group %d: %+v, want %v", idx, n, gi, g, wantGroups[gi])
			}
		}
	}

	gotBJunk := map[string]struct{}{}
	for k := range sm.bJunk {
		gotBJunk[toStr(k)] = struct{}{}
	}
	if !stringSetEqual(gotBJunk, c.BJunk) {
		t.Errorf("case %d: bJunk = %v, want %v", idx, gotBJunk, c.BJunk)
	}
	gotBPopular := map[string]struct{}{}
	for k := range sm.bPopular {
		gotBPopular[toStr(k)] = struct{}{}
	}
	if !stringSetEqual(gotBPopular, c.BPopular) {
		t.Errorf("case %d: bPopular = %v, want %v", idx, gotBPopular, c.BPopular)
	}
}

func TestDifferentialGetCloseMatches(t *testing.T) {
	cases := loadCases(t).CloseMatches
	if len(cases) == 0 {
		t.Skip("no close_matches cases in testdata/cases.json")
	}
	for idx, c := range cases {
		got, err := GetCloseMatchesString(c.Word, c.Possibilities, c.N, c.Cutoff)
		if c.Error {
			if err == nil {
				t.Errorf("case %d: want error, got none (result %v)", idx, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("case %d: unexpected error: %v", idx, err)
			continue
		}
		if !stringSliceEqual(got, c.Result) {
			t.Errorf("case %d: GetCloseMatchesString(%q, %v, %d, %v) = %v, want %v",
				idx, c.Word, c.Possibilities, c.N, c.Cutoff, got, c.Result)
		}
	}
}

func TestDifferentialDifferCompare(t *testing.T) {
	cases := loadCases(t).DifferCompare
	if len(cases) == 0 {
		t.Skip("no differ_compare cases in testdata/cases.json")
	}
	d := NewDiffer(nil, nil)
	for idx, c := range cases {
		got := d.Compare(c.A, c.B)
		if !stringSliceEqual(got, c.Result) {
			t.Errorf("case %d (%s): Differ.Compare(%q, %q) =\n%v\nwant\n%v", idx, c.Tag, c.A, c.B, got, c.Result)
		}
	}
}

func TestDifferentialNdiff(t *testing.T) {
	cases := loadCases(t).Ndiff
	if len(cases) == 0 {
		t.Skip("no ndiff cases in testdata/cases.json")
	}
	for idx, c := range cases {
		got := Ndiff(c.A, c.B)
		if !stringSliceEqual(got, c.Result) {
			t.Errorf("case %d (%s): Ndiff(%q, %q) =\n%v\nwant\n%v", idx, c.Tag, c.A, c.B, got, c.Result)
		}
	}
}

func TestDifferentialRestore(t *testing.T) {
	cases := loadCases(t).Restore
	if len(cases) == 0 {
		t.Skip("no restore cases in testdata/cases.json")
	}
	for idx, c := range cases {
		got, err := Restore(c.Delta, c.Which)
		if c.Error {
			if err == nil {
				t.Errorf("case %d (%s): want error, got none (result %v)", idx, c.Tag, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("case %d (%s): unexpected error: %v", idx, c.Tag, err)
			continue
		}
		if !stringSliceEqual(got, c.Result) {
			t.Errorf("case %d (%s): Restore() = %v, want %v", idx, c.Tag, got, c.Result)
		}
	}
}

func TestDifferentialUnifiedDiff(t *testing.T) {
	cases := loadCases(t).UnifiedDiff
	if len(cases) == 0 {
		t.Skip("no unified_diff cases in testdata/cases.json")
	}
	for idx, c := range cases {
		got := UnifiedDiff(c.A, c.B, c.FromFile, c.ToFile, c.FromDate, c.ToDate, c.N, c.LineTerm)
		if !stringSliceEqual(got, c.Result) {
			t.Errorf("case %d (%s): UnifiedDiff() =\n%v\nwant\n%v", idx, c.Tag, got, c.Result)
		}
	}
}

func TestDifferentialContextDiff(t *testing.T) {
	cases := loadCases(t).ContextDiff
	if len(cases) == 0 {
		t.Skip("no context_diff cases in testdata/cases.json")
	}
	for idx, c := range cases {
		got := ContextDiff(c.A, c.B, c.FromFile, c.ToFile, c.FromDate, c.ToDate, c.N, c.LineTerm)
		if !stringSliceEqual(got, c.Result) {
			t.Errorf("case %d (%s): ContextDiff() =\n%v\nwant\n%v", idx, c.Tag, got, c.Result)
		}
	}
}

func TestDifferentialIsLineJunk(t *testing.T) {
	cases := loadCases(t).IsLineJunk
	if len(cases) == 0 {
		t.Skip("no is_line_junk cases in testdata/cases.json")
	}
	for idx, c := range cases {
		if got := IsLineJunk(c.Line); got != c.Result {
			t.Errorf("case %d: IsLineJunk(%q) = %v, want %v", idx, c.Line, got, c.Result)
		}
	}
}

func TestDifferentialIsCharacterJunk(t *testing.T) {
	cases := loadCases(t).IsCharacterJunk
	if len(cases) == 0 {
		t.Skip("no is_character_junk cases in testdata/cases.json")
	}
	for idx, c := range cases {
		r := []rune(c.Ch)[0]
		if got := IsCharacterJunk(r); got != c.Result {
			t.Errorf("case %d: IsCharacterJunk(%q) = %v, want %v", idx, c.Ch, got, c.Result)
		}
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
