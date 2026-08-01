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
	"strings"
	"testing"

	"github.com/rivo/tview"
)

// TestNetworkDisplayNoOuterBorder asserts the Network page has NO outer
// LineBox/title around the whole page: Python's NetworkDisplay sets
// `self.widget = self.columns` directly (Network.py:1666) — the two columns sit
// in the body with no enclosing border, each pane providing its own. The prior
// Go skeleton wrapped everything in a bordered Flex titled "Network", which is
// not parity. The columns Flex itself therefore carries no title.
func TestNetworkDisplayNoOuterBorder(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	cols, ok := nd.Widget().(*tview.Flex)
	if !ok {
		t.Fatalf("Widget is %T, want *tview.Flex (the columns)", nd.Widget())
	}
	if got := cols.GetTitle(); got != "" {
		t.Errorf("outer columns title = %q, want empty (Python has no outer border/title)", got)
	}
}

// TestNetworkDisplayLeftPaneTitled asserts the left pane carries its own titled
// border matching the active list mode, mirroring Python's sub-widget LineBoxes:
// KnownNodes is titled "Saved Nodes" (Network.py:867), AnnounceStream is titled
// "Announce Stream" (Network.py:446). Python defaults list_display=1
// (Network.py:1638), so the pane opens on "Saved Nodes"; toggling swaps to the
// announce stream and updates the title. The title is stored with a
// leading/trailing space (SetTitledBorder) to match urwid LineBox rendering.
func TestNetworkDisplayLeftPaneTitled(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	if got := nd.leftPanel.GetTitle(); got != " Saved Nodes " {
		t.Errorf("default left pane title = %q, want \" Saved Nodes \"", got)
	}

	nd.toggleList() // showingNodes -> false (announce stream)
	if got := nd.leftPanel.GetTitle(); got != " Announce Stream " {
		t.Errorf("after toggle left pane title = %q, want \" Announce Stream \"", got)
	}

	nd.toggleList() // back to saved nodes
	if got := nd.leftPanel.GetTitle(); got != " Saved Nodes " {
		t.Errorf("after second toggle left pane title = %q, want \" Saved Nodes \"", got)
	}
}

// TestNetworkDisplayNodesEmptyState asserts that with no saved nodes the left
// pane shows the centered empty-state message (Python KnownNodes empty-state,
// Network.py:833-882) in place of the list, and that adding nodes via
// UpdateNodes swaps the list back in.
func TestNetworkDisplayNodesEmptyState(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	if nd.nodes.GetItemCount() != 0 {
		t.Fatalf("precondition: nodes list should be empty")
	}
	if got := nd.nodeEmptyState.GetText(true); !strings.Contains(got, "Currently, no nodes are saved") {
		t.Errorf("empty-state text = %q, want it to contain the no-nodes message", got)
	}
	if !strings.Contains(nd.nodeEmptyState.GetText(true), "Ctrl+L to view the announce stream") {
		t.Errorf("empty-state text = %q, want it to contain the Ctrl+L hint", nd.nodeEmptyState.GetText(true))
	}

	// Adding a node and refreshing swaps the list in (nodesView returns the
	// IndicativeListBox). addNodeEntry + refreshNodesView mirror the non-marshaling
	// half of UpdateNodes, avoiding QueueUpdateDraw (which blocks with no loop).
	nd.addNodeEntry(NodeEntry{SourceHash: "000102030405060708090a0b0c0d0e0f", TrustLevel: "unknown", DisplayName: "Alice"})
	nd.refreshNodesView()
	if nd.nodes.GetItemCount() != 1 {
		t.Errorf("after add, item count = %d, want 1", nd.nodes.GetItemCount())
	}
	if nd.nodesView() != (tview.Primitive)(nd.nodesList) {
		t.Errorf("nodesView after adding = %T, want nodesList", nd.nodesView())
	}
}
