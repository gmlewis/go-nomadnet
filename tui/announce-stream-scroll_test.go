// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestAnnounceStreamMouseWheelScroll verifies that a mouse-wheel scroll over
// the Announce Stream list moves the highlight (tview.List currentItem) and that
// the viewport (itemOffset) follows it after a Draw. Python's AnnounceStream is
// a urwid ListBox, which scrolls on the wheel by default; the Go port wraps a
// tview.List in an IndicativeListBox inside a pileFiller, so the wheel must
// reach the list through the pileFiller's MouseHandler (which forwards to every
// child, not just the focused one). The IndicativeListBox wheel fix translates
// the wheel into a SetCurrentItem jump (arrow-key semantics) — the highlight
// follows the wheel and the viewport follows the highlight via the Draw
// keep-current-visible clamp, instead of tview's default itemOffset-only move
// that the clamp cancels at the viewport edges.
func TestAnnounceStreamMouseWheelScroll(t *testing.T) {
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	now := time.Now()
	announces := make([]AnnounceEntry, 40)
	for i := range announces {
		announces[i] = AnnounceEntry{
			DisplayName: "Node", Type: "node", SourceHash: "aaaa", Timestamp: now, AppData: "Node",
		}
	}
	nd := NewNetworkDisplay(app, announces, nil)
	nd.toggleList() // showingNodes -> Announce Stream
	as := nd.announceStream

	if got := as.ilb.List.GetItemCount(); got != 40 {
		t.Fatalf("list has %v items, want 40", got)
	}

	// Pin the rows-per-notch multiplier so the highlight jump is deterministic.
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	SetMouseWheelLines(3)

	// Render small enough that not all 40 rows fit, so scrolling is meaningful.
	// The IndicativeListBox applies the multiplier itself (SetCurrentItem jumps
	// mouseWheelLines rows in one delivery), so the bare widget is the real path
	// the app uses — no root wrapper is needed.
	w, h := 50, 8
	root := as.Widget()
	root.SetRect(0, 0, w, h)
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(w, h)
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	root.Draw(screen)
	screen.Show()

	// Layout: tab bar (rows 0-1), filter bar (row 2), list (rows 3-7).
	// Scroll over the middle of the list area.
	scrollY := 5
	scrollX := 25

	cur0 := as.ilb.List.GetCurrentItem()
	off0, _ := as.ilb.List.GetOffset()
	t.Logf("before scroll: currentItem=%v itemOffset=%v", cur0, off0)

	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	ev := tcell.NewEventMouse(scrollX, scrollY, tcell.ButtonNone, tcell.ModNone)
	if mh := root.MouseHandler(); mh != nil {
		if consumed, _ := mh(tview.MouseScrollDown, ev, setFocus); !consumed {
			t.Error("MouseScrollDown: consumed=false, want true")
		}
	}
	// The real event loop draws because the handler returns consumed=true; the
	// Draw keep-current-visible clamp then moves itemOffset to follow the
	// highlight. Draw explicitly here so itemOffset settles.
	root.Draw(screen)

	cur1 := as.ilb.List.GetCurrentItem()
	off1, _ := as.ilb.List.GetOffset()
	t.Logf("after MouseScrollDown + Draw: currentItem=%v itemOffset=%v", cur1, off1)
	if cur1 <= cur0 {
		t.Errorf("MouseScrollDown did not advance the highlight (currentItem %v -> %v) — the wheel event is not reaching the tview.List through the pileFiller", cur0, cur1)
	}
	if off1 <= off0 {
		t.Errorf("MouseScrollDown did not advance the viewport (itemOffset %v -> %v) after Draw", off0, off1)
	}

	// Scroll back up: the highlight and viewport return to the start.
	if mh := root.MouseHandler(); mh != nil {
		mh(tview.MouseScrollUp, ev, setFocus)
	}
	root.Draw(screen)
	cur2 := as.ilb.List.GetCurrentItem()
	off2, _ := as.ilb.List.GetOffset()
	if cur2 != cur0 {
		t.Errorf("MouseScrollUp did not return the highlight to %v (got %v)", cur0, cur2)
	}
	if off2 != off0 {
		t.Errorf("MouseScrollUp did not return the viewport to %v (got %v)", off0, off2)
	}
}
