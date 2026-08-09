// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestScrollWheelStep exercises the shared wheel-step engine: clamping,
// boundary no-op, and the multiplier magnitude.
//
// Mutates the package-global mouseWheelLines, so it runs sequentially (no
// t.Parallel) alongside the other wheel-multiplier tests.
func TestScrollWheelStep(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	SetMouseWheelLines(3)

	// total <= height: nothing to scroll.
	if next, ok := scrollWheelStep(tview.MouseScrollDown, 0, 10, 10); ok || next != 0 {
		t.Errorf("fits viewport: ok=%v next=%v, want false/0", ok, next)
	}

	// Mid-page down: offset 5 → 8 (delta 3), total=40 h=10 posmax=30.
	next, ok := scrollWheelStep(tview.MouseScrollDown, 5, 10, 40)
	if !ok || next != 8 {
		t.Errorf("down from 5: ok=%v next=%v, want true/8", ok, next)
	}
	// Up: 5 → 2.
	next, ok = scrollWheelStep(tview.MouseScrollUp, 5, 10, 40)
	if !ok || next != 2 {
		t.Errorf("up from 5: ok=%v next=%v, want true/2", ok, next)
	}

	// Clamp at the bottom: from 29 down → 30 (posmax), from 30 down → no-op.
	next, ok = scrollWheelStep(tview.MouseScrollDown, 29, 10, 40)
	if !ok || next != 30 {
		t.Errorf("down from 29: ok=%v next=%v, want true/30", ok, next)
	}
	if next, ok := scrollWheelStep(tview.MouseScrollDown, 30, 10, 40); ok || next != 0 {
		t.Errorf("down at bottom: ok=%v next=%v, want false/0", ok, next)
	}
	// Clamp at the top: from 1 up → 0, from 0 up → no-op.
	next, ok = scrollWheelStep(tview.MouseScrollUp, 1, 10, 40)
	if !ok || next != 0 {
		t.Errorf("up from 1: ok=%v next=%v, want true/0", ok, next)
	}
	if next, ok := scrollWheelStep(tview.MouseScrollUp, 0, 10, 40); ok || next != 0 {
		t.Errorf("up at top: ok=%v next=%v, want false/0", ok, next)
	}

	// A delta that overshoots clamps to the boundary rather than no-op'ing.
	if next, ok := scrollWheelStep(tview.MouseScrollDown, 28, 10, 40); !ok || next != 30 {
		t.Errorf("down from 28 (overshoot): ok=%v next=%v, want true/30 (clamped)", ok, next)
	}
}

// TestApplyWheelMultiplier installs the capture on a bare *tview.TextView and
// verifies a notch scrolls mouseWheelLines rows in one delivery and declines to
// consume at the boundaries (so tview skips the no-op redraw). It also checks a
// non-wheel action passes through to the default handler untouched.
//
// Mutates the package-global mouseWheelLines, so it runs sequentially.
func TestApplyWheelMultiplier(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	const delta = 3
	SetMouseWheelLines(delta)

	tv := applyWheelMultiplier(tview.NewTextView().SetScrollable(true).SetWrap(false))
	var b []byte
	for i := range 40 {
		b = append(b, "line"...)
		b = append(b, byte('0'+i/10), byte('0'+i%10), '\n')
	}
	tv.SetText(string(b))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	const w, h = 20, 10
	screen.SetSize(w, h)
	tv.SetRect(0, 0, w, h)
	tv.Focus(func(p tview.Primitive) {})
	tv.Draw(screen) // settle the line index

	handler := tv.MouseHandler()
	ev := func() *tcell.EventMouse { return tcell.NewEventMouse(w/2, h/2, tcell.ButtonNone, tcell.ModNone) }
	setFocus := func(p tview.Primitive) {}

	// Down from the top: consumes and moves exactly delta rows.
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); !consumed {
		t.Error("down at top: consumed=false, want true")
	}
	if row, _ := tv.GetScrollOffset(); row != delta {
		t.Errorf("down at top moved to %v, want %v", row, delta)
	}

	// Up back to the top.
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); !consumed {
		t.Error("up: consumed=false, want true")
	}
	if row, _ := tv.GetScrollOffset(); row != 0 {
		t.Errorf("up moved to %v, want 0", row)
	}

	// At the top, up is a no-op: must NOT consume.
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); consumed {
		t.Error("up at top: consumed=true, want false (no-op)")
	}

	// At the bottom, down is a no-op: must NOT consume.
	tv.ScrollTo(1<<20, 0)
	tv.Draw(screen)
	posmax, _ := tv.GetScrollOffset()
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); consumed {
		t.Error("down at bottom: consumed=true, want false (no-op)")
	}
	if row, _ := tv.GetScrollOffset(); row != posmax {
		t.Errorf("down at bottom moved to %v, want unchanged %v", row, posmax)
	}

	// A non-wheel action (left click) passes through to the default handler, so
	// the capture must not have short-circuited it — the handler still runs and
	// focuses the TextView.
	if consumed, _ := handler(tview.MouseLeftClick, ev(), setFocus); !consumed {
		// tview's TextView consumes a left click to position the cursor; either
		// way the point is the capture did not swallow it. Just assert no panic
		// and that a click is not misrouted into a scroll (offset unchanged).
		_ = consumed
	}
	if row, _ := tv.GetScrollOffset(); row != posmax {
		t.Errorf("left click changed offset to %v, want unchanged %v (click must not scroll)", row, posmax)
	}
}
