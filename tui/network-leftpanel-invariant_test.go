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
	"fmt"
	"testing"

	"github.com/rivo/tview"
)

// The left-panel invariants every mutation sequence must preserve (python
// NetworkDisplay parity):
//
//  1. leftPanel ALWAYS contains exactly TWO items.
//  2. Index 0 is the list slot: either the listBox (Saved Nodes / Announce
//     Stream / Announce Info / Peers — the toggled list modes) or the
//     listSlotOverlay that temporarily covers it.
//  3. Index 1 is the PACK panel: LocalPeer, or the Local Node Info detail
//     panel (packPanelIsNodeInfo), or the transient status dialog.
//  4. The Local Node Info panel NEVER renders above the list slot.
//
// The bug this pins: the previous incremental mutators (RemoveItem + AddItem
// per panel swap) interleaved badly when the Local Node Info detail panel was
// showing while a list-slot dialog or status notice fired — tview Flex.AddItem
// always appends, so items stranded in the wrong index and hidden panels were
// resurrected (fleet render: [Local Node Info, Announce Stream, Local Peer
// Info] with the browser pane pushed right). rebuildLeftPanel now rebuilds
// the stack from state, and TestLeftPanelInvariantsUnderMutation walks a
// deterministic-but-varying mutator sequence asserting the invariants after
// every step so any future mutator that skips the rebuild reintroduces the
// mess instead of silently corrupting the layout.
func assertLeftPanelInvariants(t *testing.T, nd *NetworkDisplay, step string) {
	t.Helper()
	if got := nd.leftPanel.GetItemCount(); got != 2 {
		t.Fatalf("%s: left panel item count = %d, want exactly 2", step, got)
	}
	item0 := nd.leftPanel.GetItem(0)
	if item0 != tview.Primitive(nd.listBox) && item0 != tview.Primitive(nd.listSlotOverlay) {
		t.Fatalf("%s: index 0 = %T, want listBox or listSlotOverlay", step, item0)
	}
	idx1 := nd.leftPanel.GetItem(1)
	var want1 tview.Primitive
	switch {
	case nd.statusInPeerSlot != nil:
		want1 = nd.statusInPeerSlot
	case nd.packPanelIsNodeInfo && nd.nodeInfo != nil:
		want1 = nd.nodeInfo.Widget()
	default:
		want1 = nd.localPeer.Widget()
	}
	if idx1 != want1 {
		t.Fatalf("%s: index 1 = %T, want %T (pack state: nodeInfo=%v status=%v)",
			step, idx1, want1, nd.packPanelIsNodeInfo, nd.statusInPeerSlot != nil)
	}
}

// TestLeftPanelInvariantsUnderRandomMutations drags the Network left pane
// through every panel mutator in a shuffled deterministic order and asserts
// the canonical composition after each single step. Any future mutator that
// bypasses rebuildLeftPanel (incremental RemoveItem/AddItem) scrambles the
// stack and fails this test immediately, instead of corrupting the live TUI.
func TestLeftPanelInvariantsUnderMutatorSequence(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	nd := NewNetworkDisplay(app, nil, nil)

	// The exact mutator set that touched nd.leftPanel before the rebuild
	// centralization (the fleet's penguin node hit this order live).
	steps := []string{
		"ShowNodeInfo",        // Local Node Info detail replaces Local Peer Info
		"ShowListSlotDialog",  // in-slot dialog overlays the list
		"CloseListSlotDialog", // restore list
		"ShowLocalPeer",       // Local Peer Info back in the pack slot
		"ShowNodeInfo",        // again (repeat press)
		"CloseListSlotDialog", // no-op when no dialog: must stay canonical
		"ShowLocalPeerStatus", // Saved / Announce Sent notice
		"CloseListSlotDialog", // dialog cycles interleaved with the status
		"ShowListSlotDialog",
		"ShowNodeInfo",
		"ShowLocalPeerStatus",
		"CloseLocalPeerStatus",
		"ShowLocalPeer",
	}
	for i, step := range steps {
		switch step {
		case "ShowNodeInfo":
			nd.ShowNodeInfo(NodeInfoData{Addr: "6853937554960093a05764c3974f28e6"})
		case "ShowLocalPeer":
			nd.ShowLocalPeer()
		case "ShowListSlotDialog":
			nd.ShowListSlotDialog(NewDialogLineBox("?", tview.NewFlex(), nil), 6)
		case "ShowLocalPeerStatus":
			nd.ShowLocalPeerStatus(fmt.Sprintf("\n\n\nStatus %d\n\n", i), 6)
		case "CloseListSlotDialog":
			nd.CloseListSlotDialog()
		}
		assertLeftPanelInvariants(t, nd, fmt.Sprintf("step %d (%v)", i, step))
	}
	_ = tview.NewFlex
	_ = fmt.Sprintf
}
