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

	"github.com/rivo/tview"
)

// TestListSlotDialogPreservesPanelOrder pins the reported bug "when a Saved
// Node is connected to, the panels in gonomadnet switch positions unexpectedly
// (the Local Node Info should always be at the bottom)". Python's left_pile is
// a urwid.Pile whose contents are addressed by index:
//
//	contents[0] = WEIGHT 1 list slot (KnownNodes / AnnounceStream / overlay)
//	contents[1] = PACK    LocalPeer (Local Node Info) — ALWAYS at the bottom
//
// Python swaps contents[0] via indexed assignment
// (left_pile.contents[0] = overlay, Network.py:918-919) which keeps
// contents[1] in place. The Go port uses tview.Flex, whose AddItem always
// appends to the end; removing only listBox and appending the overlay left
// localPeer at index 0 (top) and the overlay at index 1 (bottom). Closing the
// dialog appended listBox after localPeer, leaving [localPeer, listBox] —
// Local Node Info at the top, list at the bottom — the reported swap.
//
// The fix removes BOTH items and re-adds them in the correct order so
// localPeer always stays at index 1 (bottom), matching Python's contents[1].
func TestListSlotDialogPreservesPanelOrder(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	nd := NewNetworkDisplay(app, nil, nil)

	// Initial layout: [listBox, localPeer] — list at top, LocalPeer at bottom.
	if nd.leftPanel.GetItemCount() != 2 {
		t.Fatalf("initial item count = %d, want 2", nd.leftPanel.GetItemCount())
	}
	if nd.leftPanel.GetItem(0) != tview.Primitive(nd.listBox) {
		t.Errorf("initial index 0 = %T, want listBox", nd.leftPanel.GetItem(0))
	}
	if nd.leftPanel.GetItem(1) != nd.localPeer.Widget() {
		t.Errorf("initial index 1 = %T, want localPeer.Widget", nd.leftPanel.GetItem(1))
	}

	// ShowListSlotDialog replaces listBox with an overlay. localPeer must stay
	// at index 1 (bottom), overlay at index 0 (top).
	dialog := NewDialogLineBox("?", tview.NewFlex(), nil)
	nd.ShowListSlotDialog(dialog, 6)

	if nd.leftPanel.GetItemCount() != 2 {
		t.Fatalf("after ShowListSlotDialog item count = %d, want 2", nd.leftPanel.GetItemCount())
	}
	if nd.leftPanel.GetItem(0) == tview.Primitive(nd.listBox) {
		t.Errorf("after ShowListSlotDialog index 0 is still listBox; want overlay")
	}
	if nd.leftPanel.GetItem(1) != nd.localPeer.Widget() {
		t.Errorf("after ShowListSlotDialog index 1 = %T, want localPeer.Widget (Local Node Info must stay at bottom)", nd.leftPanel.GetItem(1))
	}

	// CloseListSlotDialog restores listBox. localPeer must STILL be at index 1.
	nd.CloseListSlotDialog()

	if nd.leftPanel.GetItemCount() != 2 {
		t.Fatalf("after CloseListSlotDialog item count = %d, want 2", nd.leftPanel.GetItemCount())
	}
	if nd.leftPanel.GetItem(0) != tview.Primitive(nd.listBox) {
		t.Errorf("after CloseListSlotDialog index 0 = %T, want listBox (list must return to top)", nd.leftPanel.GetItem(0))
	}
	if nd.leftPanel.GetItem(1) != nd.localPeer.Widget() {
		t.Errorf("after CloseListSlotDialog index 1 = %T, want localPeer.Widget (Local Node Info must stay at bottom)", nd.leftPanel.GetItem(1))
	}
}

// TestListSlotDialogRepeatPreservesOrder verifies the ordering survives
// multiple open/close cycles — the scenario the user hits when connecting to
// several saved nodes in a row.
func TestListSlotDialogRepeatPreservesOrder(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	nd := NewNetworkDisplay(app, nil, nil)

	for i := range 3 {
		dialog := NewDialogLineBox("?", tview.NewFlex(), nil)
		nd.ShowListSlotDialog(dialog, 6)
		nd.CloseListSlotDialog()

		if nd.leftPanel.GetItem(0) != tview.Primitive(nd.listBox) {
			t.Errorf("cycle %d: index 0 = %T, want listBox", i, nd.leftPanel.GetItem(0))
		}
		if nd.leftPanel.GetItem(1) != nd.localPeer.Widget() {
			t.Errorf("cycle %d: index 1 = %T, want localPeer.Widget", i, nd.leftPanel.GetItem(1))
		}
	}
}
