package difflib

// Portions of this file are a close translation of CPython's
// Lib/difflib.py (Copyright (c) 2001 Python Software Foundation; All
// Rights Reserved), used under the Python Software Foundation License
// Version 2. See LICENSE-python.

import "fmt"

// Restore generates one of the two sequences that produced a delta.
//
// Given a delta produced by [Differ.Compare], [Ndiff], or [NdiffFunc],
// Restore extracts the lines originating from file 1 or file 2
// (which, 1 or 2), stripping off the two-character line prefixes.
// which must be 1 or 2, or an error is returned.
func Restore(delta []string, which int) ([]string, error) {
	var tag string
	switch which {
	case 1:
		tag = "- "
	case 2:
		tag = "+ "
	default:
		return nil, fmt.Errorf("difflib: unknown delta choice (must be 1 or 2): %d", which)
	}

	var out []string
	for _, line := range delta {
		if len(line) < 2 {
			continue
		}
		prefix := line[:2]
		if prefix == "  " || prefix == tag {
			out = append(out, line[2:])
		}
	}
	return out, nil
}
