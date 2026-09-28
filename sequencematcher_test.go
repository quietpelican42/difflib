package difflib

import (
	"reflect"
	"testing"
)

// TestDocstringExample reproduces SequenceMatcher's own doc example
// (find_longest_match over " abcd" vs "abcd abcd"), confirming the
// generic implementation matches CPython's documented contract for the
// string case.
func TestDocstringExample(t *testing.T) {
	sm := NewSequenceMatcher[rune](nil, []rune(" abcd"), []rune("abcd abcd"))
	got := sm.FindLongestMatch(0, 5, 0, 9)
	want := Match{A: 0, B: 4, Size: 5}
	if got != want {
		t.Errorf("FindLongestMatch() = %+v, want %+v", got, want)
	}

	sm = NewSequenceMatcher[rune](func(r rune) bool { return r == ' ' }, []rune(" abcd"), []rune("abcd abcd"))
	got = sm.FindLongestMatch(0, 5, 0, 9)
	want = Match{A: 1, B: 0, Size: 4}
	if got != want {
		t.Errorf("FindLongestMatch() with isJunk = %+v, want %+v", got, want)
	}

	sm = NewSequenceMatcher[rune](nil, []rune("ab"), []rune("c"))
	got = sm.FindLongestMatch(0, 2, 0, 1)
	want = Match{A: 0, B: 0, Size: 0}
	if got != want {
		t.Errorf("FindLongestMatch() on disjoint sequences = %+v, want %+v", got, want)
	}
}

// TestGenericIntSequence exercises SequenceMatcher over a non-string,
// non-rune element type, confirming the port is genuinely generic and
// not secretly specialized to strings.
func TestGenericIntSequence(t *testing.T) {
	a := []int{1, 2, 3, 4, 5}
	b := []int{1, 2, 30, 4, 5}
	sm := NewSequenceMatcher[int](nil, a, b)

	wantBlocks := []Match{{0, 0, 2}, {3, 3, 2}, {5, 5, 0}}
	if got := sm.GetMatchingBlocks(); !reflect.DeepEqual(got, wantBlocks) {
		t.Errorf("GetMatchingBlocks() = %v, want %v", got, wantBlocks)
	}

	wantOps := []OpCode{
		{OpEqual, 0, 2, 0, 2},
		{OpReplace, 2, 3, 2, 3},
		{OpEqual, 3, 5, 3, 5},
	}
	if got := sm.GetOpCodes(); !reflect.DeepEqual(got, wantOps) {
		t.Errorf("GetOpCodes() = %v, want %v", got, wantOps)
	}

	if r := sm.Ratio(); r != 0.8 {
		t.Errorf("Ratio() = %v, want 0.8", r)
	}
}

// TestSetSeq1Reuse exercises the documented "compare one sequence
// against many" reuse pattern (SetSeq2 once, SetSeq1 repeatedly), which
// get_close_matches itself relies on.
func TestSetSeq1Reuse(t *testing.T) {
	sm := NewSequenceMatcher[rune](nil, []rune("abcd"), nil)
	sm.SetSeq2([]rune("bcde"))
	if r := sm.Ratio(); r != 0.75 {
		t.Errorf("Ratio() = %v, want 0.75", r)
	}
	sm.SetSeq1([]rune("bcde"))
	if r := sm.Ratio(); r != 1.0 {
		t.Errorf("after SetSeq1 to identical sequence, Ratio() = %v, want 1.0", r)
	}
}

// TestNullSequences guards the SF-bug-763023 edge case: ratio of two
// empty sequences is 1, not a division-by-zero panic.
func TestNullSequences(t *testing.T) {
	sm := NewSequenceMatcher[rune](nil, nil, nil)
	if r := sm.Ratio(); r != 1 {
		t.Errorf("Ratio() on two empty sequences = %v, want 1", r)
	}
	if r := sm.QuickRatio(); r != 1 {
		t.Errorf("QuickRatio() on two empty sequences = %v, want 1", r)
	}
	if r := sm.RealQuickRatio(); r != 1 {
		t.Errorf("RealQuickRatio() on two empty sequences = %v, want 1", r)
	}
}

// TestGetCloseMatchesGenericErrors covers the documented ValueError
// (here, error) cases for n and cutoff, on the generic entry point (the
// differential suite covers GetCloseMatchesString's error cases against
// real CPython output; this exercises the same validation on the
// non-string generic API).
func TestGetCloseMatchesGenericErrors(t *testing.T) {
	possibilities := [][]rune{[]rune("egg")}
	word := []rune("spam")

	if _, err := GetCloseMatches(word, possibilities, 0, 0.6); err == nil {
		t.Error("n=0: want error, got nil")
	}
	if _, err := GetCloseMatches(word, possibilities, -1, 0.6); err == nil {
		t.Error("n=-1: want error, got nil")
	}
	if _, err := GetCloseMatches(word, possibilities, 3, 1.1); err == nil {
		t.Error("cutoff=1.1: want error, got nil")
	}
	if _, err := GetCloseMatches(word, possibilities, 3, -0.1); err == nil {
		t.Error("cutoff=-0.1: want error, got nil")
	}
}

// TestRestoreInvalidWhich covers restore()'s documented ValueError for
// an unknown delta choice.
func TestRestoreInvalidWhich(t *testing.T) {
	if _, err := Restore(nil, 0); err == nil {
		t.Error("which=0: want error, got nil")
	}
	if _, err := Restore(nil, 3); err == nil {
		t.Error("which=3: want error, got nil")
	}
}
