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

// TestAnnounceStreamTabReachesList verifies that pressing Tab in the
// Announce Stream view cycles focus through the pileFiller's items
// (tab bar → filter bar → IndicativeListBox) and does NOT escape to
// the browser pane. In Python nomadnet, the AnnounceStream Pile handles
// Tab internally (urwid Pile.keypress moves focus_position), so the
// enclosing Columns never sees Tab. In the Go port, the urwidColumns
// (mainCols) was intercepting Tab before the pileFiller could handle
// it, moving column focus to the browser instead of cycling the pile.
func TestAnnounceStreamTabReachesList(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "aaaa", DisplayName: "Node A"},
		{SourceHash: "bbbb", DisplayName: "Node B"},
	})
	app.Main.SetDisplay("network", nd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)

	// Switch to the Announce Stream view (Ctrl-L toggles from Saved Nodes).
	app.Main.SelectPage("network")
	app.Main.FocusBody()
	nd.toggleList()
	app.Main.Root().Draw(screen)

	as := nd.announceStream
	if as == nil {
		t.Fatal("announceStream is nil")
	}
	if as.pile == nil {
		t.Fatal("announceStream.pile is nil")
	}

	// Ensure the pileFiller has focus (tab bar, index 0).
	app.SetFocus(as.pile)
	app.Main.Root().Draw(screen)
	if as.pile.focusIndex != 0 {
		t.Fatalf("initial pileFocusIndex=%d, want 0 (tab bar)", as.pile.focusIndex)
	}

	// Dispatch Tab through the mainCols InputHandler (the real event
	// dispatch path: root → bodyPages → mainCols).
	handler := nd.mainCols.InputHandler()
	if handler == nil {
		t.Fatal("mainCols has no InputHandler")
	}
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	// Tab #1: tab bar → filter bar (pileFocusIndex 0 → 1).
	handler(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), setFocus)
	app.Main.Root().Draw(screen)
	if as.pile.focusIndex != 1 {
		t.Errorf("after Tab#1: pileFocusIndex=%d, want 1 (filter bar)", as.pile.focusIndex)
	}

	// Tab #2: filter bar → IndicativeListBox (pileFocusIndex 1 → 2).
	handler(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), setFocus)
	app.Main.Root().Draw(screen)
	if as.pile.focusIndex != 2 {
		t.Errorf("after Tab#2: pileFocusIndex=%d, want 2 (IndicativeListBox)", as.pile.focusIndex)
	}

	// Verify the IndicativeListBox is the pile's focused item (so
	// keyboard navigation and the pile's Up-at-top→menu escape work).
	fi := as.pile.focusedItem()
	if fi == nil {
		t.Fatal("pileFiller has no focused item after Tab#2")
	}
	if _, ok := fi.(*IndicativeListBox); !ok {
		t.Errorf("pileFiller focused item=%T, want *IndicativeListBox", fi)
	}
}
