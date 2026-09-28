package difflib

// Portions of this file are a close translation of CPython's
// Lib/difflib.py (Copyright (c) 2001 Python Software Foundation; All
// Rights Reserved), used under the Python Software Foundation License
// Version 2. See LICENSE-python.

import (
	"strings"
	"unicode"
)

// IsLineJunk returns true for an ignorable line: one that is blank, or
// that (once leading/trailing whitespace is stripped) is exactly "#".
//
// It is a heuristic default for [Differ]'s linejunk filter; CPython's own
// doc comment recommends leaving linejunk unset (nil in this port) in
// preference to this, since [SequenceMatcher]'s adaptive "popular
// element" notion of noise does a better job than any static definition
// can.
func IsLineJunk(line string) bool {
	s := strings.TrimSpace(line)
	return s == "" || s == "#"
}

// IsCharacterJunk returns true for an ignorable character: a space or a
// tab. It is meant for [Differ]'s charjunk filter. Do not use it (or any
// function) to treat a newline as junk.
func IsCharacterJunk(ch rune) bool {
	return ch == ' ' || ch == '\t'
}

// isUnicodeSpace reports whether r is whitespace per Python's
// str.isspace(), used to reproduce Differ's "keep the original
// whitespace character in the '?' hint line" behavior exactly.
func isUnicodeSpace(r rune) bool {
	return unicode.IsSpace(r)
}
