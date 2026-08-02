// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/rivo/tview"
)

// TestBrowserJumpToAnchorScrollOffset asserts JumpToAnchor scrolls the browser
// content so the anchor's line sits at the top, mirroring Python Browser
// ._jump_to_anchor (Browser.py:324-357): the scroll target is _rows_above(
// target_idx, cols) — the count of wrapped display rows preceding the anchor
// line. We compute the expected offset independently with tview.WordWrap (the
// same word-wrap tview's TextView uses to draw), so the test pins the
// integration of anchor lookup + row-summing + ScrollTo, not the wrap
// algorithm itself.
func TestBrowserJumpToAnchorScrollOffset(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)

	// Give the content a known wrap width of 40 (no live screen needed: SetRect
	// sets the geometry GetInnerRect reports and that JumpToAnchor wraps to).
	bd.content.SetRect(0, 0, 40, 12)

	// Line 0: short (1 row). Line 1: long, wraps to >1 row at width 40.
	// Line 2: the anchor target. Line 3: trailing content.
	markup := "`:a alpha\nthe quick brown fox jumps over the lazy dog and keeps on running for a while\n`:target the target line\ntrailing"
	bd.RenderPage(markup)

	targetIdx, ok := bd.anchors.JumpTarget("target")
	if !ok {
		t.Fatal("anchor \"target\" not found in anchor map")
	}
	if targetIdx != 2 {
		t.Fatalf("target anchor index = %v, want 2", targetIdx)
	}

	// Expected scroll row = wrapped rows of lines 0..targetIdx-1 at width 40.
	const innerW = 40
	expected := 0
	for i, lt := range bd.lineTexts {
		if i >= targetIdx {
			break
		}
		rows := len(tview.WordWrap(lt, innerW))
		if rows < 1 {
			rows = 1
		}
		expected += rows
	}
	if expected <= 1 {
		t.Fatalf("expected wrapped rows preceding target = %v, want >1 (test must exercise wrapping)", expected)
	}

	bd.JumpToAnchor("target")
	row, _ := bd.content.GetScrollOffset()
	if row != expected {
		t.Errorf("JumpToAnchor scroll row = %v, want %v", row, expected)
	}
}

// TestBrowserJumpToAnchorUnknownIsNoOp asserts an unknown anchor name does not
// move the scroll position (Python: target_idx is None → return; the Go port
// has no browser-footer slot to mirror Python's "Unknown anchor: #name"
// message, so the faithful mapping is a no-op, matching GuideDisplay).
func TestBrowserJumpToAnchorUnknownIsNoOp(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 12)
	bd.RenderPage("`:a alpha\nsecond line")

	before, _ := bd.content.GetScrollOffset()
	bd.JumpToAnchor("does-not-exist")
	after, _ := bd.content.GetScrollOffset()
	if after != before {
		t.Errorf("scroll moved on unknown anchor: before=%v after=%v", before, after)
	}
}

// TestBrowserJumpToAnchorEmptyJumpsToNextHeader asserts an empty anchor name
// (a bare "#" link) scrolls to the first heading below the current scroll
// position, mirroring Python's _jump_to_anchor else-branch (Browser.py:337-348):
// it walks header_rows and jumps to the first whose _rows_above exceeds the
// current scrollpos. With no heading below the cursor it is a no-op.
func TestBrowserJumpToAnchorEmptyJumpsToNextHeader(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 40, 12)

	// A heading on line 2 (after enough wrapped rows above that scrolling is
	// observable). Heading text auto-generates a slug anchor.
	markup := "`:a alpha\nthe quick brown fox jumps over the lazy dog and keeps on running for a while\n> A Heading\nbody"
	bd.RenderPage(markup)

	// Find the heading line index.
	headingIdx := -1
	for i, sl := range bd.currentLines {
		if sl != nil && sl.HeadingLevel > 0 {
			headingIdx = i
			break
		}
	}
	if headingIdx < 0 {
		t.Fatal("no heading line found in rendered page")
	}

	// Expected scroll = wrapped rows preceding the heading line.
	const innerW = 40
	expected := 0
	for i, lt := range bd.lineTexts {
		if i >= headingIdx {
			break
		}
		rows := len(tview.WordWrap(lt, innerW))
		if rows < 1 {
			rows = 1
		}
		expected += rows
	}
	if expected <= 1 {
		t.Fatalf("expected wrapped rows preceding heading = %v, want >1 (test must exercise wrapping)", expected)
	}

	bd.JumpToAnchor("")
	row, _ := bd.content.GetScrollOffset()
	if row != expected {
		t.Errorf("JumpToAnchor(\"\") scroll row = %v, want %v (next header)", row, expected)
	}
}
