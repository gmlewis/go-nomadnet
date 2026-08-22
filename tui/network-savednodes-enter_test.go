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
)

// TestSavedNodesEnterFiresOnConnectNode pins the reported parity bug "in the
// Saved Nodes, it is impossible to hit Enter on a saved node to open a saved
// node". Python's KnownNodes NodeEntry wires the urwid ListEntry "click" signal
// to connect_node (Network.py:1019 + 881-919), and that signal fires on Enter,
// opening the "Connect to node?" dialog. The Go port had previously made Enter
// a no-op (mistaking node_list_selection — the selection-change callback,
// Network.py:878-879 — for the Enter handler). Enter on a Saved Nodes row must
// now dispatch the selected node to OnConnectNode, which the wiring layer uses
// to open the connect dialog.
func TestSavedNodesEnterFiresOnConnectNode(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	node := NodeEntry{
		SourceHash:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
		DisplayName: "TestNode",
		TrustLevel:  "unknown",
	}
	nd := NewNetworkDisplay(app, nil, []NodeEntry{node})
	nd.Widget().SetRect(0, 0, 200, 50)

	// showingNodes starts true, so the left pane shows the Saved Nodes list.
	if !nd.showingNodes {
		t.Fatalf("setup: Saved Nodes should be the initial left-pane view")
	}
	if got := nd.nodes.GetItemCount(); got != 1 {
		t.Fatalf("setup: saved nodes list should have 1 row, got %d", got)
	}

	// Focus the saved-nodes list (the constructor swaps it in but does not focus
	// it; focusLeftList is the UI-thread focus step toggleList/etc. perform).
	nd.focusLeftList()

	var got NodeEntry
	var fired bool
	nd.OnConnectNode = func(n NodeEntry) { got = n; fired = true }

	// Enter on the selected row must fire OnConnectNode with that node.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if !fired {
		t.Fatalf("Enter on a Saved Nodes row did not fire OnConnectNode (the connect dialog opener); the port must wire the list's Enter to connect_node, not leave it a no-op")
	}
	if got.SourceHash != node.SourceHash {
		t.Errorf("OnConnectNode received SourceHash %q, want %q", got.SourceHash, node.SourceHash)
	}
	if got.DisplayName != node.DisplayName {
		t.Errorf("OnConnectNode received DisplayName %q, want %q", got.DisplayName, node.DisplayName)
	}
}
