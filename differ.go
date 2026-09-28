package difflib

// Portions of this file are a close translation of CPython's
// Lib/difflib.py (Copyright (c) 2001 Python Software Foundation; All
// Rights Reserved), used under the Python Software Foundation License
// Version 2. See LICENSE-python.

import (
	"strings"
)

// Differ compares sequences of lines of text and produces human-readable
// differences or deltas.
//
// Each line of a Differ delta begins with a two-character code:
//
//	"- "   line unique to sequence 1
//	"+ "   line unique to sequence 2
//	"  "   line common to both sequences
//	"? "   line not present in either input sequence
//
// Lines beginning with "? " attempt to guide the eye to intraline
// differences, and were not present in either input sequence. These
// lines can be confusing if the sequences contain tab characters.
//
// Differ makes no claim to produce a minimal diff. To the contrary,
// minimal diffs are often counter-intuitive, because they synch up
// anywhere possible, sometimes on accidental matches far apart.
// Restricting synch points to contiguous matches preserves some notion
// of locality, at the occasional cost of producing a longer diff.
//
// The zero value has both filters nil, matching Differ() in Python.
type Differ struct {
	// LineJunk, if non-nil, reports whether a line should be treated as
	// junk for the purpose of synchronizing sequences. [IsLineJunk] may
	// be used, though CPython recommends leaving this nil; see its doc
	// comment.
	LineJunk func(line string) bool

	// CharJunk, if non-nil, reports whether a single character (rune)
	// should be treated as junk when marking intraline differences.
	// [IsCharacterJunk] filters out spaces and tabs; never treat a
	// newline as junk.
	CharJunk func(ch rune) bool
}

// NewDiffer constructs a Differ with the given junk filters. Either may
// be nil.
func NewDiffer(lineJunk func(string) bool, charJunk func(rune) bool) *Differ {
	return &Differ{LineJunk: lineJunk, CharJunk: charJunk}
}

// Compare compares two sequences of lines and returns the resulting
// delta.
//
// Each input slice must contain individual single-line strings ending
// with newlines (such sequences can be obtained by splitting on "\n" and
// keeping the separators). The delta generated also consists of
// newline-terminated strings.
func (d *Differ) Compare(a, b []string) []string {
	cruncher := NewSequenceMatcher[string](d.LineJunk, a, b)
	var out []string
	for _, oc := range cruncher.GetOpCodes() {
		switch oc.Tag {
		case OpReplace:
			out = append(out, d.fancyReplace(a, oc.I1, oc.I2, b, oc.J1, oc.J2)...)
		case OpDelete:
			out = append(out, dumpTagged("-", a, oc.I1, oc.I2)...)
		case OpInsert:
			out = append(out, dumpTagged("+", b, oc.J1, oc.J2)...)
		case OpEqual:
			out = append(out, dumpTagged(" ", a, oc.I1, oc.I2)...)
		}
	}
	return out
}

// dumpTagged generates "<tag> <line>" for each line in x[lo:hi].
func dumpTagged(tag string, x []string, lo, hi int) []string {
	out := make([]string, 0, hi-lo)
	for i := lo; i < hi; i++ {
		out = append(out, tag+" "+x[i])
	}
	return out
}

// plainReplace dumps the shorter block first, reducing the burden on
// short-term memory if the blocks are of very different sizes.
func plainReplace(a []string, alo, ahi int, b []string, blo, bhi int) []string {
	var out []string
	if bhi-blo < ahi-alo {
		out = append(out, dumpTagged("+", b, blo, bhi)...)
		out = append(out, dumpTagged("-", a, alo, ahi)...)
	} else {
		out = append(out, dumpTagged("-", a, alo, ahi)...)
		out = append(out, dumpTagged("+", b, blo, bhi)...)
	}
	return out
}

// fancyHelper dispatches to plainReplace, or a single-sided dump, for a
// (possibly empty on either side) replace range.
func fancyHelper(a []string, alo, ahi int, b []string, blo, bhi int) []string {
	switch {
	case alo < ahi && blo < bhi:
		return plainReplace(a, alo, ahi, b, blo, bhi)
	case alo < ahi:
		return dumpTagged("-", a, alo, ahi)
	case blo < bhi:
		return dumpTagged("+", b, blo, bhi)
	}
	return nil
}

// fancyReplaceWindow is the +/- search radius fancyReplace uses when
// looking for a line pair to synchronize on, matching CPython's WINDOW.
const fancyReplaceWindow = 10

// fancyReplaceCutoff is the minimum per-line similarity ratio required
// before two lines are used as an intraline synch point, matching
// CPython's cutoff (chosen so that a bare majority of matching
// characters is not enough).
const fancyReplaceCutoff = 0.74999

// fancyReplace searches the blocks a[alo:ahi] and b[blo:bhi] for
// similar lines; the best-matching pair (if any) is used as a synch
// point, and intraline difference marking is done on the similar pair.
func (d *Differ) fancyReplace(a []string, alo, ahi int, b []string, blo, bhi int) []string {
	cruncher := NewSequenceMatcher[rune](d.CharJunk, nil, nil)

	var out []string
	dumpI, dumpJ := alo, blo // smallest indices not yet resolved

	for j := blo; j < bhi; j++ {
		cruncher.SetSeq2([]rune(b[j]))
		aequiv := alo + (j - blo)
		aloRange := maxInt(aequiv-fancyReplaceWindow, dumpI)
		ahiRange := minInt(aequiv+fancyReplaceWindow+1, ahi)
		if aloRange >= ahiRange {
			// likely exit if a is shorter than b
			break
		}
		bestRatio := fancyReplaceCutoff
		bestI, bestJ := -1, -1
		for i := aloRange; i < ahiRange; i++ {
			cruncher.SetSeq1([]rune(a[i]))
			// Cheapest to most expensive ratio check, for an early out.
			if cruncher.RealQuickRatio() > bestRatio &&
				cruncher.QuickRatio() > bestRatio &&
				cruncher.Ratio() > bestRatio {
				bestI, bestJ, bestRatio = i, j, cruncher.Ratio()
			}
		}

		if bestI == -1 {
			// found nothing to synch on yet - move to next j
			continue
		}

		// pump out straight replace from before this synch pair
		out = append(out, fancyHelper(a, dumpI, bestI, b, dumpJ, bestJ)...)

		// do intraline marking on the synch pair
		aelt, belt := a[bestI], b[bestJ]
		if aelt != belt {
			var atags, btags strings.Builder
			cruncher.SetSeqs([]rune(aelt), []rune(belt))
			for _, oc := range cruncher.GetOpCodes() {
				la, lb := oc.I2-oc.I1, oc.J2-oc.J1
				switch oc.Tag {
				case OpReplace:
					atags.WriteString(strings.Repeat("^", la))
					btags.WriteString(strings.Repeat("^", lb))
				case OpDelete:
					atags.WriteString(strings.Repeat("-", la))
				case OpInsert:
					btags.WriteString(strings.Repeat("+", lb))
				case OpEqual:
					atags.WriteString(strings.Repeat(" ", la))
					btags.WriteString(strings.Repeat(" ", lb))
				}
			}
			out = append(out, qformat(aelt, belt, atags.String(), btags.String())...)
		} else {
			out = append(out, "  "+aelt)
		}
		dumpI, dumpJ = bestI+1, bestJ+1
	}

	// pump out straight replace from after the last synch pair
	out = append(out, fancyHelper(a, dumpI, ahi, b, dumpJ, bhi)...)
	return out
}

// keepOriginalWS replaces whitespace markers in tagS with the original
// whitespace character from s at the same rune position, so that e.g. a
// trailing "? ...  \n" hint line reproduces the real newline rather than
// a literal space marker.
func keepOriginalWS(s, tagS string) string {
	sr := []rune(s)
	tr := []rune(tagS)
	n := len(sr)
	if len(tr) < n {
		n = len(tr)
	}
	out := make([]rune, n)
	for i := 0; i < n; i++ {
		if tr[i] == ' ' && isUnicodeSpace(sr[i]) {
			out[i] = sr[i]
		} else {
			out[i] = tr[i]
		}
	}
	return string(out)
}

// qformat formats the "?" hint output for a synched, differing line
// pair, dealing with tabs and other whitespace by keeping the original
// character in the hint line wherever the tag is a plain space.
func qformat(aline, bline, atags, btags string) []string {
	atags = strings.TrimRightFunc(keepOriginalWS(aline, atags), isUnicodeSpace)
	btags = strings.TrimRightFunc(keepOriginalWS(bline, btags), isUnicodeSpace)

	var out []string
	out = append(out, "- "+aline)
	if atags != "" {
		out = append(out, "? "+atags+"\n")
	}
	out = append(out, "+ "+bline)
	if btags != "" {
		out = append(out, "? "+btags+"\n")
	}
	return out
}

// Ndiff compares a and b (slices of lines) and returns a Differ-style
// delta, using [IsCharacterJunk] as the character-junk filter and no
// line-junk filter — matching ndiff(a, b) in Python, whose default
// charjunk is IS_CHARACTER_JUNK.
func Ndiff(a, b []string) []string {
	return NdiffFunc(a, b, nil, IsCharacterJunk)
}

// NdiffFunc is [Ndiff] with explicit junk filters, matching
// ndiff(a, b, linejunk, charjunk) in Python. Either filter may be nil.
func NdiffFunc(a, b []string, lineJunk func(string) bool, charJunk func(rune) bool) []string {
	return NewDiffer(lineJunk, charJunk).Compare(a, b)
}
