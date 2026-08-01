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
	"testing"

	"github.com/rivo/tview"
)

// TestJumpToAnchorScrollOffset asserts jumpToAnchor scrolls the reader so the
// anchor's line sits at the top, mirroring Python's Guide.jump_to_anchor
// (Guide.py:236-261): the scroll target is _rows_above(attrmaps, target_idx,
// cols) — the count of wrapped display rows preceding the anchor line. We
// compute the expected offset independently with tview.WordWrap (the same
// word-wrap tview's TextView uses to draw), so the test pins the integration
// of anchor lookup + row-summing + ScrollTo, not the wrap algorithm itself.
func TestJumpToAnchorScrollOffset(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)

	// Give the reader a known wrap width of 40 (no live screen needed: SetRect
	// sets the geometry GetInnerRect reports and that jumpToAnchor wraps to).
	gd.reader.SetRect(0, 0, 40, 12)

	// Line 0: short (1 row). Line 1: long, wraps to >1 row at width 40.
	// Line 2: the anchor target. Line 3: trailing content.
	markup := "`:a alpha\nthe quick brown fox jumps over the lazy dog and keeps on running for a while\n`:target the target line\ntrailing"
	gd.showMarkupForTest(markup)

	targetIdx, ok := gd.anchors.JumpTarget("target")
	if !ok {
		t.Fatal("anchor \"target\" not found in anchor map")
	}
	if targetIdx != 2 {
		t.Fatalf("target anchor index = %v, want 2", targetIdx)
	}

	// Expected scroll row = wrapped rows of lines 0..targetIdx-1 at width 40.
	const innerW = 40
	expected := 0
	for i, lt := range gd.lineTexts {
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

	gd.jumpToAnchor("target")
	row, _ := gd.reader.GetScrollOffset()
	if row != expected {
		t.Errorf("jumpToAnchor scroll row = %v, want %v", row, expected)
	}
}

// TestJumpToAnchorUnknownIsNoOp asserts an unknown anchor name does not move the
// scroll position (Python: target_idx is None → return).
func TestJumpToAnchorUnknownIsNoOp(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 40, 12)
	gd.showMarkupForTest("`:a alpha\nsecond line")

	before, _ := gd.reader.GetScrollOffset()
	gd.jumpToAnchor("does-not-exist")
	after, _ := gd.reader.GetScrollOffset()
	if after != before {
		t.Errorf("scroll moved on unknown anchor: before=%v after=%v", before, after)
	}
}

// TestGuideHandleLinkAnchorJump asserts handleLink dispatches a "#name" URL to
// jumpToAnchor (in-page), matching Python GuideLinkDelegate.handle_link
// (Guide.py:103-113), and does NOT invoke the external OnHandleLink callback.
func TestGuideHandleLinkAnchorJump(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 40, 12)
	gd.showMarkupForTest("`:a alpha\n`:target the target line")

	externalCalled := false
	gd.OnHandleLink = func(target, fields string) { externalCalled = true }

	gd.handleLink("#target", "")

	row, _ := gd.reader.GetScrollOffset()
	if row == 0 {
		t.Error("handleLink(#target) did not scroll the reader (jumpToAnchor not invoked)")
	}
	if externalCalled {
		t.Error("handleLink(#target) invoked external OnHandleLink; anchor jumps are in-page")
	}
}

// TestGuideHandleLinkExternal asserts a non-anchor URL is forwarded to
// OnHandleLink (Python: show_network + browser.handle_link), not treated as an
// in-page jump.
func TestGuideHandleLinkExternal(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 40, 12)
	gd.showMarkupForTest("`:a alpha\n`:target the target line")

	var gotTarget, gotFields string
	gd.OnHandleLink = func(target, fields string) {
		gotTarget, gotFields = target, fields
	}

	beforeRow, _ := gd.reader.GetScrollOffset()
	gd.handleLink("a8d24177d946de4f1f0a0fe1af9a1338:/page/index.mu", "fields-here")

	if gotTarget != "a8d24177d946de4f1f0a0fe1af9a1338:/page/index.mu" {
		t.Errorf("external target = %q, want the URL", gotTarget)
	}
	if gotFields != "fields-here" {
		t.Errorf("external fields = %q, want %q", gotFields, "fields-here")
	}
	afterRow, _ := gd.reader.GetScrollOffset()
	if afterRow != beforeRow {
		t.Errorf("external link moved scroll (before=%v after=%v); want no in-page jump", beforeRow, afterRow)
	}
}

// TestGuideHandleLinkEmpty asserts an empty target is a no-op (Python:
// `if not target: return`).
func TestGuideHandleLinkEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.showMarkupForTest("`:a alpha")
	called := false
	gd.OnHandleLink = func(target, fields string) { called = true }

	gd.handleLink("", "")
	if called {
		t.Error("handleLink(\"\") invoked OnHandleLink; want no-op")
	}
}
