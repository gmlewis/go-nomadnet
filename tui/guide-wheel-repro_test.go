// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestGuideWheelAfterTopicSwitchNoJump reproduces and pins the fix for the
// reported bug:
//  1. click Guide topic A, scroll all the way down,
//  2. click topic B (resets scroll to the top),
//  3. the FIRST mouse-scroll-down JUMPS all the way to the bottom.
//
// Root cause: tview's TextView wheel handler bumps lineOffset by 1 and sets
// trackEnd=true when its lazy line index is short relative to the offset; the
// next Draw then jumps lineOffset to len(lineIndex)-height (the bottom). After
// a topic switch SetText resets the line index to nil and a Draw only rebuilds
// it to lineOffset+height+1, so the very first wheel notch sets trackEnd and
// the following Draw leaps to the bottom.
//
// The fix is the per-primitive wheel multiplier (installWheelCapture, wired in
// NewScrollBar): a notch scrolls mouseWheelLines rows in ONE delivery via
// ScrollTo, which sets trackEnd=false — so there is no premature trackEnd and
// no jump. This test drives the real ScrollBar→guideReader path (the capture is
// installed on the reader via the wheelScrollable interface).
func TestGuideWheelAfterTopicSwitchNoJump(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	const delta = 3
	SetMouseWheelLines(delta)

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	gd := NewGuideDisplay(app)

	const w, h = 80, 12
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(w, h)

	// Laying out the ScrollBar lays out the wrapped reader (its SetRect assigns
	// the reader width-1 and full height); drawing it builds the reader's line
	// index. The wheel capture runs against the reader, so fire wheel events
	// through the ScrollBar's MouseHandler (which delegates to the reader).
	gd.scroll.SetRect(0, 0, w, h)
	gd.scroll.Text.Focus(func(p tview.Primitive) {})

	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	handler := gd.scroll.MouseHandler()
	ev := func() *tcell.EventMouse {
		rx, ry, _, _ := gd.reader.GetRect()
		return tcell.NewEventMouse(rx+5, ry+2, tcell.ButtonNone, tcell.ModNone)
	}

	// 1. Topic A: render + draw so the line index builds at the top.
	gd.showTopic(0)
	gd.scroll.Draw(screen)
	screen.Sync()
	if row, _ := gd.reader.GetScrollOffset(); row != 0 {
		t.Fatalf("topic A not at top after showTopic(0): row=%v want 0", row)
	}

	// 2. Scroll to the bottom of topic A: fire wheel-down notches with a Draw
	// between each (the real event loop draws between notches) until the offset
	// stops advancing.
	bottom := -1
	for range 500 {
		handler(tview.MouseScrollDown, ev(), setFocus)
		gd.scroll.Draw(screen)
		row, _ := gd.reader.GetScrollOffset()
		if row == bottom {
			break
		}
		bottom = row
	}
	if bottom <= 0 {
		t.Skip("topic A does not overflow at this size; cannot exercise the jump")
	}
	t.Logf("topic A scrolled to bottom: lineOffset=%v", bottom)

	// 3. Switch to topic B (resets scroll to the top).
	gd.showTopic(1)
	gd.scroll.Draw(screen)
	screen.Sync()
	rowAfterSwitch, _ := gd.reader.GetScrollOffset()
	if rowAfterSwitch != 0 {
		t.Fatalf("showTopic(1) did not reset scroll: lineOffset=%v want 0", rowAfterSwitch)
	}

	// The bottom of topic B, from the reader's own line index (the same figure
	// the capture clamps against via GetWrappedLineCount).
	total := gd.reader.GetWrappedLineCount()
	_, _, _, viewportH := gd.reader.GetInnerRect()
	posmax := total - viewportH
	t.Logf("topic B: total=%v viewportH=%v posmax=%v", total, viewportH, posmax)
	if posmax < 2*delta {
		t.Skipf("topic B too short to distinguish a delta-step from the bottom (posmax=%v, delta=%v)", posmax, delta)
	}

	// 4. ONE wheel-down notch. Before the fix this jumped to the bottom; now it
	// must move exactly delta rows (trackEnd stays false).
	handler(tview.MouseScrollDown, ev(), setFocus)
	gd.scroll.Draw(screen)
	afterOneNotch, _ := gd.reader.GetScrollOffset()
	t.Logf("after ONE wheel-down notch: lineOffset=%v (delta=%v posmax=%v)", afterOneNotch, delta, posmax)

	if afterOneNotch >= posmax {
		t.Errorf("BUG REPRODUCED: one wheel-down notch jumped to the bottom (lineOffset=%v, posmax=%v) — want %v rows", afterOneNotch, posmax, delta)
	}
	if afterOneNotch != delta {
		t.Errorf("one wheel-down notch moved lineOffset to %v, want %v (delta)", afterOneNotch, delta)
	}

	// 5. A second notch moves another delta rows (cumulative 2*delta), still far
	// from the bottom — no jump on the second notch either.
	handler(tview.MouseScrollDown, ev(), setFocus)
	gd.scroll.Draw(screen)
	afterTwoNotches, _ := gd.reader.GetScrollOffset()
	t.Logf("after TWO wheel-down notches: lineOffset=%v", afterTwoNotches)
	if want := 2 * delta; afterTwoNotches != want {
		t.Errorf("two wheel-down notches moved lineOffset to %v, want %v", afterTwoNotches, want)
	}
}
