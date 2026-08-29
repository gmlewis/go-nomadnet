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
	"github.com/rivo/tview"
)

// msgOpButtonRow returns the AnnounceInfo button-row Columns from the network
// display's list slot (the focused pile item the button handlers live in).
func msgOpButtonRow(t *testing.T, nd *NetworkDisplay) *pileFiller {
	t.Helper()
	btnRow, ok := nd.listBox.GetItem(0).(*pileFiller)
	if !ok {
		t.Fatalf("expected pileFiller in listBox")
	}
	return btnRow
}

// pressMsgOp focuses the Msg Op button (Right ×2 from the Back button) and
// activates it with Enter, mirroring a user keyboard press on the AnnounceInfo
// button row.
func pressMsgOp(t *testing.T, nd *NetworkDisplay, app *App) {
	t.Helper()
	btnRow := msgOpButtonRow(t, nd)
	handler := btnRow.InputHandler()
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	for range 2 {
		handler(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), setFocus)
	}
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), setFocus)
}

// TestAnnounceInfoMsgOpTargetsOperatorLXMF pins the AnnounceInfo "Msg Op"
// button against Python's msg_op (Network.py:146-166): it must fire the Msg Op
// callback with the operator's LXMF destination hash (AnnounceInfoData.OpHash,
// the "lxmf.delivery" hash derived from the announced node's recallable
// identity), NOT the node announce's own source hash, and must pop the left
// pane back to the announce stream (show_announce_stream).
func TestAnnounceInfoMsgOpTargetsOperatorLXMF(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	opHash := strings.Repeat("1", 32)
	nodeHash := strings.Repeat("2", 32)
	ann := AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		SourceHash:  nodeHash,
		AppData:     "MyNode",
		Type:        "node",
		DisplayName: "MyNode",
	}
	nd.OnResolveAnnounceInfo = func(a AnnounceEntry) (AnnounceInfoData, bool) {
		return AnnounceInfoData{
			DisplayStr: "MyNode",
			TrustStr:   "Unknown",
			TrustStyle: "list_unknown",
			OpStr:      "My Operator",
			OpHash:     opHash,
		}, true
	}

	var gotOpHash string
	calls := 0
	nd.OnMsgOp = func(hash string) {
		calls++
		gotOpHash = hash
	}

	nd.showAnnounceDetailFor(ann)
	if !nd.inInfoView {
		t.Fatal("showAnnounceDetailFor did not enter info view")
	}
	pressMsgOp(t, nd, app)

	if calls != 1 {
		t.Fatalf("OnMsgOp fired %v times, want 1", calls)
	}
	if gotOpHash != opHash {
		t.Errorf("OnMsgOp target = %q, want the operator LXMF hash %q", gotOpHash, opHash)
	}
	if gotOpHash == nodeHash {
		t.Errorf("OnMsgOp target = the node announce source hash %q, want the operator LXMF address", gotOpHash)
	}
	if nd.inInfoView {
		t.Error("Msg Op did not return to the announce stream view")
	}
}

// TestMsgOpCreatesAndOpensOperatorConversation pins the fixed behavior: acting
// on Msg Op performs the same action as the Conversations page's "New
// Conversation on this address and open it" path. The callback is wired the way
// the production wiring does it — create the conversation on the target (the
// operator's LXMF address), refresh the list, open it with
// DisplayConversation, and switch to the Conversations page. Before this
// path existed, Msg Op switched to the Conversations page with no entry
// created and no conversation opened ("No conversation selected").
func TestMsgOpCreatesAndOpensOperatorConversation(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	app.Main.SetDisplay("network", nd.Widget())
	cd := NewConversationsDisplay(app, nil)
	app.Main.SetDisplay("conversations", cd.Widget())

	opHash := strings.Repeat("7", 32)
	nodeHash := strings.Repeat("6", 32)
	ann := AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		SourceHash:  nodeHash,
		AppData:     "MyNode",
		Type:        "node",
		DisplayName: "MyNode",
	}
	nd.OnResolveAnnounceInfo = func(a AnnounceEntry) (AnnounceInfoData, bool) {
		return AnnounceInfoData{OpStr: "My Operator", OpHash: opHash}, true
	}

	// Stub of the wiring-layer Msg Op handler: the app layer creates the
	// conversation (the "New Conversation on this address" path), then the
	// wiring refreshes, opens, and switches pages.
	nd.OnMsgOp = func(hash string) {
		cd.SetConversations([]ConversationInfo{
			{SourceHash: hash, DisplayName: "My Operator", TrustLevel: "unknown"},
		})
		cd.DisplayConversation(hash)
		app.Main.SelectPage("conversations")
	}

	nd.showAnnounceDetailFor(ann)
	pressMsgOp(t, nd, app)

	if app.Main.activePage != "conversations" {
		t.Errorf("activePage = %q, want %q (the Conversations page)", app.Main.activePage, "conversations")
	}
	if cd.currentWidget == nil {
		t.Fatal("no conversation opened on the Conversations page")
	}
	if cd.currentWidget.source != opHash {
		t.Errorf("open conversation = %q, want the operator LXMF hash %q", cd.currentWidget.source, opHash)
	}
}
