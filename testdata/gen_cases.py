"""Generate cases.json, the differential test fixture for the difflib Go
port, by running each case through the real CPython 3.14 `difflib` module
and recording its exact output.

Run from this directory with Python 3.14 (stdlib only, no third-party
dependency needed):

    python3.14 -m venv venv
    ./venv/bin/python gen_cases.py

Case groups, each a top-level JSON array:

- sequence_matcher: SequenceMatcher over both character sequences ("chars",
  a/b given as strings) and line sequences ("lines", a/b given as []string).
  Covers isjunk variants, autojunk on/off, sequences long enough (>=200) to
  trigger the popular-element heuristic, get_matching_blocks, get_opcodes,
  get_grouped_opcodes at a couple of context widths, and the three ratio
  functions.
- close_matches: get_close_matches(word, possibilities, n, cutoff), including
  cases designed to tie on score so the heapq.nlargest tie-break shows up,
  and the documented ValueError cases (recorded as "error": true).
- differ_compare / ndiff: Differ().compare(a, b) and ndiff(a, b) over line
  lists, including near-matching lines that trigger _fancy_replace's
  intraline '?' hints, tabs (the "keep original whitespace" path), and the
  "align top to bottom" case from CPython's own test suite.
- restore: restore(delta, which), including the invalid-`which` ValueError
  case.
- unified_diff / context_diff: various n and lineterm combinations, plus the
  documented tab-delimited header and empty-date cases.
- is_line_junk / is_character_junk: the module-level junk predicates.

Cases marked "derived_from_cpython_tests": true describe behavior CPython's
own Lib/test/test_difflib.py exercises (not copied from it — reconstructed
from reading it, to a JSON fixture instead of Python source).
"""
import difflib
import json
import random

random.seed(20260928)

cases = {
    "sequence_matcher": [],
    "close_matches": [],
    "differ_compare": [],
    "ndiff": [],
    "restore": [],
    "unified_diff": [],
    "context_diff": [],
    "is_line_junk": [],
    "is_character_junk": [],
}

# ---------------------------------------------------------------------------
# isjunk variants used for "chars"-kind SequenceMatcher cases. Names must
# match the switch in differential_test.go's charJunkByName.
CHAR_JUNK = {
    "none": None,
    "space": lambda x: x == " ",
    "space_or_b": lambda x: x in (" ", "b"),
    "vowel": lambda x: x in "aeiou",
}

# isjunk variants for "lines"-kind SequenceMatcher cases (must match
# lineJunkByName in differential_test.go).
LINE_JUNK = {
    "none": None,
    "is_line_junk": difflib.IS_LINE_JUNK,
    "blank_only": lambda x: x.strip() == "",
}


def add_sm_case(kind, a, b, isjunk_name, autojunk, junk_table):
    isjunk = junk_table[isjunk_name]
    sm = difflib.SequenceMatcher(isjunk, a, b, autojunk=autojunk)
    mb = [[m.a, m.b, m.size] for m in sm.get_matching_blocks()]
    oc = [[t, i1, i2, j1, j2] for t, i1, i2, j1, j2 in sm.get_opcodes()]
    # get_grouped_opcodes(n) has an undocumented side effect: it mutates
    # the first/last entries of the *cached* opcodes list in place (see
    # CPython Lib/difflib.py — `codes = self.get_opcodes()` returns the
    # live self.opcodes list, then assigns into codes[0]/codes[-1]).
    # That mutation leaks into every later get_opcodes()/
    # get_grouped_opcodes() call on the same instance. It is not part of
    # the documented contract (nothing says grouping depends on call
    # order or n), so each n here gets its own fresh SequenceMatcher to
    # record the *intended* (order-independent) result, which is what
    # the Go port implements.
    grouped = {}
    for n in (0, 1, 2, 3, 5):
        fresh = difflib.SequenceMatcher(isjunk, a, b, autojunk=autojunk)
        grouped[str(n)] = [
            [[t, i1, i2, j1, j2] for t, i1, i2, j1, j2 in g]
            for g in fresh.get_grouped_opcodes(n)
        ]
    cases["sequence_matcher"].append({
        "kind": kind,
        "a": a,
        "b": b,
        "isjunk": isjunk_name,
        "autojunk": autojunk,
        "ratio": sm.ratio(),
        "quick_ratio": sm.quick_ratio(),
        "real_quick_ratio": sm.real_quick_ratio(),
        "matching_blocks": mb,
        "opcodes": oc,
        "grouped_opcodes": grouped,
        "bjunk": sorted(sm.bjunk) if kind == "chars" else sorted(sm.bjunk),
        "bpopular": sorted(sm.bpopular) if kind == "chars" else sorted(sm.bpopular),
    })


# --- derived from CPython's TestWithAscii / TestAutojunk / TestSFbugs -----
derived_sm = [
    ("chars", "b" * 100, "a" + "b" * 100, "none", True),
    ("chars", "b" * 100, "b" * 50 + "a" + "b" * 50, "none", True),
    ("chars", "a" * 40 + "c" + "b" * 40, "a" * 40 + "b" * 40, "none", True),
    ("chars", "a" * 40 + "b" * 40, "a" * 44 + "b" * 40, "space", True),
    ("chars", "a" * 40 + "b" * 40, "a" * 44 + "b" * 40 + " " * 20, "space", True),
    ("chars", "a" * 40 + "b" * 40, "a" * 44 + "b" * 40 + " " * 20, "space_or_b", True),
    ("chars", "b" * 200, "a" + "b" * 200, "none", True),
    ("chars", "b" * 200, "a" + "b" * 200, "none", False),
    ("chars", "", "", "none", True),
    ("chars", "abxcd", "abcd", "none", True),
    ("chars", "dabcd", "d" * 100 + "abc" + "d" * 100, "none", True),
    ("chars", "foo bar", "foo baz bar", "none", True),
    ("lines", [], [], "none", True),
]
for kind, a, b, junk_name, autojunk in derived_sm:
    add_sm_case(kind, a, b, junk_name, autojunk, CHAR_JUNK if kind == "chars" else LINE_JUNK)

# --- randomized "chars" cases: short alphabets to force repeats/collisions
alphabets = ["ab", "abc", "abcd", "abcde", "xyzxyz", "aaab", "abcdefgh"]
for _ in range(220):
    alpha = random.choice(alphabets)
    la = random.randint(0, 40)
    lb = random.randint(0, 40)
    a = "".join(random.choice(alpha) for _ in range(la))
    b = "".join(random.choice(alpha) for _ in range(lb))
    junk_name = random.choice(list(CHAR_JUNK))
    autojunk = random.choice([True, False])
    add_sm_case("chars", a, b, junk_name, autojunk, CHAR_JUNK)

# --- randomized "chars" cases long enough to trigger autojunk (>=200) -----
for _ in range(60):
    alpha = random.choice(["ab", "abc", "aaab", "xy"])
    la = random.randint(200, 400)
    lb = random.randint(200, 400)
    a = "".join(random.choice(alpha) for _ in range(la))
    b = "".join(random.choice(alpha) for _ in range(lb))
    junk_name = random.choice(list(CHAR_JUNK))
    autojunk = random.choice([True, False])
    add_sm_case("chars", a, b, junk_name, autojunk, CHAR_JUNK)

# --- randomized "lines" cases, with repeated lines (to trigger autojunk
# via popular *line* values once long enough), and blank lines -------------
line_pool = [
    "hello there\n", "how are you\n", "\n", "# comment\n", "foo\n", "bar\n",
    "the quick brown fox\n", "same line\n", "same line\n", "  \n",
    "line with trailing space \n",
]
for _ in range(120):
    la = random.randint(0, 30)
    lb = random.randint(0, 30)
    a = [random.choice(line_pool) for _ in range(la)]
    b = [random.choice(line_pool) for _ in range(lb)]
    junk_name = random.choice(list(LINE_JUNK))
    autojunk = random.choice([True, False])
    add_sm_case("lines", a, b, junk_name, autojunk, LINE_JUNK)

for _ in range(30):
    # long, highly repetitive line sequences to actually cross the
    # autojunk n>=200 threshold on *lines*.
    la = random.randint(200, 260)
    lb = random.randint(200, 260)
    a = [random.choice(["same\n", "same\n", "same\n", "different %d\n" % i]) for i in range(la)]
    b = [random.choice(["same\n", "same\n", "same\n", "different %d\n" % i]) for i in range(lb)]
    add_sm_case("lines", a, b, "none", random.choice([True, False]), LINE_JUNK)

# ---------------------------------------------------------------------------
# get_close_matches

def add_close_case(word, possibilities, n, cutoff):
    try:
        result = difflib.get_close_matches(word, possibilities, n, cutoff)
        cases["close_matches"].append({
            "word": word, "possibilities": possibilities, "n": n,
            "cutoff": cutoff, "result": result, "error": False,
        })
    except ValueError:
        cases["close_matches"].append({
            "word": word, "possibilities": possibilities, "n": n,
            "cutoff": cutoff, "result": None, "error": True,
        })


add_close_case("appel", ["ape", "apple", "peach", "puppy"], 3, 0.6)
add_close_case("wheel", ["while", "shield", "wheeled", "heel"], 3, 0.6)
add_close_case("Apple", ["apple", "Apple", "APPLE"], 3, 0.6)
add_close_case("spam", ["egg"], 0, 0.6)     # ValueError: n must be > 0
add_close_case("spam", ["egg"], -1, 0.6)    # ValueError
add_close_case("spam", ["egg"], 3, 1.1)     # ValueError
add_close_case("spam", ["egg"], 3, -0.1)    # ValueError
add_close_case("abc", [], 3, 0.6)
# ties: several possibilities equidistant from word, to exercise
# heapq.nlargest's (score, word) tie-break.
add_close_case("aaaa", ["aaab", "aaac", "aaad", "aaae"], 3, 0.1)
add_close_case("xxxx", ["yxxx", "xxxy", "xyxx", "xxyx"], 4, 0.1)
add_close_case("same", ["same", "same", "same"], 2, 0.6)

words = ["hello", "world", "python", "difflib", "sequence", "matcher", "banana", ""]
pools = [
    ["hallo", "hullo", "yellow", "help"],
    ["word", "wold", "worlds", "ward"],
    ["pithon", "cython", "pyThon", "typhon"],
    [],
    ["a", "aa", "aaa"],
]
for word in words:
    for pool in pools:
        n = random.choice([1, 2, 3, 5])
        cutoff = round(random.uniform(0.0, 1.0), 2)
        add_close_case(word, pool, n, cutoff)

# ---------------------------------------------------------------------------
# Differ.compare / ndiff

def add_differ_case(a, b, tag):
    result = list(difflib.Differ().compare(a, b))
    cases["differ_compare"].append({"a": a, "b": b, "result": result, "tag": tag})


def add_ndiff_case(a, b, tag):
    result = list(difflib.ndiff(a, b))
    cases["ndiff"].append({"a": a, "b": b, "result": result, "tag": tag})


# derived from CPython's TestDiffer / TestSFbugs
add_differ_case(
    ["cat\n", "dog\n", "close match 1\n", "close match 2\n"],
    ["close match 3\n", "close match 4\n", "kitten\n", "puppy\n"],
    "close_matches_aligned",
)
add_differ_case(list("bb"), list("a") + list("bb"), "one_insert_chars")
add_differ_case(list("a") + list("bb"), list("bb"), "one_delete_chars")
add_differ_case(["\tI am a buggy"], ["\t\tI am a bug"], "added_tab_hint")
add_differ_case(["\t \t \t^"], ["\t \t \t^\n"], "hint_indented_with_tabs")
add_differ_case(
    "one\ntwo\nthree\n".splitlines(keepends=True),
    "ore\ntree\nemu\n".splitlines(keepends=True),
    "ndiff_docstring_example",
)
add_ndiff_case(
    "one\ntwo\nthree\n".splitlines(keepends=True),
    "ore\ntree\nemu\n".splitlines(keepends=True),
    "ndiff_docstring_example",
)

text1 = "  1. Beautiful is better than ugly.\n  2. Explicit is better than implicit.\n  3. Simple is better than complex.\n  4. Complex is better than complicated.\n"
text2 = "  1. Beautiful is better than ugly.\n  3.   Simple is better than complex.\n  4. Complicated is better than complex.\n  5. Flat is better than nested.\n"
add_differ_case(text1.splitlines(keepends=True), text2.splitlines(keepends=True), "differ_class_docstring")
add_ndiff_case(text1.splitlines(keepends=True), text2.splitlines(keepends=True), "differ_class_docstring")

# randomized line-level differ/ndiff cases, including near-duplicate lines
# (to trigger fancy_replace intraline hints) and tabs/whitespace variants.
base_sentences = [
    "the quick brown fox jumps",
    "the slow brown fox jumps",
    "over the lazy dog",
    "over the lazy cat",
    "hello world",
    "hello  world",
    "\thello\tworld",
    "goodbye world",
    "line with a change here",
    "line with a chAnge here",
    "",
]
for _ in range(150):
    la = random.randint(0, 8)
    lb = random.randint(0, 8)
    a = [random.choice(base_sentences) + "\n" for _ in range(la)]
    b = [random.choice(base_sentences) + "\n" for _ in range(lb)]
    add_differ_case(a, b, "random")
    add_ndiff_case(a, b, "random")

# ---------------------------------------------------------------------------
# restore

def add_restore_case(delta, which, tag):
    try:
        result = list(difflib.restore(delta, which))
        cases["restore"].append({"delta": delta, "which": which, "result": result, "error": False, "tag": tag})
    except ValueError:
        cases["restore"].append({"delta": delta, "which": which, "result": None, "error": True, "tag": tag})


d = list(difflib.ndiff(
    "one\ntwo\nthree\n".splitlines(keepends=True),
    "ore\ntree\nemu\n".splitlines(keepends=True),
))
add_restore_case(d, 1, "ndiff_docstring_example")
add_restore_case(d, 2, "ndiff_docstring_example")
add_restore_case([], 0, "invalid_which_0")
add_restore_case([], 3, "invalid_which_3")
add_restore_case(["  x\n", "- y\n", "+ z\n", "? ^\n", "short"], 1, "short_line_and_hint")
add_restore_case(["  x\n", "- y\n", "+ z\n", "? ^\n", "short"], 2, "short_line_and_hint")

# ---------------------------------------------------------------------------
# unified_diff / context_diff

def add_unified_case(a, b, fromfile, tofile, fromdate, todate, n, lineterm, tag):
    result = list(difflib.unified_diff(a, b, fromfile, tofile, fromdate, todate, n, lineterm))
    cases["unified_diff"].append({
        "a": a, "b": b, "fromfile": fromfile, "tofile": tofile,
        "fromdate": fromdate, "todate": todate, "n": n, "lineterm": lineterm,
        "result": result, "tag": tag,
    })


def add_context_case(a, b, fromfile, tofile, fromdate, todate, n, lineterm, tag):
    result = list(difflib.context_diff(a, b, fromfile, tofile, fromdate, todate, n, lineterm))
    cases["context_diff"].append({
        "a": a, "b": b, "fromfile": fromfile, "tofile": tofile,
        "fromdate": fromdate, "todate": todate, "n": n, "lineterm": lineterm,
        "result": result, "tag": tag,
    })


one_two_three_four = "one two three four".split()
zero_one_tree_four = "zero one tree four".split()
add_unified_case(one_two_three_four, zero_one_tree_four, "Original", "Current",
                  "2005-01-26 23:30:50", "2010-04-02 10:20:52", 3, "", "docstring_example")
add_context_case(
    "one\ntwo\nthree\nfour\n".splitlines(keepends=True),
    "zero\none\ntree\nfour\n".splitlines(keepends=True),
    "Original", "Current", "", "", 3, "\n", "docstring_example",
)
add_unified_case(["one"], ["two"], "Original", "Current",
                  "2005-01-26 23:30:50", "2010-04-02 10:20:52", 3, "", "tab_delimiter")
add_context_case(["one"], ["two"], "Original", "Current",
                  "2005-01-26 23:30:50", "2010-04-02 10:20:52", 3, "", "tab_delimiter")
add_unified_case(["one"], ["two"], "Original", "Current", "", "", 3, "", "no_trailing_tab_empty_date")
add_context_case(["one"], ["two"], "Original", "Current", "", "", 3, "", "no_trailing_tab_empty_date")
add_unified_case([], [], "", "", "", "", 3, "\n", "empty_both")
add_context_case([], [], "", "", "", "", 3, "\n", "empty_both")

for _ in range(60):
    la = random.randint(0, 20)
    lb = random.randint(0, 20)
    a = [random.choice(base_sentences) + "\n" for _ in range(la)]
    b = [random.choice(base_sentences) + "\n" for _ in range(lb)]
    n = random.choice([0, 1, 2, 3, 5])
    lineterm = random.choice(["\n", ""])
    add_unified_case(a, b, "a.txt", "b.txt", "", "", n, lineterm, "random")
    add_context_case(a, b, "a.txt", "b.txt", "", "", n, lineterm, "random")

# ---------------------------------------------------------------------------
# IS_LINE_JUNK / IS_CHARACTER_JUNK

for line in ["#", "  ", " #", "# ", " # ", "", "##", " ##", "## ", "abc ", "abc #",
             "Mr. Moose is up!", "\n", "   \t  ", "#\n"]:
    cases["is_line_junk"].append({"line": line, "result": difflib.IS_LINE_JUNK(line)})

for ch in [" ", "\t", "a", "#", "\n", "\f", "\r", "\v", "x", "0"]:
    cases["is_character_junk"].append({"ch": ch, "result": difflib.IS_CHARACTER_JUNK(ch)})

with open("cases.json", "w") as f:
    json.dump(cases, f, indent=1, sort_keys=True)

print("wrote cases.json:", {k: len(v) for k, v in cases.items()})
