#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.
"""Find text frozen in the Go port from a SUPERSEDED Python release.

A parity test that asserts a hand-transcribed literal can pin wording from the
WRONG nomadnet release, and because it then asserts that wording it *enforces*
the divergence instead of catching it. That is not hypothetical: the
identity-unknown conversation banner was transcribed from a nomadnet 1.2.3
checkout, 1.2.8 reworded it, and the frozen literal hid the change for the whole
life of the port (`tui/editor-banner-parity_test.go`).

This scan finds that class exhaustively. It compares the string literals of a
superseded Python tree against the source-of-truth tree and reports every
superseded literal still present, verbatim, in the Go sources:

    /opt/homebrew/bin/python3 tooling/sot_drift_scan.py

Run it with an interpreter that has the pinned nomadnet installed (the script
resolves the SOT tree from that interpreter's own `import nomadnet`), and point
`--stale` at the checkout to compare against when the pin moves.

What it catches: Go source carrying text that exists in the stale tree but not
in the SOT — i.e. a frozen or wrongly-transcribed reference string.

What it CANNOT catch, and which needs the reverse sweep (every user-visible SOT
string must appear somewhere in Go):

  * wording the Go port invented that never existed in any release, and
  * a SOT string the Go port omits entirely.

Matching is exact on both sides. Python's adjacent string literals are folded by
`ast`, so text assembled from several quoted fragments is compared as the single
string a caller would see; the Go side is folded the same way, by decoding each
run of literals joined with `+` (the port's own multi-line concatenation breaks
the text at different points than Python did). Requiring a verbatim substring
rather than a fuzzy overlap is what distinguishes "the Go port froze this
sentence" from "this file holds a long block that merely resembles the old one,
while genuinely missing what the SOT added" — the latter is a real but different
finding, out of scope here.

Exit status: 0 no drift (or nothing to compare), 1 drift found, 2 could not run.
"""

import argparse
import ast
import re
import sys
from pathlib import Path

DEFAULT_STALE_CANDIDATES = (
    Path("original-nomadnet-repo"),
    Path.home() / "src/github.com/markqvist/nomadnet",
)

_VERSION_RE = re.compile(r"""^__version__\s*=\s*["']([^"']+)["']""", re.MULTILINE)

# One Go string literal: interpreted ("…" with escapes) or raw (`…`).
_GO_LITERAL_RE = re.compile(r'"((?:[^"\\\n]|\\.)*)"|`([^`]*)`')

# A maximal run of Go literals joined by `+`, which is how the port spreads a
# paragraph over several source lines.
_GO_CONCAT_RE = re.compile(
    r'(?:"(?:[^"\\\n]|\\.)*"|`[^`]*`)'
    r'(?:\s*\+\s*(?:"(?:[^"\\\n]|\\.)*"|`[^`]*`))*'
)

_GO_ESCAPES = {
    "a": "\a",
    "b": "\b",
    "f": "\f",
    "n": "\n",
    "r": "\r",
    "t": "\t",
    "v": "\v",
    "\\": "\\",
    '"': '"',
    "'": "'",
}


def decode_go_interpreted(body):
    """Decode the body of a Go interpreted string literal to its value.

    Covers the escapes that can appear in message text: the single-character
    set, \\xHH, \\uHHHH, \\UHHHHHHHH and three-digit octal. An unknown escape
    keeps the escaped character, which is what Go reports as a compile error
    rather than something this scan needs to model.
    """
    out = []
    i = 0
    while i < len(body):
        char = body[i]
        if char != "\\" or i + 1 >= len(body):
            out.append(char)
            i += 1
            continue
        nxt = body[i + 1]
        if nxt in _GO_ESCAPES:
            out.append(_GO_ESCAPES[nxt])
            i += 2
        elif nxt == "x" and i + 4 <= len(body):
            out.append(chr(int(body[i + 2 : i + 4], 16)))
            i += 4
        elif nxt == "u" and i + 6 <= len(body):
            out.append(chr(int(body[i + 2 : i + 6], 16)))
            i += 6
        elif nxt == "U" and i + 10 <= len(body):
            out.append(chr(int(body[i + 2 : i + 10], 16)))
            i += 10
        elif nxt in "01234567" and i + 4 <= len(body):
            out.append(chr(int(body[i + 1 : i + 4], 8)))
            i += 4
        else:
            out.append(nxt)
            i += 2
    return "".join(out)


def go_folded_strings(source):
    """Return every folded string value in a Go source file.

    Each element is one run of literals joined by `+`, decoded, so a paragraph
    the port split across lines reads as the single string the program builds.
    """
    folded = []
    for group in _GO_CONCAT_RE.finditer(source):
        parts = []
        for literal in _GO_LITERAL_RE.finditer(group.group(0)):
            interpreted, raw = literal.group(1), literal.group(2)
            parts.append(decode_go_interpreted(interpreted) if raw is None else raw)
        if parts:
            folded.append("".join(parts))
    return folded


def read_version(root):
    """Return the version declared in root/_version.py, or None.

    Read as text rather than imported: the stale tree is a checkout whose
    dependencies are not necessarily installable, and importing it could shadow
    the reference the caller is trying to compare against.
    """
    try:
        text = (Path(root) / "_version.py").read_text(encoding="utf-8")
    except OSError:
        return None
    match = _VERSION_RE.search(text)
    return match.group(1) if match else None


def string_literals(root, min_len):
    """Map each relative .py path under root to the set of its string constants.

    `ast` folds adjacent string literals, so a value spread over several quoted
    fragments is collected as the one string the program would actually use.
    Values shorter than min_len, or with no space in them, are dropped: they are
    identifiers, keys and format fragments that would only add noise.
    """
    out = {}
    for path in sorted(Path(root).rglob("*.py")):
        if "__pycache__" in path.parts:
            continue
        try:
            tree = ast.parse(path.read_text(encoding="utf-8", errors="replace"))
        except SyntaxError:
            continue
        out[str(path.relative_to(root))] = {
            node.value
            for node in ast.walk(tree)
            if isinstance(node, ast.Constant)
            and isinstance(node.value, str)
            and " " in node.value
            and len(node.value) >= min_len
        }
    return out


def superseded_literals(stale_root, sot_root, min_len):
    """Return {literal: stale relative path} for text gone from the SOT.

    A literal counts as superseded only when its own file exists in both trees:
    a file added or removed between releases is a structural change this scan
    does not speak to.
    """
    stale, sot = string_literals(stale_root, min_len), string_literals(sot_root, min_len)
    superseded = {}
    for rel, values in stale.items():
        if rel not in sot:
            continue
        for value in values - sot[rel]:
            superseded.setdefault(value, rel)
    return superseded


def go_sources(go_root):
    """Return [(relative path, source text)] for every .go file under go_root."""
    sources = []
    for path in sorted(Path(go_root).rglob("*.go")):
        if "vendor" in path.parts or ".git" in path.parts:
            continue
        try:
            sources.append(
                (
                    str(path.relative_to(go_root)),
                    path.read_text(encoding="utf-8", errors="replace"),
                )
            )
        except OSError:
            continue
    return sources


def scan(go_root, superseded):
    """Return [(go relative path, stale relative path, literal)] for frozen text.

    Reports the first Go file that contains each superseded literal verbatim.
    """
    hits = []
    remaining = dict(superseded)
    for go_rel, source in go_sources(go_root):
        if not remaining:
            break
        folded = go_folded_strings(source)
        for value, stale_rel in list(remaining.items()):
            if any(value in chunk for chunk in folded):
                hits.append((go_rel, stale_rel, value))
                del remaining[value]
    return hits


def as_package_dir(path):
    """Return the nomadnet package directory for either tree layout, or None.

    Accepts a package directory (which holds _version.py directly) or a checkout
    root (which holds nomadnet/_version.py), so an installed reference and a git
    checkout can both be named the same way on the command line.
    """
    path = Path(path)
    for candidate in (path, path / "nomadnet"):
        if (candidate / "_version.py").exists():
            return candidate
    return None


def resolve_sot_tree(explicit):
    """Return the source-of-truth nomadnet package directory, or None.

    Prefers an explicit path, then the tree the running interpreter imports, so
    the scan compares against exactly the reference the parity tests use.
    """
    if explicit:
        return as_package_dir(explicit)
    try:
        import nomadnet

        return as_package_dir(Path(nomadnet.__file__).resolve().parent)
    except ImportError:
        return None


def resolve_stale_tree(explicit, go_root):
    """Return the superseded nomadnet package directory, or None."""
    if explicit:
        return as_package_dir(explicit)
    for candidate in DEFAULT_STALE_CANDIDATES:
        path = candidate if candidate.is_absolute() else Path(go_root) / candidate
        if (package := as_package_dir(path)) is not None:
            return package
    return None


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Find text frozen in the Go port from a superseded Python release.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="Run with the interpreter that has the pinned nomadnet installed.",
    )
    parser.add_argument(
        "--stale",
        help="superseded nomadnet checkout (default: ./original-nomadnet-repo, then "
        "$HOME/src/github.com/markqvist/nomadnet)",
    )
    parser.add_argument(
        "--sot",
        help="source-of-truth nomadnet package (default: the tree the running "
        "interpreter imports)",
    )
    parser.add_argument(
        "--go",
        default=str(Path(__file__).resolve().parent.parent),
        help="Go tree to search (default: this repository)",
    )
    parser.add_argument(
        "--min-len",
        type=int,
        default=20,
        help="shortest literal to compare (default: 20)",
    )
    args = parser.parse_args(argv)

    sot_root = resolve_sot_tree(args.sot)
    if sot_root is None or not (sot_root / "_version.py").exists():
        print(
            "error: could not locate the source-of-truth nomadnet package.\n"
            "       Run this with an interpreter that has the pinned reference installed,\n"
            "       or pass --sot /path/to/nomadnet.",
            file=sys.stderr,
        )
        return 2

    stale_root = resolve_stale_tree(args.stale, args.go)
    if stale_root is None or not (Path(stale_root) / "_version.py").exists():
        print(
            "note: no superseded nomadnet checkout found, so there is nothing to compare.\n"
            "      Pass --stale /path/to/older/nomadnet to scan against one.",
            file=sys.stderr,
        )
        return 0

    stale_ver, sot_ver = read_version(stale_root), read_version(sot_root)
    print(f"superseded tree : {Path(stale_root).resolve()} (nomadnet {stale_ver})")
    print(f"source of truth : {sot_root.resolve()} (nomadnet {sot_ver})")
    if stale_ver is not None and stale_ver == sot_ver:
        print(
            "warning: both trees report the same version; if the superseded tree is meant\n"
            "         to be older, check out the intended release before trusting this scan.",
            file=sys.stderr,
        )

    superseded = superseded_literals(stale_root, sot_root, args.min_len)
    hits = scan(args.go, superseded)
    print(f"\n{len(superseded)} superseded literal(s); {len(hits)} still frozen in Go")

    if not hits:
        print("\nno drift: the Go tree carries no superseded reference text.")
        return 0

    print()
    for go_rel, stale_rel, value in hits:
        print(f"{go_rel}\n    superseded text from {stale_rel}\n    {value[:160]!r}\n")
    print(
        "Re-derive each of these from the source-of-truth tree rather than transcribing\n"
        "it, and prefer a test that derives its expectation from the reference (see\n"
        "testutils.RunPythonNomadnet) over one asserting a copied literal."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
