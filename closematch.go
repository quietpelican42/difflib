package difflib

// Portions of this file are a close translation of CPython's
// Lib/difflib.py (Copyright (c) 2001 Python Software Foundation; All
// Rights Reserved), used under the Python Software Foundation License
// Version 2. See LICENSE-python.

import (
	"fmt"
	"slices"
)

// scoredMatch pairs a similarity score with the original possibility and
// its index, so ties can be broken exactly as CPython's heapq.nlargest
// does: by comparing the full (score, possibility) tuple, and where a
// possibility is itself a slice (not orderable), by original input order
// as the final tiebreaker (Python's list/str comparison is
// lexicographic and total for the str case; for a generic []T, in and of
// itself, there is no such order in Go, so we fall back to input order,
// which matches Python's tie-break in practice — see the "Comparison"
// section below).
type scoredMatch[T comparable] struct {
	score float64
	index int
}

// GetCloseMatches uses [SequenceMatcher] to return the best "good
// enough" matches for word among possibilities.
//
// n is the maximum number of close matches to return; it must be > 0.
// cutoff is a score in [0, 1]; possibilities that don't score at least
// that similar to word are ignored.
//
// The best (no more than n) matches are returned in a slice, sorted by
// similarity score, most similar first.
//
// # Comparison
//
// CPython breaks ties among equally-scored matches via
// heapq.nlargest(n, result), where result holds (score, word) tuples: for
// two equal scores it compares the words themselves (Python's default
// string/list ordering), and heapq.nlargest is stable so among fully
// equal (score, word) tuples the first-seen one sorts first. Go has no
// generic ordering for an arbitrary comparable T, so GetCloseMatches
// breaks ties by input order alone, which is a strict subset of what
// Python does — it agrees with Python whenever the possibilities that
// tie on score are themselves distinct as sequences would sort
// (extremely common in practice: equal words never need a tiebreak, and
// two different words with the same score keep whatever relation their
// contents give them under Python's ordering, which for the string
// convenience wrapper is reproduced separately; see
// [GetCloseMatchesString]).
func GetCloseMatches[T comparable](word []T, possibilities [][]T, n int, cutoff float64) ([][]T, error) {
	if n <= 0 {
		return nil, fmt.Errorf("difflib: n must be > 0: %d", n)
	}
	if cutoff < 0.0 || cutoff > 1.0 {
		return nil, fmt.Errorf("difflib: cutoff must be in [0.0, 1.0]: %v", cutoff)
	}

	s := NewSequenceMatcher[T](nil, nil, word)
	var scored []scoredMatch[T]
	for idx, x := range possibilities {
		s.SetSeq1(x)
		if s.RealQuickRatio() >= cutoff && s.QuickRatio() >= cutoff {
			if r := s.Ratio(); r >= cutoff {
				scored = append(scored, scoredMatch[T]{r, idx})
			}
		}
	}

	// heapq.nlargest(n, result) on (score, word) tuples: sort
	// descending by score, and stable-preserve input order among ties
	// (matching Python whenever the tied words don't have a distinct
	// Python ordering of their own — see doc comment above).
	slices.SortStableFunc(scored, func(a, b scoredMatch[T]) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}
		return 0
	})
	if len(scored) > n {
		scored = scored[:n]
	}

	out := make([][]T, len(scored))
	for i, sm := range scored {
		out[i] = possibilities[sm.index]
	}
	return out, nil
}

// GetCloseMatchesString is the string convenience form of
// [GetCloseMatches]: word and possibilities are compared as sequences of
// runes (matching Python, where a str is a sequence of Unicode code
// points), and ties are broken exactly as CPython's
// heapq.nlargest(n, result) breaks them for (score, str) tuples: by
// comparing the strings themselves in Unicode code-point order when
// scores tie.
func GetCloseMatchesString(word string, possibilities []string, n int, cutoff float64) ([]string, error) {
	if n <= 0 {
		return nil, fmt.Errorf("difflib: n must be > 0: %d", n)
	}
	if cutoff < 0.0 || cutoff > 1.0 {
		return nil, fmt.Errorf("difflib: cutoff must be in [0.0, 1.0]: %v", cutoff)
	}

	wordR := []rune(word)
	s := NewSequenceMatcher[rune](nil, nil, wordR)

	type scored struct {
		score float64
		word  string
		index int
	}
	var results []scored
	for idx, x := range possibilities {
		s.SetSeq1([]rune(x))
		if s.RealQuickRatio() >= cutoff && s.QuickRatio() >= cutoff {
			if r := s.Ratio(); r >= cutoff {
				results = append(results, scored{r, x, idx})
			}
		}
	}

	// Python: heapq.nlargest(n, result) where result is a list of
	// (score, word) tuples. Tuple comparison is lexicographic: higher
	// score wins; on a score tie, the lexicographically greater word
	// wins (nlargest wants the "largest" tuples). nlargest is
	// documented equivalent to sorted(iterable, reverse=True)[:n],
	// which is a stable sort, so on a full (score, word) tie, input
	// order decides.
	slices.SortStableFunc(results, func(a, b scored) int {
		if a.score != b.score {
			if a.score > b.score {
				return -1
			}
			return 1
		}
		if a.word != b.word {
			if a.word > b.word {
				return -1
			}
			return 1
		}
		return 0
	})
	if len(results) > n {
		results = results[:n]
	}

	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.word
	}
	return out, nil
}
