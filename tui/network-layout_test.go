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

	"github.com/gdamore/tcell/v2"
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

	cols, ok := nd.Widget().(*urwidColumns)
	if !ok {
		t.Fatalf("Widget is %T, want *urwidColumns (Python urwid.Columns)", nd.Widget())
	}
	if cols == nil {
		t.Fatalf("expected non-nil urwidColumns")
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

	if got := nd.listBox.GetTitle(); got != " Saved Nodes " {
		t.Errorf("default list box title = %q, want \" Saved Nodes \"", got)
	}

	nd.toggleList() // showingNodes -> false (announce stream)
	if got := nd.listBox.GetTitle(); got != " Announce Stream " {
		t.Errorf("after toggle list box title = %q, want \" Announce Stream \"", got)
	}

	nd.toggleList() // back to saved nodes
	if got := nd.listBox.GetTitle(); got != " Saved Nodes " {
		t.Errorf("after second toggle list box title = %q, want \" Saved Nodes \"", got)
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
	if got := nd.nodeEmptyState.GetText(); !strings.Contains(got, "Currently, no nodes are saved") {
		t.Errorf("empty-state text = %q, want it to contain the no-nodes message", got)
	}
	if !strings.Contains(nd.nodeEmptyState.GetText(), "Ctrl+L to view the announce stream") {
		t.Errorf("empty-state text = %q, want it to contain the Ctrl+L hint", nd.nodeEmptyState.GetText())
	}

	// Adding a node and refreshing swaps the list in (nodesView returns the
	// IndicativeListBox). addNodeEntry + refreshNodesView mirror the non-marshaling
	// half of UpdateNodes, avoiding QueueUpdateDraw (which blocks with no loop).
	nd.addNodeEntry(NodeEntry{SourceHash: "000102030405060708090a0b0c0d0e0f", TrustLevel: "unknown", DisplayName: "Alice"})
	nd.refreshNodesView()
	if nd.nodes.GetItemCount() != 1 {
		t.Errorf("after add, item count = %v, want 1", nd.nodes.GetItemCount())
	}
	if nd.nodesView() != (tview.Primitive)(nd.nodesList) {
		t.Errorf("nodesView after adding = %T, want nodesList", nd.nodesView())
	}
}

// TestNetworkDisplayShowNodeInfoSwap verifies the bottom of the left pane swaps
// from the Local Peer Info panel to the Local Node Info panel and back,
// matching Python's node_info_query/show_peer_info (Network.py:1399-1401,
// 1396-1398) which replace left_pile.contents[1]. The list slot (contents[0])
// stays put; only the PACK bottom slot changes.
func TestNetworkDisplayShowNodeInfoSwap(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	nd := NewNetworkDisplay(app, nil, nil)

	// Initially the bottom slot is the Local Peer Info panel.
	if nd.leftPanel.GetItem(1) != nd.localPeer.Widget() {
		t.Errorf("initial bottom slot = %T, want localPeer.Widget", nd.leftPanel.GetItem(1))
	}

	// ShowNodeInfo swaps the bottom slot to the NodeInfo panel without
	// touching the list slot (index 0 remains listBox).
	nd.ShowNodeInfo(NodeInfoData{HasNode: false})
	if nd.nodeInfo == nil {
		t.Fatal("ShowNodeInfo did not create the NodeInfo panel")
	}
	if nd.leftPanel.GetItem(1) != nd.nodeInfo.Widget() {
		t.Errorf("after ShowNodeInfo bottom slot = %T, want nodeInfo.Widget", nd.leftPanel.GetItem(1))
	}
	if nd.leftPanel.GetItem(0) != nd.listBox {
		t.Errorf("list slot changed after ShowNodeInfo; want listBox unchanged")
	}

	// ShowLocalPeer swaps back to the Local Peer Info panel.
	nd.ShowLocalPeer()
	if nd.leftPanel.GetItem(1) != nd.localPeer.Widget() {
		t.Errorf("after ShowLocalPeer bottom slot = %T, want localPeer.Widget", nd.leftPanel.GetItem(1))
	}
}

// TestNetworkDisplayShowPeersSwap pins the C-p (show_peers) left-pane swap and
// the ctrl-l toggle-back, matching Python's show_peers + toggle_list
// (Network.py:1688, 1668). C-p swaps the list slot to the LXMF peers list
// (titled "LXMF Propagation Peers (0)"); a subsequent ctrl-l returns to the
// mode that was showing before C-p (because show_peers flips the toggle state).
func TestNetworkDisplayShowPeersSwap(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	// Boot: Saved Nodes (showingNodes=true).
	if got := nd.listBox.GetTitle(); got != " Saved Nodes " {
		t.Fatalf("boot title = %q, want ' Saved Nodes '", got)
	}
	if nd.ShowingPeers() {
		t.Error("boot: ShowingPeers should be false")
	}

	// C-p → peers list. The list slot's first child becomes the peers content.
	nd.showPeers()
	if !nd.ShowingPeers() {
		t.Error("after showPeers: ShowingPeers should be true")
	}
	if got := nd.listBox.GetTitle(); got != " LXMF Propagation Peers (0) " {
		t.Errorf("after showPeers title = %q, want ' LXMF Propagation Peers (0) '", got)
	}

	// ctrl-l from peers returns to Saved Nodes (show_peers flipped showingNodes
	// from true→false; toggleList shows the opposite of showingNodes=false →
	// Saved Nodes, and flips showingNodes back to true).
	nd.toggleList()
	if nd.ShowingPeers() {
		t.Error("after toggleList from peers: ShowingPeers should be false")
	}
	if got := nd.listBox.GetTitle(); got != " Saved Nodes " {
		t.Errorf("after toggleList title = %q, want ' Saved Nodes '", got)
	}

	// From Announce Stream, C-p then ctrl-l returns to Announce Stream.
	nd.toggleList() // Saved Nodes → Announce Stream
	if got := nd.listBox.GetTitle(); got != " Announce Stream " {
		t.Fatalf("expected Announce Stream, got %q", got)
	}
	nd.showPeers()
	if got := nd.listBox.GetTitle(); got != " LXMF Propagation Peers (0) " {
		t.Errorf("after showPeers (from announce) title = %q", got)
	}
	nd.toggleList() // showingNodes was flipped false→true by showPeers; toggle → Announce Stream
	if got := nd.listBox.GetTitle(); got != " Announce Stream " {
		t.Errorf("after toggleList (from announce path) title = %q, want ' Announce Stream '", got)
	}
}

// TestNetworkDisplayUpdateLXMFPeersRefreshesTitle verifies UpdateLXMFPeers
// updates the slot title while the peers list is displayed.
func TestNetworkDisplayUpdateLXMFPeersRefreshesTitle(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	nd.showPeers()
	if got := nd.listBox.GetTitle(); got != " LXMF Propagation Peers (0) " {
		t.Fatalf("title = %q, want count 0", got)
	}

	nd.UpdateLXMFPeers([]LXMFPeerEntry{{Hash: "abcdef", Name: "node1", Alive: true}})
	if got := nd.lxmfPeers.Count(); got != 1 {
		t.Errorf("Count = %v, want 1", got)
	}
	if got := nd.listBox.GetTitle(); got != " LXMF Propagation Peers (1) " {
		t.Errorf("title after update = %q, want count 1", got)
	}
}

// TestNetworkDisplayColumnFocusNavigation verifies RightArrow/Tab moves focus from
// the left panel into the Remote Node browser pane, and mouse clicking on the
// right pane focuses the browser pane.
func TestNetworkDisplayColumnFocusNavigation(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	cols, ok := nd.Widget().(*urwidColumns)
	if !ok {
		t.Fatalf("Widget is %T, want *urwidColumns", nd.Widget())
	}

	setFocus := func(p tview.Primitive) {
		if p != nil {
			p.Focus(func(tview.Primitive) {})
		}
	}

	cols.Focus(setFocus)
	if cols.FocusIndex() != 0 {
		t.Errorf("initial focusIndex = %v, want 0 (left panel)", cols.FocusIndex())
	}

	// KeyRight moves focus to column 1 (Remote Node browser pane)
	cols.InputHandler()(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), setFocus)
	if cols.FocusIndex() != 1 {
		t.Errorf("after KeyRight focusIndex = %v, want 1 (browser pane)", cols.FocusIndex())
	}

	// KeyLeft does NOT move back to column 0: the browser pane is self-managing,
	// so Left belongs to the browser's own part-cursor model (and its Left-at-
	// start focus release via OnReleaseFocus, which is what hands focus back to
	// the left list in Python's micron_released_focus — not the outer Columns
	// pane-wrapping). The outer Columns stays on the browser column.
	cols.InputHandler()(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), setFocus)
	if cols.FocusIndex() != 1 {
		t.Errorf("after KeyLeft focusIndex = %v, want 1 (browser owns Left, no pane-wrap)", cols.FocusIndex())
	}

	// Mouse click on right pane (e.g. x=60, y=10) sets focus to column 1
	cols.SetRect(0, 0, 100, 30)
	ev := tcell.NewEventMouse(60, 10, tcell.Button1, 0)
	cols.MouseHandler()(tview.MouseLeftClick, ev, setFocus)
	if cols.FocusIndex() != 1 {
		t.Errorf("after mouse click at x=60 focusIndex = %v, want 1 (browser pane)", cols.FocusIndex())
	}
}
