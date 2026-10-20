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
)

// The keyboard route to the Local Peer panel's "Node Info" button, following
// urwid 4.0.3 (the version the Python original runs under). Python's Network
// left pane is
//
//	left_pile = Pile([(WEIGHT, 1, known_nodes), (PACK, local_peer)])
//
// and urwid's Pile.keypress moves focus_position only on UP/DOWN (Columns only
// on LEFT/RIGHT — Tab is forwarded and handled by no container), so with the
// saved-nodes list focused the route is:
//
//	Down  × (rows-1)   list cursor walks to the last row
//	Down               list returns the key at its last row -> Pile moves
//	                   focus_position to contents[1] = LocalPeer, whose own
//	                   Pile defaults focus_position to its first selectable
//	                   widget = the Name ReadlineEdit
//	Down               Name edit -> Announce Now button
//	Down               Announce Now -> the Save | Node Info Columns (Save)
//	Right              Columns moves Save -> Node Info
//	Enter              Node Info activates node_info_query
//
// Up reverses the walk, and Up at the panel's first selectable row hands focus
// back to the list (urwid Pile.keypress candidates run off the top).
//
// The Go port must reproduce this through the real dispatch path (app capture →
// root → frame → bodyPages → mainCols), which is what dispatchApp drives.
func TestNetworkNodeInfoButtonKeyboardReachable(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6", DisplayName: "Saved A"},
		{SourceHash: "b1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d7", DisplayName: "Saved B"},
	})
	var opened int
	nd.SetLocalPeerHandlers(nil, nil, func() { opened++ })

	md := app.Main
	md.SetDisplay("network", nd.Widget())
	md.SelectPage("network")
	md.Root().SetRect(0, 0, 135, 32)
	app.SetRoot()
	nd.FocusLists()

	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	right := tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)

	// Down #1 walks the saved-nodes cursor (0 -> 1) and leaves the panel alone.
	dispatchApp(t, app, down)
	if got := nd.nodes.GetCurrentItem(); got != 1 {
		t.Fatalf("after Down the saved-nodes cursor = %v, want 1 (still in the list)", got)
	}
	if nd.localPeer.Widget().HasFocus() {
		t.Fatalf("the Local Peer panel took focus before the list reached its last row (focus=%T)", app.GetFocus())
	}

	// Down out of the list's last row hands focus to the panel's Name edit.
	dispatchApp(t, app, down)
	if !nd.localPeer.NameEditHasFocus() {
		t.Fatalf("after Down out of the list, focus = %T, want the Name edit", app.GetFocus())
	}

	// Down -> Announce Now.
	dispatchApp(t, app, down)
	if !nd.localPeer.AnnounceButtonHasFocus() {
		t.Fatalf("after Down from the Name edit, focus = %T, want the Announce Now button", app.GetFocus())
	}

	// Down -> the Save | Node Info button row, on Save.
	dispatchApp(t, app, down)
	if !nd.localPeer.SaveButtonHasFocus() {
		t.Fatalf("after Down from Announce Now, focus = %T, want the Save button", app.GetFocus())
	}

	// Right moves across the button row to Node Info without leaving the pane.
	dispatchApp(t, app, right)
	if !nd.localPeer.NodeInfoButtonHasFocus() {
		t.Fatalf("after Right, focus = %T, want the Node Info button", app.GetFocus())
	}
	if got := nd.mainCols.FocusIndex(); got != 0 {
		t.Errorf("Right left the left pane for the browser pane (mainCols focus = %v, want 0)", got)
	}

	// Enter opens the Local Node Info view.
	dispatchApp(t, app, enter)
	if opened != 1 {
		t.Errorf("Enter on the Node Info button fired OnNodeInfo %v times, want 1", opened)
	}
}

// TestAnnounceStreamDownEntersPanel pins the nested-pile half of the same
// urwid rule: with the Announce Stream shown (its own Pile of tab bar / filter
// bar / list), Down walks tab bar → filter bar → list, the list keeps Down
// while its cursor can still move, and at the list's last row the key bubbles
// out of the inner Pile into the left pane's next item — the Local Peer panel —
// as the outer left_pile.contents[1] takes focus_position.
func TestAnnounceStreamDownEntersPanel(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, []AnnounceEntry{
		nodeAnnounce("Node A"),
		nodeAnnounce("Node B"),
	}, nil)

	md := app.Main
	md.SetDisplay("network", nd.Widget())
	md.SelectPage("network")
	md.Root().SetRect(0, 0, 135, 32)
	app.SetRoot()

	// Ctrl-L: Saved Nodes → Announce Stream, focused on its tab bar (Python's
	// AnnounceStream Pile defaults to index 0).
	nd.toggleList()
	nd.FocusLists()
	if got := nd.announceStream.pile.FocusIndex(); got != 0 {
		t.Fatalf("announce stream pile focus = %v, want 0 (tab bar)", got)
	}

	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)

	// Down #1/#2: tab bar → filter bar → the stream list (the inner Pile's own
	// traversal, which the outer pile must not steal).
	dispatchApp(t, app, down)
	dispatchApp(t, app, down)
	if got := nd.announceStream.pile.FocusIndex(); got != 2 {
		t.Fatalf("after two Downs the announce stream pile focus = %v, want 2 (the list)", got)
	}

	// Down #3: the list still has a row below, so it keeps the key.
	dispatchApp(t, app, down)
	if got := nd.announces.GetCurrentItem(); got != 1 {
		t.Fatalf("after Down in the stream list, cursor = %v, want 1 (the list keeps Down)", got)
	}
	if nd.localPeer.Widget().HasFocus() {
		t.Fatalf("the panel took focus before the stream list reached its last row")
	}

	// Down #4: at the list's last row the key leaves the nested pile and lands
	// on the Local Peer panel's first selectable row (the Name edit).
	dispatchApp(t, app, down)
	if !nd.localPeer.NameEditHasFocus() {
		t.Fatalf("after Down at the stream list's last row, focus = %T, want the Name edit", app.GetFocus())
	}
}

// TestNetworkPanelUpReturnsToList pins the reverse of the route above: Up from
// the panel's first selectable row (the Name edit) hands focus back to the
// saved-nodes list at its last row, exactly as urwid's Pile.keypress does when
// the candidates run off the top of the focused widget's own pile.
func TestNetworkPanelUpReturnsToList(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6", DisplayName: "Saved A"},
		{SourceHash: "b1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d7", DisplayName: "Saved B"},
	})

	md := app.Main
	md.SetDisplay("network", nd.Widget())
	md.SelectPage("network")
	md.Root().SetRect(0, 0, 135, 32)
	app.SetRoot()
	nd.FocusLists()

	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)

	// Walk into the panel (Down past the list's last row).
	dispatchApp(t, app, down)
	dispatchApp(t, app, down)
	if !nd.localPeer.NameEditHasFocus() {
		t.Fatalf("setup: Down out of the list did not focus the Name edit (focus=%T)", app.GetFocus())
	}

	// Up returns to the list, whose cursor is still on its last row.
	dispatchApp(t, app, up)
	if nd.localPeer.Widget().HasFocus() {
		t.Fatalf("after Up from the panel's first row, the panel still has focus (focus=%T)", app.GetFocus())
	}
	if got := nd.nodes.GetCurrentItem(); got != len(nd.nodeData)-1 {
		t.Errorf("after Up, saved-nodes cursor = %v, want %v (the list keeps its row)", got, len(nd.nodeData)-1)
	}

	// Up #2 walks the list cursor back up.
	dispatchApp(t, app, up)
	if got := nd.nodes.GetCurrentItem(); got != 0 {
		t.Errorf("after a second Up, saved-nodes cursor = %v, want 0", got)
	}
}
