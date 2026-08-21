// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"testing"
)

// fixturePageMarkup is the deterministic parity-fixture page (all body content
// at micron depth 2 after the `>>` header, no `<` reset), served over loopback
// RNS by tooling/parity-fixtures/fixture-page/index.mu. At depth 2 Python's
// MicronParser wraps each line in Padding(left=2, right=2) (left_indent =
// right_indent = (depth-1)*SECTION_INDENT, MicronParser.py:418-422), so the body
// text wraps at content_width-4 and EVERY wrapped row — including continuations
// — is offset 2 columns. The Go port must mirror that: narrow the wrap by the
// right indent AND indent continuation rows, not just the first row.
const fixturePageMarkup = ">> Parity Fixture Page\n\n" +
	"This is a deterministic fixture page served over loopback RNS.\n\n" +
	"Recently added:\n" +
	"`[Book One`__LOCAL_NODE__:/page/books/1.mu]\n" +
	"`[Book Two`__LOCAL_NODE__:/page/books/2.mu]\n\n" +
	">> Section Two\n" +
	"Some body text here for layout comparison.\n"

// renderedRows renders markup into a BrowserDisplay whose content TextView is
// laid out at the given inner width, and returns the plain (tag-stripped) rows
// of the displayed page text. Tags are stripped via GetText(true); the rows are
// the \n-separated lines the TextView holds (pre-wrapped by the renderer).
func renderedRows(t *testing.T, markup string, innerW int) []string {
	t.Helper()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, innerW, 24)
	bd.RenderPage(markup)
	return strings.Split(bd.content.GetText(true), "\n")
}

// TestBrowserBodyWrapRightIndent pins the Q3 fix: a depth-2 body paragraph wraps
// one word narrower than the full content width (right indent) AND indents its
// continuation row by the left indent, matching Python's Padding(left,right)
// model. Before the fix the Go port wrapped at content_width-left_indent only
// (no right indent) and left continuations at column 0, so "served" landed on the
// first row and "over loopback RNS." at column 0 — diverging from Python which
// puts "served" at the start of an indented continuation row.
func TestBrowserBodyWrapRightIndent(t *testing.T) {
	t.Parallel()
	// innerW=44 → wrap width 44-4=40. "This is a deterministic fixture page" is
	// 35 runes (fits 40); +" served" reaches 42 > 40, so "served" wraps. Both
	// rows are offset 2 (the left indent).
	const innerW = 44
	rows := renderedRows(t, fixturePageMarkup, innerW)

	// Find the body paragraph's first row and assert the wrap + continuation.
	idx := -1
	for i, r := range rows {
		if strings.Contains(r, "This is a deterministic fixture page") {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("body paragraph row not found in:\n%s", strings.Join(rows, "\n"))
	}
	first := rows[idx]
	if strings.Contains(first, "served") {
		t.Errorf("body first row contains \"served\" (wrapped too wide, missing right indent): %q", first)
	}
	if !strings.HasPrefix(first, "  This is a deterministic fixture page") {
		t.Errorf("body first row not left-indented by 2: %q", first)
	}
	if idx+1 >= len(rows) {
		t.Fatalf("no continuation row after body first row")
	}
	second := rows[idx+1]
	if !strings.HasPrefix(second, "  served over loopback RNS.") {
		t.Errorf("body continuation row = %q, want \"  served over loopback RNS.\" (continuation indented by left indent)", second)
	}
}

// TestBrowserBodyWrapDepth0NoIndent pins the depth-0 counterpart: at depth 0
// the left and right indents are both 0, so the body wraps at the full content
// width with continuations at column 0 — no indent on any row. Uses a depth-0
// paragraph long enough to wrap at width 30.
func TestBrowserBodyWrapDepth0NoIndent(t *testing.T) {
	t.Parallel()
	markup := "Some longer paragraph that surely wraps around the column boundary here.\n"
	const innerW = 30
	rows := renderedRows(t, markup, innerW)
	// The paragraph must wrap into more than one row (pre-wrap is active at every
	// depth), and every row must sit at column 0 (no left indent at depth 0).
	wrapped := false
	for _, r := range rows {
		if r == "" {
			continue
		}
		if strings.HasPrefix(r, " ") {
			t.Errorf("depth-0 row is indented (should wrap at full width, col 0): %q", r)
		}
	}
	for _, r := range rows {
		if strings.Contains(r, "column boundary") {
			wrapped = true
		}
	}
	// "column boundary here." should be on its own (wrapped) row, proving the
	// paragraph wrapped rather than overflowing one row.
	if !wrapped {
		t.Fatalf("depth-0 paragraph did not wrap:\n%s", strings.Join(rows, "\n"))
	}
}
