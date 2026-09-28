// Package difflib is a Go port of Python's difflib module: helpers for
// computing deltas between sequences.
//
// [SequenceMatcher] finds the longest contiguous matching subsequence
// between two sequences of any comparable element type, recursively, the
// same "gestalt pattern matching" algorithm CPython uses. Built on it:
// [GetCloseMatches] (fuzzy string matching), [Differ] and [Ndiff]
// (human-readable line deltas with intraline highlighting), [UnifiedDiff]
// and [ContextDiff] (patch-style output), and [Restore] (recovering an
// input from an ndiff delta).
//
// The algorithms and API shape are translated closely from CPython 3.14's
// Lib/difflib.py to reproduce its exact output, including the "autojunk"
// popular-element heuristic and the tie-breaking rules of
// get_close_matches.
//
// Portions of this package are a close translation of CPython's
// Lib/difflib.py (Copyright (c) 2001 Python Software Foundation; All
// Rights Reserved), used under the Python Software Foundation License
// Version 2. See LICENSE-python for the full license text and
// README.md's "License" section for which files this applies to.
package difflib

// Match describes a matching block: a[A:A+Size] == b[B:B+Size].
type Match struct {
	A, B, Size int
}

// OpCode describes one step of turning sequence a into sequence b. Tag is
// one of "replace", "delete", "insert", or "equal":
//
//   - "replace": a[I1:I2] should be replaced by b[J1:J2]
//   - "delete": a[I1:I2] should be deleted (J1 == J2)
//   - "insert": b[J1:J2] should be inserted at a[I1:I1] (I1 == I2)
//   - "equal": a[I1:I2] == b[J1:J2]
type OpCode struct {
	Tag            string
	I1, I2, J1, J2 int
}

const (
	// OpReplace tags a range that should be replaced.
	OpReplace = "replace"
	// OpDelete tags a range that should be deleted.
	OpDelete = "delete"
	// OpInsert tags a range that should be inserted.
	OpInsert = "insert"
	// OpEqual tags a range that is unchanged.
	OpEqual = "equal"
)

func calculateRatio(matches, length int) float64 {
	if length != 0 {
		return 2.0 * float64(matches) / float64(length)
	}
	return 1.0
}

// SequenceMatcher is a flexible type for comparing pairs of sequences of
// any comparable element type. The basic algorithm predates, and is a
// little fancier than, an algorithm published in the late 1980s by
// Ratcliff and Obershelp under the name "gestalt pattern matching". The
// basic idea is to find the longest contiguous matching subsequence that
// contains no "junk" elements. The same idea is then applied recursively
// to the pieces of the sequences to the left and to the right of the
// matching subsequence. This does not yield minimal edit sequences, but
// does tend to yield matches that "look right" to people.
//
// Unlike e.g. Unix diff, the fundamental notion is the longest
// contiguous & junk-free matching subsequence. That's what catches
// people's eyes. This method appears to be the least vulnerable to
// syncing up on blocks of "junk" elements (like blank lines in ordinary
// text files), because it has a concept of "junk".
//
// The zero value is not ready to use; construct with [NewSequenceMatcher]
// or [NewSequenceMatcherAutoJunk].
type SequenceMatcher[T comparable] struct {
	a, b []T

	isJunk   func(T) bool
	autoJunk bool

	b2j      map[T][]int
	bJunk    map[T]struct{}
	bPopular map[T]struct{}

	matchingBlocks []Match
	haveMatching   bool
	opcodes        []OpCode
	haveOpcodes    bool

	fullBCount    map[T]int
	haveFullBCoun bool
}

// NewSequenceMatcher constructs a SequenceMatcher comparing a against b,
// with the "automatic junk heuristic" (autojunk) enabled, matching
// SequenceMatcher(isjunk, a, b) in Python (autojunk defaults to true
// there).
//
// isJunk may be nil, meaning no elements are considered junk. Otherwise
// it is a one-argument function that returns true iff the element is
// junk — e.g. func(r rune) bool { return r == ' ' } if comparing runes
// and blanks should not be synced on.
func NewSequenceMatcher[T comparable](isJunk func(T) bool, a, b []T) *SequenceMatcher[T] {
	return NewSequenceMatcherAutoJunk(isJunk, a, b, true)
}

// NewSequenceMatcherAutoJunk constructs a SequenceMatcher like
// [NewSequenceMatcher], but lets the caller disable the "automatic junk
// heuristic" that treats popular elements as junk (autoJunk=false),
// matching SequenceMatcher(isjunk, a, b, autojunk=False) in Python.
func NewSequenceMatcherAutoJunk[T comparable](isJunk func(T) bool, a, b []T, autoJunk bool) *SequenceMatcher[T] {
	m := &SequenceMatcher[T]{isJunk: isJunk, autoJunk: autoJunk}
	m.SetSeqs(a, b)
	return m
}

// SetSeqs sets the two sequences to be compared.
func (m *SequenceMatcher[T]) SetSeqs(a, b []T) {
	m.SetSeq1(a)
	m.SetSeq2(b)
}

// SetSeq1 sets the first sequence to be compared. The second sequence to
// be compared is not changed.
//
// SequenceMatcher computes and caches detailed information about the
// second sequence, so if you want to compare one sequence against many
// sequences, use SetSeq2 once and call SetSeq1 repeatedly for each of the
// other sequences.
func (m *SequenceMatcher[T]) SetSeq1(a []T) {
	m.a = a
	m.matchingBlocks = nil
	m.haveMatching = false
	m.opcodes = nil
	m.haveOpcodes = false
}

// SetSeq2 sets the second sequence to be compared. The first sequence to
// be compared is not changed.
func (m *SequenceMatcher[T]) SetSeq2(b []T) {
	m.b = b
	m.matchingBlocks = nil
	m.haveMatching = false
	m.opcodes = nil
	m.haveOpcodes = false
	m.fullBCount = nil
	m.haveFullBCoun = false
	m.chainB()
}

// chainB builds b2j: for x in b, b2j[x] is the list of indices (into b)
// at which x appears, in increasing order — except junk and "popular"
// elements, which are left out entirely. See the module doc comment for
// why: this stops find_longest_match from ever starting a matching block
// at a junk or popular element, and dropping popular elements gives an
// enormous speedup on inputs with many repeats (e.g. diffing program
// files with hundreds of blank lines).
//
// "Popular" means: elements accounting for more than 1/(n//100 + 1) of
// b's length, i.e. roughly more than 1% of b, once len(b) >= 200. This
// mirrors CPython's autojunk heuristic exactly, including its off-by-one
// (ntest = n//100 + 1, popular iff count > ntest).
func (m *SequenceMatcher[T]) chainB() {
	b := m.b
	b2j := make(map[T][]int)
	for i, elt := range b {
		b2j[elt] = append(b2j[elt], i)
	}

	bJunk := make(map[T]struct{})
	if m.isJunk != nil {
		for elt := range b2j {
			if m.isJunk(elt) {
				bJunk[elt] = struct{}{}
			}
		}
		for elt := range bJunk {
			delete(b2j, elt)
		}
	}

	bPopular := make(map[T]struct{})
	n := len(b)
	if m.autoJunk && n >= 200 {
		ntest := n/100 + 1
		for elt, idxs := range b2j {
			if len(idxs) > ntest {
				bPopular[elt] = struct{}{}
			}
		}
		for elt := range bPopular {
			delete(b2j, elt)
		}
	}

	m.b2j = b2j
	m.bJunk = bJunk
	m.bPopular = bPopular
}

// FindLongestMatch finds the longest matching block in a[alo:ahi] and
// b[blo:bhi].
//
// If isJunk is nil: returns (i, j, k) such that a[i:i+k] == b[j:j+k],
// where alo <= i <= i+k <= ahi and blo <= j <= j+k <= bhi, and for all
// (i', j', k') meeting those conditions, k >= k', i <= i', and if i ==
// i', j <= j'. In other words, of all maximal matching blocks, this
// returns one that starts earliest in a, and of all those, one that
// starts earliest in b.
//
// If isJunk is not nil, the longest matching block is first determined
// as above, but with the additional restriction that no junk element
// appears in the block. That block is then extended as far as possible
// by matching (only) junk elements on both sides — so the block never
// matches on junk except where identical junk happens to be adjacent to
// an "interesting" match.
//
// If no blocks match, returns (alo, blo, 0).
func (m *SequenceMatcher[T]) FindLongestMatch(alo, ahi, blo, bhi int) Match {
	a, b := m.a, m.b
	besti, bestj, bestsize := alo, blo, 0

	// j2len[j] = length of the longest junk-free match ending with
	// a[i-1] and b[j], as of the current iteration of i.
	j2len := make(map[int]int)
	for i := alo; i < ahi; i++ {
		newj2len := make(map[int]int)
		for _, j := range m.b2j[a[i]] {
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			k := j2len[j-1] + 1
			newj2len[j] = k
			if k > bestsize {
				besti, bestj, bestsize = i-k+1, j-k+1, k
			}
		}
		j2len = newj2len
	}

	junkContains := func(x T) bool {
		_, ok := m.bJunk[x]
		return ok
	}

	// Extend the best match by non-junk elements on each end.
	for besti > alo && bestj > blo &&
		!junkContains(b[bestj-1]) &&
		a[besti-1] == b[bestj-1] {
		besti, bestj, bestsize = besti-1, bestj-1, bestsize+1
	}
	for besti+bestsize < ahi && bestj+bestsize < bhi &&
		!junkContains(b[bestj+bestsize]) &&
		a[besti+bestsize] == b[bestj+bestsize] {
		bestsize++
	}

	// Now suck up matching junk on each side too.
	for besti > alo && bestj > blo &&
		junkContains(b[bestj-1]) &&
		a[besti-1] == b[bestj-1] {
		besti, bestj, bestsize = besti-1, bestj-1, bestsize+1
	}
	for besti+bestsize < ahi && bestj+bestsize < bhi &&
		junkContains(b[bestj+bestsize]) &&
		a[besti+bestsize] == b[bestj+bestsize] {
		bestsize++
	}

	return Match{besti, bestj, bestsize}
}

// GetMatchingBlocks returns the list of triples describing matching
// subsequences.
//
// Each triple is of the form (i, j, n) meaning a[i:i+n] == b[j:j+n]. The
// triples are monotonically increasing in i and in j. Adjacent triples
// never describe adjacent equal blocks (if they did, the algorithm would
// have merged them into one bigger block).
//
// The last triple is a dummy, (len(a), len(b), 0), and is the only
// triple with n == 0.
func (m *SequenceMatcher[T]) GetMatchingBlocks() []Match {
	if m.haveMatching {
		return m.matchingBlocks
	}
	la, lb := len(m.a), len(m.b)

	type region struct{ alo, ahi, blo, bhi int }
	queue := []region{{0, la, 0, lb}}
	var matchingBlocks []Match
	for len(queue) > 0 {
		r := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x := m.FindLongestMatch(r.alo, r.ahi, r.blo, r.bhi)
		i, j, k := x.A, x.B, x.Size
		if k != 0 {
			matchingBlocks = append(matchingBlocks, x)
			if r.alo < i && r.blo < j {
				queue = append(queue, region{r.alo, i, r.blo, j})
			}
			if i+k < r.ahi && j+k < r.bhi {
				queue = append(queue, region{i + k, r.ahi, j + k, r.bhi})
			}
		}
	}
	sortMatches(matchingBlocks)

	// Collapse adjacent equal blocks.
	var nonAdjacent []Match
	var i1, j1, k1 int
	for _, blk := range matchingBlocks {
		i2, j2, k2 := blk.A, blk.B, blk.Size
		if i1+k1 == i2 && j1+k1 == j2 {
			k1 += k2
		} else {
			if k1 != 0 {
				nonAdjacent = append(nonAdjacent, Match{i1, j1, k1})
			}
			i1, j1, k1 = i2, j2, k2
		}
	}
	if k1 != 0 {
		nonAdjacent = append(nonAdjacent, Match{i1, j1, k1})
	}
	nonAdjacent = append(nonAdjacent, Match{la, lb, 0})

	m.matchingBlocks = nonAdjacent
	m.haveMatching = true
	return m.matchingBlocks
}

func sortMatches(ms []Match) {
	// Sorts by (A, B, Size) ascending, matching Python's tuple sort of
	// (i, j, k) triples. Insertion sort is fine: blocks list is small
	// relative to input in the common case, and this keeps the file
	// dependency-free; swap for sort.Slice if profiling ever demands it.
	for i := 1; i < len(ms); i++ {
		v := ms[i]
		j := i - 1
		for j >= 0 && matchLess(v, ms[j]) {
			ms[j+1] = ms[j]
			j--
		}
		ms[j+1] = v
	}
}

func matchLess(a, b Match) bool {
	if a.A != b.A {
		return a.A < b.A
	}
	if a.B != b.B {
		return a.B < b.B
	}
	return a.Size < b.Size
}

// GetOpCodes returns the list of opcodes describing how to turn a into
// b. The first opcode has I1 == J1 == 0, and remaining opcodes have I1
// == the I2 from the opcode preceding it, and likewise for J1 == the
// previous J2.
func (m *SequenceMatcher[T]) GetOpCodes() []OpCode {
	if m.haveOpcodes {
		return m.opcodes
	}
	i, j := 0, 0
	var answer []OpCode
	for _, blk := range m.GetMatchingBlocks() {
		ai, bj, size := blk.A, blk.B, blk.Size
		tag := ""
		switch {
		case i < ai && j < bj:
			tag = OpReplace
		case i < ai:
			tag = OpDelete
		case j < bj:
			tag = OpInsert
		}
		if tag != "" {
			answer = append(answer, OpCode{tag, i, ai, j, bj})
		}
		i, j = ai+size, bj+size
		if size != 0 {
			answer = append(answer, OpCode{OpEqual, ai, i, bj, j})
		}
	}
	m.opcodes = answer
	m.haveOpcodes = true
	return m.opcodes
}

// GetGroupedOpCodes isolates change clusters by eliminating ranges with
// no changes, returning groups with up to n lines of context on either
// side of each change. Each group is in the same format as
// [SequenceMatcher.GetOpCodes].
func (m *SequenceMatcher[T]) GetGroupedOpCodes(n int) [][]OpCode {
	codes := m.GetOpCodes()
	if len(codes) == 0 {
		codes = []OpCode{{OpEqual, 0, 1, 0, 1}}
	} else {
		// copy: we mutate the first/last entries below.
		cp := make([]OpCode, len(codes))
		copy(cp, codes)
		codes = cp
	}

	if codes[0].Tag == OpEqual {
		c := codes[0]
		codes[0] = OpCode{c.Tag, maxInt(c.I1, c.I2-n), c.I2, maxInt(c.J1, c.J2-n), c.J2}
	}
	if codes[len(codes)-1].Tag == OpEqual {
		c := codes[len(codes)-1]
		codes[len(codes)-1] = OpCode{c.Tag, c.I1, minInt(c.I2, c.I1+n), c.J1, minInt(c.J2, c.J1+n)}
	}

	nn := n + n
	var groups [][]OpCode
	var group []OpCode
	for _, c := range codes {
		tag, i1, i2, j1, j2 := c.Tag, c.I1, c.I2, c.J1, c.J2
		if tag == OpEqual && i2-i1 > nn {
			group = append(group, OpCode{tag, i1, minInt(i2, i1+n), j1, minInt(j2, j1+n)})
			groups = append(groups, group)
			group = nil
			i1, j1 = maxInt(i1, i2-n), maxInt(j1, j2-n)
		}
		group = append(group, OpCode{tag, i1, i2, j1, j2})
	}
	if len(group) > 0 && !(len(group) == 1 && group[0].Tag == OpEqual) {
		groups = append(groups, group)
	}
	return groups
}

// Ratio returns a measure of the sequences' similarity, a float in
// [0,1]. Where T is the total number of elements in both sequences and
// M is the number of matches, this is 2.0*M/T. This is 1 if the
// sequences are identical, and 0 if they have nothing in common.
//
// Ratio is expensive to compute if [SequenceMatcher.GetMatchingBlocks]
// or [SequenceMatcher.GetOpCodes] hasn't already been called, in which
// case [SequenceMatcher.QuickRatio] or
// [SequenceMatcher.RealQuickRatio] may be worth trying first, to get a
// cheaper upper bound.
func (m *SequenceMatcher[T]) Ratio() float64 {
	matches := 0
	for _, blk := range m.GetMatchingBlocks() {
		matches += blk.Size
	}
	return calculateRatio(matches, len(m.a)+len(m.b))
}

// QuickRatio returns an upper bound on Ratio, computed more quickly by
// viewing a and b as multisets and counting matches without regard to
// order.
func (m *SequenceMatcher[T]) QuickRatio() float64 {
	if !m.haveFullBCoun {
		fullBCount := make(map[T]int)
		for _, elt := range m.b {
			fullBCount[elt]++
		}
		m.fullBCount = fullBCount
		m.haveFullBCoun = true
	}
	avail := make(map[T]int)
	matches := 0
	for _, elt := range m.a {
		var numb int
		if v, ok := avail[elt]; ok {
			numb = v
		} else {
			numb = m.fullBCount[elt]
		}
		avail[elt] = numb - 1
		if numb > 0 {
			matches++
		}
	}
	return calculateRatio(matches, len(m.a)+len(m.b))
}

// RealQuickRatio returns an upper bound on Ratio, computed very quickly:
// there can't be more matches than the number of elements in the
// shorter sequence.
func (m *SequenceMatcher[T]) RealQuickRatio() float64 {
	la, lb := len(m.a), len(m.b)
	return calculateRatio(minInt(la, lb), la+lb)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
