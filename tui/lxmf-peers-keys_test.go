// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"bytes"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestLXMFPeersKeybindings pins the C-x (unpeer) and C-r (delivery sync)
// keybindings of LXMFPeersDisplay (Python LXMFPeers.keypress, Network.py:1793-
// 1798): each fires its callback with the *selected* peer's destination hash
// and consumes the event; other keys pass through untouched.
func TestLXMFPeersKeybindings(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	pp := NewLXMFPeersDisplay(app)

	hashA := []byte{0xaa, 0x11, 0x22}
	hashB := []byte{0xbb, 0x33, 0x44}
	peers := []LXMFPeerEntry{
		{Name: "PeerA", DisplayText: "PeerA", DestinationHash: hashA},
		{Name: "PeerB", DisplayText: "PeerB", DestinationHash: hashB},
	}
	pp.SetPeers(peers)
	if pp.Count() != 2 {
		t.Fatalf("Count = %v, want 2", pp.Count())
	}

	list, ok := pp.Widget().(*tview.List)
	if !ok {
		t.Fatalf("Widget is %T, want *tview.List", pp.Widget())
	}

	var unpeerGot, syncGot []byte
	pp.OnUnpeer = func(h []byte) { unpeerGot = append(unpeerGot[:0:0], h...) }
	pp.OnSync = func(h []byte) { syncGot = append(syncGot[:0:0], h...) }

	// Select the second peer (index 1) and dispatch C-x → unpeer peer B.
	list.SetCurrentItem(1)
	ev := tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone)
	if got := pp.handlePeerKey(list, ev); got != nil {
		t.Errorf("ctrl-x should be consumed, got %#v", got)
	}
	if !bytes.Equal(unpeerGot, hashB) {
		t.Errorf("OnUnpeer got %x, want %x", unpeerGot, hashB)
	}

	// C-r on the first peer (index 0) → sync peer A.
	list.SetCurrentItem(0)
	ev = tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone)
	if got := pp.handlePeerKey(list, ev); got != nil {
		t.Errorf("ctrl-r should be consumed, got %#v", got)
	}
	if !bytes.Equal(syncGot, hashA) {
		t.Errorf("OnSync got %x, want %x", syncGot, hashA)
	}

	// A neutral key passes through unchanged.
	neutral := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if got := pp.handlePeerKey(list, neutral); got != neutral {
		t.Errorf("neutral key should pass through, got %#v", got)
	}

	// With no callbacks wired, C-x/C-r still consume (Python returns from the
	// keypress) but call nothing — verified by clearing callbacks and checking
	// no panic and event still consumed.
	pp.OnUnpeer = nil
	pp.OnSync = nil
	if got := pp.handlePeerKey(list, tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone)); got != nil {
		t.Errorf("ctrl-x with nil callback should still be consumed")
	}
}

// TestLXMFPeersRenderFullText verifies a populated peer list renders the
// pre-rendered peer_info_str DisplayText as each row's main text.
func TestLXMFPeersRenderFullText(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	pp := NewLXMFPeersDisplay(app)
	pp.SetPeers([]LXMFPeerEntry{
		{DisplayText: "↑ <aaaa>\n  Available, last heard just now", DestinationHash: []byte{0xaa}},
		{DisplayText: "↑ <bbbb>\n  Unresponsive, last heard just now", DestinationHash: []byte{0xbb}},
	})

	list, ok := pp.Widget().(*tview.List)
	if !ok {
		t.Fatalf("Widget is %T, want *tview.List", pp.Widget())
	}
	if got := list.GetItemCount(); got != 2 {
		t.Fatalf("item count = %v, want 2", got)
	}
	mainA, _ := list.GetItemText(0)
	if mainA != "↑ <aaaa>\n  Available, last heard just now" {
		t.Errorf("row 0 main text = %q", mainA)
	}
}
