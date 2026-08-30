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
// page. Python's MenuColumns.keypress (Main.py:171-176) sets
// frame.focus_position="body" for Down and returns the key unhandled, and
// urwid's Frame dispatches by the entry-time focus part — so the key DIES at
// the main frame, but focus has already entered the body: the Interfaces list
// gains the terminal focus and its CURRENT item renders the focus marker
// (verified live on nomadnet 1.2.8: one Down lights up item 0, a SECOND Down
// moves to item 1). A single Down from the menu must therefore leave the list
// focused at its current item (0) WITHOUT moving the selection.
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

	// A single Down from the menu enters the body and drops the key (Python
	// MenuColumns.keypress Main.py:171-176 + urwid frame.py entry-time
	// dispatch): no event reaches any body widget.
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if ev := md.handleInput(down); ev != nil {
		t.Fatalf("handleInput(Down) from menu forwarded %v; Python drops the key (first Down only enters the body)", ev)
	}

	if md.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body after Down from menu", md.focusRegion)
	}
	// The Interfaces list must be the focused primitive (the focus marker
	// moves to the list's non-selectable header in Python), and the selection
	// must NOT have advanced.
	if got := app.GetFocus(); got != tview.Primitive(id.listBox) {
		t.Errorf("focus after Down from menu = %T, want the Interfaces list", got)
	}
	if got := id.SelectedIndex(); got != -1 {
		t.Errorf("after one Down from menu, SelectedIndex = %d, want -1 (Python's list focus starts on the non-selectable header; the dropped key must not advance the list)", got)
	}

	// The SECOND Down now navigates the focused list (Python: focus moves from
	// the header onto item 0, the ● glyph appears).
	if h := md.Root().InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	}
	if got := id.SelectedIndex(); got != 0 {
		t.Errorf("after two Downs, SelectedIndex = %d, want 0 (the second Down advances the list)", got)
	}
}
