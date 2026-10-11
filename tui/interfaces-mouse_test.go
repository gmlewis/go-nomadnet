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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// newInterfacesMouseTestDisplay builds an InterfacesDisplay with three
// interfaces drawn at 80x24 (InterfaceItemHeight=7 per row).
func newInterfacesMouseTestDisplay(t *testing.T) (*InterfacesDisplay, *App) {
	t.Helper()
	app := newTestApp()
	id := NewInterfacesDisplay(app, []InterfaceInfo{
		{Name: "RNode1", Type: "RNodeInterface", Status: "connected", Connected: true, Enabled: true},
		{Name: "TCP1", Type: "TCPClientInterface", Status: "disconnected", Connected: false, Enabled: true},
		{Name: "Auto1", Type: "AutoInterface", Status: "connected", Connected: true, Enabled: true},
	})
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 24)
	id.listBox.SetRect(0, 0, 80, 24)
	id.listBox.Draw(screen)
	return id, app
}

// TestInterfacesListMouseClickFocusesItem pins the click-to-focus half of the
// mouse behavior on the [ Interfaces ] page. Python's urwid ListBox focuses the
// clicked interface item (○→●) on a left click (SelectableInterfaceItem has no
// mouse_event, so the ListBox owns click-to-focus). A click on a row that is
// NOT already focused only focuses it — it must not open the details view — so
// a stray click never navigates away.
func TestInterfacesListMouseClickFocusesItem(t *testing.T) {
	t.Parallel()
	id, app := newInterfacesMouseTestDisplay(t)

	if got := id.SelectedIndex(); got != -1 {
		t.Fatalf("SelectedIndex before click = %v, want -1", got)
	}

	var activated []int
	id.OnShowInterface = func(idx int) { activated = append(activated, idx) }

	mh := id.listBox.MouseHandler()
	var focused tview.Primitive
	setFocus := func(p tview.Primitive) { app.SetFocus(p); focused = p }

	// Click on the second item (index 1). Item 1 occupies rows [7,14); click at
	// y=9 (well inside item 1).
	ev := tcell.NewEventMouse(5, 9, tcell.Button1, tcell.ModNone)
	consumed, _ := mh(tview.MouseLeftClick, ev, setFocus)
	if !consumed {
		t.Fatal("MouseHandler(MouseLeftClick) did not consume the click")
	}
	if got := id.SelectedIndex(); got != 1 {
		t.Errorf("after clicking item 1, SelectedIndex = %v, want 1 (Python focuses the clicked item)", got)
	}
	if focused != id.listBox {
		t.Errorf("after click, focused = %T, want *interfaceListBox (list must gain focus for arrow keys)", focused)
	}
	if len(activated) != 0 {
		t.Errorf("clicking a not-yet-focused row activated %v; want focus only (no navigation)", activated)
	}

	// Click on the first item (rows [0,7)); click at y=2.
	ev0 := tcell.NewEventMouse(5, 2, tcell.Button1, tcell.ModNone)
	mh(tview.MouseLeftClick, ev0, setFocus)
	if got := id.SelectedIndex(); got != 0 {
		t.Errorf("after clicking item 0, SelectedIndex = %v, want 0", got)
	}
	if len(activated) != 0 {
		t.Errorf("clicking a not-yet-focused row activated %v; want focus only", activated)
	}
}

// TestInterfacesListMouseClickOnFocusedRowOpensDetails pins the requested
// enhancement (operator, 2026-10-10): a left click on a row opens that
// interface's details view — the same view Enter opens. This is an ADDITION,
// not a parity fix: Python's SelectableInterfaceItem has no mouse_event and
// Interfaces.py has no mouse handler, so Python has no click-to-open path at
// all. To keep a stray click from navigating away, the click opens the details
// only when the row ALREADY holds the focus; a click that changes the focus
// just focuses (see TestInterfacesListMouseClickFocusesItem).
func TestInterfacesListMouseClickOnFocusedRowOpensDetails(t *testing.T) {
	t.Parallel()
	id, app := newInterfacesMouseTestDisplay(t)

	var activated []int
	id.OnShowInterface = func(idx int) { activated = append(activated, idx) }

	mh := id.listBox.MouseHandler()
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	// First click on item 1: focuses it.
	ev1 := tcell.NewEventMouse(5, 9, tcell.Button1, tcell.ModNone)
	mh(tview.MouseLeftClick, ev1, setFocus)
	if got := id.SelectedIndex(); got != 1 {
		t.Fatalf("first click SelectedIndex = %v, want 1", got)
	}
	if len(activated) != 0 {
		t.Fatalf("first click activated %v; want focus only", activated)
	}

	// Second click on the SAME row: opens its details view.
	mh(tview.MouseLeftClick, ev1, setFocus)
	if len(activated) != 1 || activated[0] != 1 {
		t.Fatalf("second click on the focused row activated %v, want [1]", activated)
	}

	// A click on a different row (item 2, rows [14,21); y=16) again only
	// focuses — it must not open item 2's details on the first click.
	activated = nil
	ev2 := tcell.NewEventMouse(5, 16, tcell.Button1, tcell.ModNone)
	mh(tview.MouseLeftClick, ev2, setFocus)
	if got := id.SelectedIndex(); got != 2 {
		t.Errorf("click on item 2 SelectedIndex = %v, want 2", got)
	}
	if len(activated) != 0 {
		t.Errorf("click on a not-yet-focused row activated %v; want focus only", activated)
	}
}
