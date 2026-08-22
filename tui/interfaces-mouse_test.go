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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestInterfacesListMouseClickFocusesItem is the regression test for the mouse
// parity gap on the [ Interfaces ] page. Python's urwid ListBox focuses the
// clicked interface item (○→●) on a left click (SelectableInterfaceItem has no
// mouse_event, so the ListBox owns click-to-focus). The Go interfaceListBox
// previously had no MouseHandler, so clicks on interface items did nothing.
// This test verifies a left click on an item focuses it and moves tview focus
// onto the list so subsequent arrow keys navigate it.
func TestInterfacesListMouseClickFocusesItem(t *testing.T) {
	t.Parallel()
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
	defer screen.Fini()
	screen.SetSize(80, 24)
	id.listBox.SetRect(0, 0, 80, 24)
	id.listBox.Draw(screen)

	if got := id.SelectedIndex(); got != -1 {
		t.Fatalf("SelectedIndex before click = %d, want -1", got)
	}

	mh := id.listBox.MouseHandler()
	var focused tview.Primitive
	setFocus := func(p tview.Primitive) { app.SetFocus(p); focused = p }

	// Click on the second item (index 1). With InterfaceItemHeight=7 and offset
	// 0, item 1 occupies rows [7,14); click at y=9 (well inside item 1).
	ev := tcell.NewEventMouse(5, 9, tcell.Button1, tcell.ModNone)
	consumed, _ := mh(tview.MouseLeftClick, ev, setFocus)
	if !consumed {
		t.Fatal("MouseHandler(MouseLeftClick) did not consume the click")
	}
	if got := id.SelectedIndex(); got != 1 {
		t.Errorf("after clicking item 1, SelectedIndex = %d, want 1 (Python focuses the clicked item)", got)
	}
	if focused != id.listBox {
		t.Errorf("after click, focused = %T, want *interfaceListBox (list must gain focus for arrow keys)", focused)
	}

	// Click on the first item (rows [0,7)); click at y=2.
	ev0 := tcell.NewEventMouse(5, 2, tcell.Button1, tcell.ModNone)
	mh(tview.MouseLeftClick, ev0, setFocus)
	if got := id.SelectedIndex(); got != 0 {
		t.Errorf("after clicking item 0, SelectedIndex = %d, want 0", got)
	}
}
