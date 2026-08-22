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

// TestInterfacesPageDownFromMenuFocusesFirstItem is the regression test for the
// reported "cursor movement via arrow keys does not work" bug on the [ Interfaces ]
// page. Python's MenuColumns.keypress (Main.py:172-176) sets
// frame.focus_position="body" for Down and returns the key unhandled; urwid.Frame
// then re-dispatches that same Down to the body, so a SINGLE Down from the menu
// both enters the body AND advances the Interfaces list to item 0 (the selection
// glyph goes ○→●). The Go port previously consumed the Down in handleMenuInput
// (returned nil), so the body never saw it: the first Down did nothing visible and
// the user had to press Down a second time to move the cursor — matching the
// "arrow keys do not work" report.
//
// This test mirrors the production key path: the app-level input capture
// (MainDisplay.handleInput) runs first, then the root primitive InputHandler
// dispatches the forwarded event to the focused Interfaces list. After one Down
// from the menu, SelectedIndex must be 0 (item 0 focused), matching Python.
func TestInterfacesPageDownFromMenuFocusesFirstItem(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	app.Main = md
	app.SetRoot()

	ifaces := []InterfaceInfo{
		{Name: "RNode1", Type: "RNodeInterface", Status: "connected", Connected: true, Enabled: true},
		{Name: "TCP1", Type: "TCPClientInterface", Status: "disconnected", Connected: false, Enabled: true},
		{Name: "Auto1", Type: "AutoInterface", Status: "connected", Connected: true, Enabled: true},
	}
	id := NewInterfacesDisplay(app, ifaces)
	md.SetDisplay("interfaces", id.Widget())

	// Select the Interfaces page and put focus in the menu bar, exactly as a
	// user who navigated to the Interfaces menu button would be.
	md.SelectPage("interfaces")
	md.FocusMenu()
	if md.focusRegion != "menu" {
		t.Fatalf("focusRegion = %q, want menu before Down", md.focusRegion)
	}

	set := func(p tview.Primitive) { app.SetFocus(p) }
	root := md.Root()

	// Send a single Down through the production key path: the app-level input
	// capture (handleInput) first, then the root InputHandler for the forwarded
	// event.
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if ev := md.handleInput(down); ev != nil {
		down = ev
	} else {
		t.Fatal("handleInput(Down) from menu returned nil; want the event forwarded to the body")
	}
	if h := root.InputHandler(); h != nil {
		h(down, set)
	}

	if md.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body after Down from menu", md.focusRegion)
	}
	if got := id.SelectedIndex(); got != 0 {
		t.Errorf("after one Down from menu, SelectedIndex = %d, want 0 (Python focuses item 0 on the single Down that enters the body)", got)
	}
}
