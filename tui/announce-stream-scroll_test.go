// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestAnnounceStreamMouseWheelScroll verifies that a mouse-wheel scroll over
// the Announce Stream list actually scrolls the list (tview.List itemOffset).
// Python's AnnounceStream is a urwid ListBox, which scrolls on the wheel by
// default; the Go port wraps a tview.List in an IndicativeListBox inside a
// pileFiller, so the wheel must reach the list through the pileFiller's
// MouseHandler (which forwards to every child, not just the focused one).
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

	// Render small enough that not all 40 rows fit, so scrolling is meaningful.
	w, h := 50, 8
	root := as.Widget()
	root.SetRect(0, 0, w, h)
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(w, h)
	screen.Init()
	root.Draw(screen)
	screen.Show()

	// Layout: tab bar (rows 0-1), filter bar (row 2), list (rows 3-7).
	// Scroll over the middle of the list area.
	scrollY := 5
	scrollX := 25

	off0, _ := as.ilb.List.GetOffset()
	t.Logf("itemOffset before scroll = %v", off0)

	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	ev := tcell.NewEventMouse(scrollX, scrollY, tcell.ButtonNone, tcell.ModNone)
	if mh := root.MouseHandler(); mh != nil {
		mh(tview.MouseScrollDown, ev, setFocus)
	}

	off1, _ := as.ilb.List.GetOffset()
	t.Logf("itemOffset after MouseScrollDown = %v", off1)
	if off1 <= off0 {
		t.Errorf("MouseScrollDown did not advance the list (itemOffset %v -> %v) — the wheel event is not reaching the tview.List through the pileFiller", off0, off1)
	}

	// Scroll back up.
	if mh := root.MouseHandler(); mh != nil {
		mh(tview.MouseScrollUp, ev, setFocus)
	}
	off2, _ := as.ilb.List.GetOffset()
	if off2 != off0 {
		t.Errorf("MouseScrollUp did not return to %v (got %v)", off0, off2)
	}
}