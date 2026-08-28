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

// TestD3NetworkReentryFocusesList pins D3: re-entering the Network page from
// the menubar with a browser still open must put focus on the saved-nodes
// LIST (Python keeps left-list focus; the live sweep observed the Go port
// dropping focus into *tui.browserPageView where every arrow is dead until
// Tab). Esc exiting an overlay (Announce Info → HandleEsc →
// showAnnounceStream) must land on the list as well.
func TestD3NetworkReentryFocusesList(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "d3aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DisplayName: "Node A"},
	})
	app.Main.SetDisplay("network", nd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("network")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	// Simulate browsing: the in-pane browser has a loaded page and holds focus
	// (the browser's part-cursor model focuses the page view after navigation).
	bd := nd.BrowserDisplay()
	bd.LoadURL("d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3:/page/index.mu")
	app.SetFocus(bd.content)
	app.Main.Root().Draw(screen)

	// To the menu bar and back to the body (menubar re-entry of the page).
	app.Main.FocusMenu()
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	focus := app.GetFocus()
	if _, ok := focus.(*browserPageView); ok {
		t.Fatalf("focus after page re-entry = *tui.browserPageView (stranded, D3); want the saved-nodes list")
	}
	if !inSubtree(focus, nd.nodesView()) && focus != tview.Primitive(nd.listBox) && focus != tview.Primitive(nd.leftPanel) {
		t.Errorf("focus after page re-entry = %T, want something in the saved-nodes list subtree", focus)
	}

	// Esc from the Announce Info overlay returns to the list with focus.
	nd.UpdateAnnounces([]AnnounceEntry{{
		DisplayName: "D3 Node", SourceHash: "d3bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Type: "node",
	}})
	nd.toggleList()
	// Walk to the stream list (two Downs) — the only way the user can Enter an
	// announce — then open the info view (Enter semantics).
	asPile := nd.announceStream.pile
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	asPile.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), setFocus)
	asPile.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), setFocus)
	nd.showAnnounceDetailFor(nd.announceStream.entries[0])
	if !nd.inInfoView {
		t.Fatal("setup: announce info should be open")
	}
	if !nd.HandleEsc() {
		t.Fatal("HandleEsc returned false with the info view open")
	}
	// After Esc the effective focus must be the announce stream LIST (Python:
	// left_pile.contents[0] = announce_stream_display and the Pile's
	// focus_position is the stream's ListBox — NOT the tab bar).
	if as := nd.announceStream; as == nil || as.pile == nil {
		t.Fatal("announce stream missing after Esc")
	} else if as.pile.focusIndex != 2 {
		t.Errorf("after Esc the announce stream pile focusIndex = %v, want 2 (the stream list)", as.pile.focusIndex)
	}
	switch f := app.GetFocus().(type) {
	case *pileFiller:
		if f != nd.announceStream.pile {
			t.Errorf("focus after Esc = a different pileFiller; want the announce stream pile")
		}
	case *browserPageView:
		t.Error("focus after Esc stranded on *tui.browserPageView (D3)")
	}
}

// inSubtree reports whether target's focus chain reaches p — approximated by
// pointer identity for the list primitives the D3 assertions care about.
func inSubtree(target tview.Primitive, p tview.Primitive) bool {
	if target == nil || p == nil {
		return false
	}
	if target == p {
		return true
	}
	switch v := p.(type) {
	case *tview.Flex:
		for i := 0; i < v.GetItemCount(); i++ {
			if inSubtree(target, v.GetItem(i)) {
				return true
			}
		}
	case *pileFiller:
		for _, it := range v.items {
			if inSubtree(target, it.widget) {
				return true
			}
		}
	case *urwidColumns:
		for _, child := range v.children {
			if inSubtree(target, child) {
				return true
			}
		}
	case *IndicativeListBox:
		return inSubtree(target, v.List)
	}
	return false
}

var _ = tcell.KeyEnter
