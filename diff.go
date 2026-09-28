package difflib

// Portions of this file are a close translation of CPython's
// Lib/difflib.py (Copyright (c) 2001 Python Software Foundation; All
// Rights Reserved), used under the Python Software Foundation License
// Version 2. See LICENSE-python.

import "fmt"

// formatRangeUnified converts a half-open [start, stop) range to the
// unified diff "ed"-style range format described at
// http://www.unix.org/single_unix_specification/.
func formatRangeUnified(start, stop int) string {
	beginning := start + 1 // lines start numbering with one
	length := stop - start
	if length == 1 {
		return fmt.Sprintf("%d", beginning)
	}
	if length == 0 {
		beginning-- // empty ranges begin at line just before the range
	}
	return fmt.Sprintf("%d,%d", beginning, length)
}

// UnifiedDiff compares two sequences of lines and returns the delta as a
// unified diff, with n lines of context (CPython's default is 3).
//
// By default (lineTerm = "\n"), the diff control lines (---, +++, @@)
// end with a trailing newline, so that a diff of file.readlines() output
// is suitable for file.writelines(). For inputs that do not have
// trailing newlines, pass lineTerm = "" so the output is uniformly
// newline-free.
//
// fromFile, toFile, fromFileDate, and toFileDate populate the unidiff
// header; any may be empty. See [UnifiedDiffStrings] for the common case
// with CPython's other defaults (n=3, lineTerm="\n").
func UnifiedDiff(a, b []string, fromFile, toFile, fromFileDate, toFileDate string, n int, lineTerm string) []string {
	var out []string
	started := false
	m := NewSequenceMatcher[string](nil, a, b)
	for _, group := range m.GetGroupedOpCodes(n) {
		if !started {
			started = true
			fromDate, toDate := "", ""
			if fromFileDate != "" {
				fromDate = "\t" + fromFileDate
			}
			if toFileDate != "" {
				toDate = "\t" + toFileDate
			}
			out = append(out, fmt.Sprintf("--- %s%s%s", fromFile, fromDate, lineTerm))
			out = append(out, fmt.Sprintf("+++ %s%s%s", toFile, toDate, lineTerm))
		}

		first, last := group[0], group[len(group)-1]
		file1Range := formatRangeUnified(first.I1, last.I2)
		file2Range := formatRangeUnified(first.J1, last.J2)
		out = append(out, fmt.Sprintf("@@ -%s +%s @@%s", file1Range, file2Range, lineTerm))

		for _, c := range group {
			if c.Tag == OpEqual {
				for _, line := range a[c.I1:c.I2] {
					out = append(out, " "+line)
				}
				continue
			}
			if c.Tag == OpReplace || c.Tag == OpDelete {
				for _, line := range a[c.I1:c.I2] {
					out = append(out, "-"+line)
				}
			}
			if c.Tag == OpReplace || c.Tag == OpInsert {
				for _, line := range b[c.J1:c.J2] {
					out = append(out, "+"+line)
				}
			}
		}
	}
	return out
}

// UnifiedDiffStrings is [UnifiedDiff] with CPython's remaining defaults
// applied (n=3, lineTerm="\n").
func UnifiedDiffStrings(a, b []string, fromFile, toFile string) []string {
	return UnifiedDiff(a, b, fromFile, toFile, "", "", 3, "\n")
}

// formatRangeContext converts a half-open [start, stop) range to the
// context diff "ed"-style range format.
func formatRangeContext(start, stop int) string {
	beginning := start + 1
	length := stop - start
	if length == 0 {
		beginning--
	}
	if length <= 1 {
		return fmt.Sprintf("%d", beginning)
	}
	return fmt.Sprintf("%d,%d", beginning, beginning+length-1)
}

// ContextDiff compares two sequences of lines and returns the delta as a
// context diff, with n lines of context (CPython's default is 3). See
// [UnifiedDiff] for the meaning of lineTerm, fromFile/toFile, and
// fromFileDate/toFileDate; [ContextDiffStrings] gives the common case
// with CPython's other defaults.
func ContextDiff(a, b []string, fromFile, toFile, fromFileDate, toFileDate string, n int, lineTerm string) []string {
	prefix := map[string]string{
		OpInsert:  "+ ",
		OpDelete:  "- ",
		OpReplace: "! ",
		OpEqual:   "  ",
	}

	var out []string
	started := false
	m := NewSequenceMatcher[string](nil, a, b)
	for _, group := range m.GetGroupedOpCodes(n) {
		if !started {
			started = true
			fromDate, toDate := "", ""
			if fromFileDate != "" {
				fromDate = "\t" + fromFileDate
			}
			if toFileDate != "" {
				toDate = "\t" + toFileDate
			}
			out = append(out, fmt.Sprintf("*** %s%s%s", fromFile, fromDate, lineTerm))
			out = append(out, fmt.Sprintf("--- %s%s%s", toFile, toDate, lineTerm))
		}

		first, last := group[0], group[len(group)-1]
		out = append(out, "***************"+lineTerm)

		file1Range := formatRangeContext(first.I1, last.I2)
		out = append(out, fmt.Sprintf("*** %s ****%s", file1Range, lineTerm))

		hasDeleteOrReplace := false
		hasInsertOrReplace := false
		for _, c := range group {
			if c.Tag == OpReplace || c.Tag == OpDelete {
				hasDeleteOrReplace = true
			}
			if c.Tag == OpReplace || c.Tag == OpInsert {
				hasInsertOrReplace = true
			}
		}

		if hasDeleteOrReplace {
			for _, c := range group {
				if c.Tag != OpInsert {
					for _, line := range a[c.I1:c.I2] {
						out = append(out, prefix[c.Tag]+line)
					}
				}
			}
		}

		file2Range := formatRangeContext(first.J1, last.J2)
		out = append(out, fmt.Sprintf("--- %s ----%s", file2Range, lineTerm))

		if hasInsertOrReplace {
			for _, c := range group {
				if c.Tag != OpDelete {
					for _, line := range b[c.J1:c.J2] {
						out = append(out, prefix[c.Tag]+line)
					}
				}
			}
		}
	}
	return out
}

// ContextDiffStrings is [ContextDiff] with CPython's remaining defaults
// applied (n=3, lineTerm="\n").
func ContextDiffStrings(a, b []string, fromFile, toFile string) []string {
	return ContextDiff(a, b, fromFile, toFile, "", "", 3, "\n")
}
