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
	"time"

	"github.com/gdamore/tcell/v2"
)

// buttonsInRow collects the UrwidButtons in a columns row in order, for
// asserting the Go-enhancement Block button placement.
func buttonsInRow(row *urwidColumns) []*UrwidButton {
	var out []*UrwidButton
	for _, c := range row.children {
		if b, ok := c.(*UrwidButton); ok {
			out = append(out, b)
		}
	}
	return out
}

// labelsInRow extracts the button labels of an announce-info button row.
func labelsInRow(row *urwidColumns) []string {
	var out []string
	for _, b := range buttonsInRow(row) {
		out = append(out, b.Label())
	}
	return out
}

// newBlockTestNetworkDisplay builds a NetworkDisplay with the given OnBlockNode
// hook for announce-info tests.
func newBlockTestNetworkDisplay(onBlock func(string)) (*NetworkDisplay, AnnounceEntry) {
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)
	nd.OnBlockNode = onBlock
	ann := AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		SourceHash:  strings.Repeat("b", 32),
		AppData:     "Node app data",
		Type:        "node",
		DisplayName: "MyNode",
	}
	return nd, ann
}

// TestAnnounceInfoNodeHasBlockButton pins the Go-only "Block" button on the
// node-branch AnnounceInfo button row. This is a deliberate Go enhancement
// with NO Python source-of-truth counterpart (Python's AnnounceInfo node row
// is Back/Connect/Msg Op/Save, Network.py:207-222); it adds a fifth Block
// button that fires OnBlockNode with the announce's source hash. The first
// row keeps the Python-parity button set and weights; Block lives on its own
// row below so the pinned Python layout (widths [11,2,11,2,11,2,11] at inner
// 50) is unchanged.
func TestAnnounceInfoNodeHasBlockButton(t *testing.T) {
	t.Parallel()
	var blocked string
	nd, ann := newBlockTestNetworkDisplay(func(hash string) { blocked = hash })

	ai := newAnnounceInfoDisplay(nd, ann, AnnounceInfoData{
		DisplayStr: "MyNode",
		TrustStr:   "Trusted",
		TrustStyle: "list_trusted",
	})

	// First row: Python-parity buttons in original order.
	first := labelsInRow(ai.buttonRow())
	wantFirst := []string{"Back", "Connect", "Msg Op", "Save"}
	if len(first) != len(wantFirst) {
		t.Fatalf("primary button row labels = %v, want %v", first, wantFirst)
	}
	for i, w := range wantFirst {
		if first[i] != w {
			t.Errorf("primary button %v = %q, want %q", i, first[i], w)
		}
	}

	// Second row: the Go enhancement Block button. It must NOT span the full
	// panel width (bug report: "< Block >" filled the whole Announce Info
	// panel); it renders in the same ~9/41 column share as the row above, so
	// it aligns with the first button of the Python-parity row.
	second := ai.blockButtonRow()
	weights := second.weights
	if len(weights) != 3 || weights[0] != 9 || weights[1] != 2 || weights[2] != 30 {
		t.Fatalf("block row weights = %v, want [9 2 30] (button + divider + trailing filler)", weights)
	}
	blockBtn := buttonsInRow(second)[0]
	if blockBtn.selected == nil {
		t.Fatal("Block button has no selected handler")
	}
	blockBtn.selected()
	if blocked != ann.SourceHash {
		t.Errorf("Block click fired with %q, want the announce source hash %q", blocked, ann.SourceHash)
	}
}

// TestAnnounceInfoBlockRowRendersNarrowWidth pins the fixed-width rendering of
// the Go-only Block row (bug report: the Block button spanned the whole panel
// width). At inner 50 the row above lays out as [11,2,11,2,11,2,11]; the Block
// row must give its button the same 11-cell column as Back's, with the rest of
// the row blank.
func TestAnnounceInfoBlockRowRendersNarrowWidth(t *testing.T) {
	t.Parallel()
	nd, ann := newBlockTestNetworkDisplay(nil)

	ai := newAnnounceInfoDisplay(nd, ann, AnnounceInfoData{
		DisplayStr: "MyNode",
		TrustStr:   "Trusted",
		TrustStyle: "list_trusted",
	})
	rows := renderPrimitive(t, ai.Widget(), 50, 14)

	// Node pile rows: 0-5 headers, 6 divider, 7-8 data, 9 divider,
	// 10 Python-parity button row, 11 the Go-only Block row.
	blockRow := rows[11]
	if want := "< Block   >"; blockRow[:11] != want {
		t.Errorf("Block row head = %q, want prefix %q (11 cells, like the Back column)", blockRow[:11], want)
	}
	if strings.TrimLeft(blockRow[11:], " ") != "" {
		t.Errorf("Block row tail = %q, want blank filler past the button", blockRow)
	}
}

// TestAnnounceInfoBlockRowFocusTraversal pins the arrow-key path to the Block
// row (bug report: arrow keys could not reach Block — the row was not a
// selectable pile item). Down from the Python-parity button row must focus
// the Block row; Up must return; and Enter on the focused Block row must fire
// OnBlockNode.
func TestAnnounceInfoBlockRowFocusTraversal(t *testing.T) {
	t.Parallel()
	var blocked string
	nd, ann := newBlockTestNetworkDisplay(func(hash string) { blocked = hash })

	ai := newAnnounceInfoDisplay(nd, ann, AnnounceInfoData{
		DisplayStr: "MyNode",
		TrustStr:   "Trusted",
		TrustStyle: "list_trusted",
	})

	if got := ai.pile.FocusIndex(); got != 0 {
		t.Fatalf("initial pile focus index = %v, want 0 (the Python-parity button row)", got)
	}

	handler := ai.pile.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), nil)
	if got := ai.pile.FocusIndex(); got != 1 {
		t.Fatalf("pile focus after Down = %v, want 1 (the Block row)", got)
	}

	// Enter on the focused Block row fires OnBlockNode with the source hash.
	if blocked != "" {
		t.Fatalf("precondition: Block not fired yet, got %q", blocked)
	}
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), nil)
	if blocked != ann.SourceHash {
		t.Errorf("Enter on the focused Block row fired with %q, want %q", blocked, ann.SourceHash)
	}

	// Up returns focus to the Python-parity button row.
	handler(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), nil)
	if got := ai.pile.FocusIndex(); got != 0 {
		t.Fatalf("pile focus after Up = %v, want 0 (the button row)", got)
	}
}

// TestAnnounceInfoPeerPNNoBlockButton verifies the Go-only Block button does
// not appear on peer or propagation-node announce info rows.
func TestAnnounceInfoPeerPNNoBlockButton(t *testing.T) {
	t.Parallel()
	nd, _ := newBlockTestNetworkDisplay(nil)

	peerAnn := AnnounceEntry{Timestamp: time.Now(), SourceHash: strings.Repeat("e", 32), Type: "peer"}
	peerLabels := labelsInRow(newAnnounceInfoDisplay(nd, peerAnn, AnnounceInfoData{}).buttonRow())
	if len(peerLabels) != 2 || peerLabels[0] != "Back" || peerLabels[1] != "Converse" {
		t.Errorf("peer button labels = %v, want [Back Converse]", peerLabels)
	}

	pnAnn := AnnounceEntry{Timestamp: time.Now(), SourceHash: strings.Repeat("c", 32), Type: "pn"}
	pnLabels := labelsInRow(newAnnounceInfoDisplay(nd, pnAnn, AnnounceInfoData{}).buttonRow())
	if len(pnLabels) != 2 || pnLabels[0] != "Back" || pnLabels[1] != "Use as default" {
		t.Errorf("pn button labels = %v, want [Back Use as default]", pnLabels)
	}
}
