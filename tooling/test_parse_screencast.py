#!/usr/bin/env python3
# Copyright 2026 Glenn Lewis. All rights reserved.
#
# Tests for tooling/parse_screencast.py — terminal model display-width handling.
#
# These pin the T-UTF8 fix: _putc must advance the cursor by the Unicode display
# width of each grapheme (2 for East-Asian Wide/Fullwidth, 0 for combining marks
# and other nonspacing/format characters, 1 otherwise), NOT unconditionally by 1.
# Without this, wide/CJK graphemes misalign the cell grid in the decoded
# comparison (TODO.md T-UTF8).

import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
from parse_screencast import Term  # noqa: E402

CJK = "中"  # 中 — East Asian Wide, display width 2
COMBINING_ACUTE = "́"  # combining acute accent, display width 0


def _fresh(cols=10, rows=1):
    return Term(cols, rows)


def test_wide_char_advances_cursor_by_two():
    """A CJK wide char occupies two cells; the cursor must advance by 2 so the
    following char lands in the third cell, not the second."""
    t = _fresh()
    t.feed("A" + CJK + "B")
    assert t.cx == 4, f"expected cx==4 (A=1 + wide=2 + B=1), got {t.cx}"
    assert t.grid[0][0].ch == "A"
    assert t.grid[0][1].ch == CJK
    # The wide char's second cell is a continuation placeholder, NOT 'B'.
    assert t.grid[0][2].ch == "", (
        f"wide-char continuation cell should be empty, got {t.grid[0][2].ch!r}"
    )
    assert t.grid[0][3].ch == "B"


def test_wide_char_text_extraction():
    """screen_text must emit the wide char once (the continuation cell
    contributes nothing), so a row 'A<wide>B' decodes to 'A<wide>B'."""
    t = _fresh(cols=6, rows=1)
    t.feed("A" + CJK + "B")
    assert t.screen_text() == "A" + CJK + "B", repr(t.screen_text())


def test_combining_mark_does_not_advance_cursor():
    """A combining mark (U+0301) is width 0: it must not advance the cursor and
    must not overwrite the base character in its cell."""
    t = _fresh()
    t.feed("e" + COMBINING_ACUTE + "x")
    # e at cx=0 (cx->1), combining mark width 0 (cx stays 1, base kept),
    # x at cx=1 (cx->2).
    assert t.cx == 2, f"expected cx==2, got {t.cx}"
    assert t.grid[0][0].ch == "e", (
        f"combining mark must not overwrite base, got {t.grid[0][0].ch!r}"
    )
    assert t.grid[0][1].ch == "x"


def test_ascii_still_advances_by_one():
    """Regression guard: ordinary ASCII width-1 chars still advance by 1."""
    t = _fresh()
    t.feed("abcd")
    assert t.cx == 4
    assert [c.ch for c in t.grid[0][:4]] == ["a", "b", "c", "d"]


def test_nomadnet_glyphs_are_width_one():
    """The nomadnet glyph set is all East-Asian Neutral/Narrow, so each must
    advance by 1 — the wide-char fix must not regress them."""
    glyphs = "✓✕⚠Ⓝ↓"  # ✓ ✕ ⚠ Ⓝ ↓
    t = _fresh(cols=10, rows=1)
    t.feed(glyphs)
    assert t.cx == 5, f"expected cx==5, got {t.cx}"
    assert t.screen_text() == glyphs, repr(t.screen_text())


def test_wide_char_at_last_column_clamps():
    """A wide char in the final column cannot fit; the cursor clamps at the
    last column (no autowrap), mirroring the existing width-1 clamp behavior."""
    t = _fresh(cols=3, rows=1)
    t.feed("A" + CJK)  # A at 0 (cx->1), wide needs 2 cells but only 1 remains
    assert t.cx == 2, f"expected clamped cx==2, got {t.cx}"


if __name__ == "__main__":
    fns = [v for k, v in sorted(globals().items()) if k.startswith("test_") and callable(v)]
    passed = failed = 0
    for fn in fns:
        try:
            fn()
            print("PASS", fn.__name__)
            passed += 1
        except Exception as e:  # noqa: BLE001
            print("FAIL", fn.__name__, "->", type(e).__name__, str(e)[:140])
            failed += 1
    print(f"\n{passed} passed, {failed} failed")
    sys.exit(1 if failed else 0)