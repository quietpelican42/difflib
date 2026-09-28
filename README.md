# difflib

A Go port of Python's `difflib` module: the Ratcliff-Obershelp sequence
matcher, `get_close_matches`, `Differ`/`ndiff` (including the `?` intraline
hint lines), `restore`, and `unified_diff`/`context_diff`, verified case by
case against CPython 3.14's own output.

```go
sm := difflib.NewSequenceMatcher[rune](nil, []rune("private Thread t;"), []rune("private volatile Thread t;"))
fmt.Println(sm.Ratio()) // 0.7906976744186046

matches, _ := difflib.GetCloseMatchesString("appel", []string{"ape", "apple", "peach", "puppy"}, 3, 0.6)
fmt.Println(matches) // [apple ape]
```

## Why

Go has one well-known `difflib` port, [pmezard/go-difflib](https://github.com/pmezard/go-difflib)
(430 stars), forked as [nao1215/diff](https://github.com/nao1215/diff). Both
describe themselves as a partial port and cover only `SequenceMatcher`,
`unified_diff`, and `context_diff` — there is no `get_close_matches`, `Differ`,
`ndiff`, or `restore`, and go-difflib has shipped without a `go.mod` since
2018.

Measured: `curl -s https://raw.githubusercontent.com/pmezard/go-difflib/master/difflib/difflib.go | grep -c '^func '`
→ `24` top-level functions/methods, `grep -c -E 'GetCloseMatches|type Differ|func Ndiff|func Restore'`
→ `0`; `curl -o /dev/null -w '%{http_code}' https://raw.githubusercontent.com/pmezard/go-difflib/master/go.mod`
→ `404`.

Hand-rolled ports of the missing piece exist but don't reproduce CPython's
algorithm. [mahibulhaque/similar](https://github.com/mahibulhaque/similar)'s
`GetCloseMatches` (`text.go`) computes its similarity ratio by running
**Myers' diff algorithm** (`captureRatio(ctx, Myers, seq1, seq2)`), not the
Ratcliff-Obershelp longest-contiguous-match algorithm `SequenceMatcher` uses —
CPython's own module doc is explicit that R-O "does not yield minimal edit
sequences" the way Myers does, so the two diverge on ordinary inputs, not just
edge cases. Its tie-break also sorts equal-score matches by ascending word
(`matches[i].word < matches[j].word`), the opposite of CPython's
`heapq.nlargest` order, which — as a `(score, word)` tuple comparison under
`reverse=True` — puts the lexicographically *larger* word first on a tie.

This package reimplements the full documented surface, translated closely
from CPython 3.14's `Lib/difflib.py` (see [License](#license)) to match its
output exactly, including the "autojunk" popular-element heuristic and
`get_close_matches`'s tie-break.

## Installation

```sh
go get github.com/__GH_OWNER__/difflib
```

## Usage

### Compare two sequences

`SequenceMatcher` is generic over any comparable element type — not just
strings:

```go
sm := difflib.NewSequenceMatcher[int](nil, []int{1, 2, 3, 4, 5}, []int{1, 2, 30, 4, 5})
for _, op := range sm.GetOpCodes() {
    fmt.Println(op.Tag, op.I1, op.I2, op.J1, op.J2)
}
```

### Fuzzy-match a word against a list of candidates

```go
matches, err := difflib.GetCloseMatchesString("wheel", []string{"while", "shield", "wheeled", "heel"}, 3, 0.6)
```

### Produce a human-readable line diff

```go
lines := difflib.Ndiff(
    []string{"one\n", "two\n", "three\n"},
    []string{"ore\n", "tree\n", "emu\n"},
)
fmt.Print(strings.Join(lines, ""))
// - one
// ?  ^
// + ore
// ?  ^
// - two
// - three
// ?  -
// + tree
// + emu
```

### Produce a unified or context diff

```go
lines := difflib.UnifiedDiffStrings(oldLines, newLines, "a.txt", "b.txt")
fmt.Print(strings.Join(lines, ""))
```

### Recover an input from an ndiff delta

```go
delta := difflib.Ndiff(a, b)
recoveredA, _ := difflib.Restore(delta, 1)
recoveredB, _ := difflib.Restore(delta, 2)
```

## Reference

### SequenceMatcher[T comparable]

| Function/method | Signature | Notes |
| --- | --- | --- |
| `NewSequenceMatcher` | `[T](isJunk func(T) bool, a, b []T) *SequenceMatcher[T]` | autojunk enabled, matching `SequenceMatcher(isjunk, a, b)` |
| `NewSequenceMatcherAutoJunk` | `[T](isJunk func(T) bool, a, b []T, autoJunk bool) *SequenceMatcher[T]` | matches `SequenceMatcher(isjunk, a, b, autojunk=...)` |
| `SetSeqs`, `SetSeq1`, `SetSeq2` | `(a, b []T)` | as in Python |
| `FindLongestMatch` | `(alo, ahi, blo, bhi int) Match` | |
| `GetMatchingBlocks` | `() []Match` | cached |
| `GetOpCodes` | `() []OpCode` | cached |
| `GetGroupedOpCodes` | `(n int) [][]OpCode` | never mutates cached state between calls — see [How it works](#how-it-works) |
| `Ratio`, `QuickRatio`, `RealQuickRatio` | `() float64` | |

`Match{A, B, Size int}` and `OpCode{Tag string, I1, I2, J1, J2 int}` mirror
CPython's `Match` namedtuple and opcode tuples. `Tag` is one of the
`OpReplace`/`OpDelete`/`OpInsert`/`OpEqual` constants (the same strings
CPython uses: `"replace"`, `"delete"`, `"insert"`, `"equal"`).

### Close matches

| Function | Signature |
| --- | --- |
| `GetCloseMatches` | `[T comparable](word []T, possibilities [][]T, n int, cutoff float64) ([][]T, error)` |
| `GetCloseMatchesString` | `(word string, possibilities []string, n int, cutoff float64) ([]string, error)` |

Both return an error for `n <= 0` or `cutoff` outside `[0, 1]`, matching
CPython's `ValueError`s. `GetCloseMatchesString` reproduces CPython's
`heapq.nlargest`-over-`(score, word)` tie-break exactly (see
[Limitations](#limitations) for the generic form).

### Differ and line-oriented functions

| Function | Signature |
| --- | --- |
| `NewDiffer` | `(lineJunk func(string) bool, charJunk func(rune) bool) *Differ` |
| `(*Differ) Compare` | `(a, b []string) []string` |
| `Ndiff` | `(a, b []string) []string` — `IsCharacterJunk` as charjunk, no linejunk, matching `ndiff(a, b)` |
| `NdiffFunc` | `(a, b []string, lineJunk func(string) bool, charJunk func(rune) bool) []string` |
| `Restore` | `(delta []string, which int) ([]string, error)` — `which` must be 1 or 2 |
| `IsLineJunk` | `(line string) bool` |
| `IsCharacterJunk` | `(ch rune) bool` |

### Unified and context diffs

| Function | Signature |
| --- | --- |
| `UnifiedDiff` | `(a, b []string, fromFile, toFile, fromFileDate, toFileDate string, n int, lineTerm string) []string` |
| `UnifiedDiffStrings` | `(a, b []string, fromFile, toFile string) []string` — `n=3, lineTerm="\n"` |
| `ContextDiff` | same shape as `UnifiedDiff` |
| `ContextDiffStrings` | same shape as `UnifiedDiffStrings` |

## How it works

`SequenceMatcher` builds a map from each element of `b` to the indices where
it occurs (`b2j`), skipping junk elements and — once `len(b) >= 200` and
autojunk is on — "popular" elements accounting for more than roughly 1% of
`b` (CPython's exact `n/100 + 1` threshold). `FindLongestMatch` then finds the
longest junk-free run shared by a window of `a` and `b` using that map, and
extends it with adjacent junk on both sides. `GetMatchingBlocks` applies this
recursively (via an explicit stack, matching CPython's non-recursive
implementation) to the pieces left and right of each match, and
`GetOpCodes`/`GetGroupedOpCodes` turn the resulting blocks into an edit
script. `Differ` runs this same matcher over lines, then — inside a
`"replace"` opcode — runs a second `SequenceMatcher` over the *characters* of
nearby line pairs to decide whether they're similar enough (ratio > 0.74999,
within a window of 10 lines) to mark up with a `?` intraline hint instead of
a plain delete+insert.

One CPython behavior this port deliberately does **not** reproduce: reading
`Lib/difflib.py`, `get_grouped_opcodes(n)` mutates the *cached* opcodes list
in place (`codes = self.get_opcodes()` returns the live list, then
`codes[0] = ...`/`codes[-1] = ...` writes into it). That mutation is
undocumented and leaks into later calls to `get_opcodes()` or
`get_grouped_opcodes()` with a different `n` on the same instance — confirmed
by calling it twice with different `n` values on one `SequenceMatcher` and
watching the second call's result change depending on the first. Every
internal user of it (`unified_diff`, `context_diff`) always builds a fresh
`SequenceMatcher`, so the leak is invisible in practice; this port's
`GetGroupedOpCodes` copies before mutating; instead, so it is safe to call
repeatedly with different `n` on one matcher.

## Limitations

- `HtmlDiff` and the internal `_mdiff` side-by-side diff support are not
  ported. They're a large, separate surface (HTML templates, `wrapcolumn`,
  legend generation) that this port's time-box didn't reach; everything else
  in CPython's public `difflib.__all__` is implemented.
- `diff_bytes` is not ported. In Python it exists to let `unified_diff`/
  `context_diff` work on `bytes` as well as `str`; Go's type system already
  keeps `[]byte`-based and `string`-based lines distinct and type-safe, so
  the shim CPython needs doesn't have a Go equivalent problem to solve.
  `_check_types`, which exists only to give a better error for Python's
  bytes/str mixups, is skipped for the same reason.
- `GetCloseMatches[T]` (the generic form) breaks ties among equally-scored
  possibilities by input order only, not CPython's `(score, word)` tuple
  order, because Go has no total ordering for an arbitrary `comparable` type.
  `GetCloseMatchesString` reproduces CPython's exact tie-break (verified
  against it — see `testdata/cases.json`'s tied-score cases), since Go's
  byte-wise string comparison agrees with Python's code-point comparison for
  valid UTF-8.
- Differential coverage is randomized-and-seeded (`math/rand` with a fixed
  seed in tests, Python's own PRNG with a fixed seed in the generator), not
  exhaustive. See [Development](#development) for how to regenerate or widen
  it.

## Compatibility

Tested with Go 1.27.1 (darwin/arm64) against CPython 3.14.7's `difflib`. The
`go.mod` directive is `go 1.23`; CI (`.github/workflows/test.yml`) runs the
same checks on `ubuntu-latest` with Go 1.23 — untested by this repo itself
beyond what CI runs.

## Development

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .   # must print nothing
```

`testdata/cases.json` holds 962 differential test cases — real output
captured by running CPython 3.14.7's stdlib `difflib` module directly (no
package to install; it's the standard library) across `SequenceMatcher`
(443, over both character and line sequences, short and long/autojunk-length,
every `isjunk`/`autojunk` combination), `get_close_matches` (51, including
`ValueError` cases and deliberately tied scores), `Differ.compare`/`ndiff`
(157 + 152), `restore` (6), `unified_diff`/`context_diff` (64 each), and
`IS_LINE_JUNK`/`IS_CHARACTER_JUNK` (15 + 10) — via the generator script kept
alongside the fixture. Some cases are randomized with a fixed seed; others
are reconstructed from reading CPython's `Lib/test/test_difflib.py` (not
copied from it) — e.g. the "align top to bottom" `Differ` case, the tab-hint
and popular-character `find_longest_match` cases, and the documented
`ValueError` cases for `get_close_matches`/`restore`. `differential_test.go`
replays every case through this package and requires exact agreement,
including on the three ratio functions, matching blocks, opcodes, grouped
opcodes at five context widths, and the `bjunk`/`bpopular` sets.

To regenerate against a different CPython version:

```sh
cd testdata
python3.14 -m venv venv
./venv/bin/python gen_cases.py
```

Real benchmark output (Apple M5 Pro, `go test -bench=. -benchmem -run=^$`),
diffing 1000 lines with 5% changed:

```
BenchmarkRatio1000Lines5PctChanged-15          	    4008	    250474 ns/op	 1041359 B/op	    9928 allocs/op
BenchmarkGetOpCodes1000Lines5PctChanged-15     	    4725	    255361 ns/op	 1057054 B/op	    9936 allocs/op
BenchmarkUnifiedDiff1000Lines5PctChanged-15    	    4348	    276135 ns/op	 1113017 B/op	   10584 allocs/op
```

## License

MIT. See [LICENSE](LICENSE).

Portions of this package (`sequencematcher.go`, `closematch.go`,
`differ.go`, `restore.go`, `diff.go`, `junk.go`) are a close translation of
CPython 3.14's `Lib/difflib.py` — chosen over an independent reimplementation
because exact parity with CPython's output, including undocumented
tie-breaks and heuristic thresholds, was the point of this package, and the
algorithms (especially `Differ`'s `_fancy_replace` and the recursive matching
block search) are easiest to get exactly right by staying close to the
original structure. Per the Python Software Foundation License Version 2,
this package retains the required notice — "Copyright (c) 2001 Python
Software Foundation; All Rights Reserved" — in those files' headers, and
ships the license text verbatim as [LICENSE-python](LICENSE-python). The
`_test.go` files and `testdata/gen_cases.py` are original.
