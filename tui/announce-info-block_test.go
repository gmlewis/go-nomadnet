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

	// Second row: the Go enhancement Block button, full width.
	second := ai.blockButtonRow()
	blockBtns := labelsInRow(second)
	if len(blockBtns) != 1 || blockBtns[0] != "Block" {
		t.Fatalf("block row buttons = %v, want [Block]", blockBtns)
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
