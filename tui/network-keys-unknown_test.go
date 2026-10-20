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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// TestAnnounceInfoKeysUnknownDialog pins Python's "Keys Unknown" dialog
// (Network.py:337-385): a NODE announce whose identity cannot be recalled
// raises KeyError in AnnounceInfo, and the Network display offers to request
// the keys instead of rendering the announce detail. Request keys asks the
// network for the path and reports "Keys requested from network"; Close (or
// Esc) restores the announce stream.
func TestAnnounceInfoKeysUnknownDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	nodeHash := strings.Repeat("2", 32)
	ann := AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		SourceHash:  nodeHash,
		AppData:     "MyNode",
		Type:        "node",
		DisplayName: "MyNode",
	}
	nd.OnResolveAnnounceInfo = func(AnnounceEntry) (AnnounceInfoData, bool) {
		return AnnounceInfoData{OpStr: "Unknown", KeysUnknown: true}, true
	}
	var requested string
	nd.OnRequestKeys = func(hash string) { requested = hash }

	nd.showAnnounceDetailFor(ann)

	ov := nd.listSlotOverlay
	if ov == nil {
		t.Fatal("a node announce with unreachable keys must show the Keys Unknown dialog")
	}
	if got := ov.Dialog().GetTitle(); got != "Keys Unknown" {
		t.Fatalf("dialog title = %q, want %q (Network.py:381)", got, "Keys Unknown")
	}

	// Python's dialog_pile focuses its first selectable child, the button row,
	// whose first button is Request keys.
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	k := nd.keysUnknown
	if k == nil {
		t.Fatal("dialog did not retain its Request keys button")
	}
	k.request.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), setFocus)

	if requested != nodeHash {
		t.Errorf("Request keys asked for %q, want the announce source hash %q", requested, nodeHash)
	}
	if got := k.message.text; got != "\nKeys requested from network\n" {
		t.Errorf("requested message = %q, want %q (Network.py:351)", got, "\nKeys requested from network\n")
	}
	if nd.listSlotOverlay == nil {
		t.Error("Request keys must leave the dialog showing its confirmation")
	}

	// Close restores the list and leaves the info-view state.
	k.closeBtn.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), setFocus)
	if nd.listSlotOverlay != nil {
		t.Error("Close must dismiss the Keys Unknown dialog")
	}
	if nd.inInfoView {
		t.Error("closing the dialog must leave the info view")
	}
}

// TestAnnounceInfoKeysKnownStillShowsDetail is the complement: a node whose
// operator identity IS recallable renders the normal AnnounceInfo, not the
// dialog (Python only catches the KeyError).
func TestAnnounceInfoKeysKnownStillShowsDetail(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ann := AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		SourceHash:  strings.Repeat("2", 32),
		AppData:     "MyNode",
		Type:        "node",
		DisplayName: "MyNode",
	}
	nd.OnResolveAnnounceInfo = func(AnnounceEntry) (AnnounceInfoData, bool) {
		return AnnounceInfoData{OpStr: "My Operator", OpHash: strings.Repeat("1", 32)}, true
	}

	nd.showAnnounceDetailFor(ann)
	if nd.listSlotOverlay != nil {
		t.Error("a recallable node must render AnnounceInfo, not the Keys Unknown dialog")
	}
	if !nd.inInfoView {
		t.Error("a recallable node must enter the announce info view")
	}
}
