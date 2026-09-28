package difflib

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// randomLines builds n pseudo-random "line" strings over a small alphabet
// of words, then returns a and b where b is a with roughly changeFrac of
// lines replaced — a rough stand-in for diffing two versions of a real
// text file.
func randomLines(n int, changeFrac float64, seed int64) (a, b []string) {
	r := rand.New(rand.NewSource(seed))
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog", "func", "return", "if", "err"}
	line := func() string {
		var sb strings.Builder
		for i := 0; i < 6; i++ {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(words[r.Intn(len(words))])
		}
		sb.WriteByte('\n')
		return sb.String()
	}
	a = make([]string, n)
	for i := range a {
		a[i] = line()
	}
	b = make([]string, len(a))
	copy(b, a)
	for i := range b {
		if r.Float64() < changeFrac {
			b[i] = line()
		}
	}
	return a, b
}

func BenchmarkRatio1000Lines5PctChanged(b *testing.B) {
	aLines, bLines := randomLines(1000, 0.05, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm := NewSequenceMatcher[string](nil, aLines, bLines)
		_ = sm.Ratio()
	}
}

func BenchmarkGetOpCodes1000Lines5PctChanged(b *testing.B) {
	aLines, bLines := randomLines(1000, 0.05, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm := NewSequenceMatcher[string](nil, aLines, bLines)
		_ = sm.GetOpCodes()
	}
}

func BenchmarkUnifiedDiff1000Lines5PctChanged(b *testing.B) {
	aLines, bLines := randomLines(1000, 0.05, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = UnifiedDiffStrings(aLines, bLines, "a.txt", "b.txt")
	}
}

// TestPrintUnifiedDiffSample is not a real assertion; it exists so `go
// test -run TestPrintUnifiedDiffSample -v` can be used to eyeball a
// sample diff during development. It's cheap enough to leave in.
func TestPrintUnifiedDiffSample(t *testing.T) {
	a := []string{"one\n", "two\n", "three\n", "four\n"}
	b := []string{"one\n", "tao\n", "three\n", "four\n", "five\n"}
	out := UnifiedDiffStrings(a, b, "a.txt", "b.txt")
	if len(out) == 0 {
		t.Fatal("expected non-empty diff")
	}
	fmt.Print(strings.Join(out, ""))
}
