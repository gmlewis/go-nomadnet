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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// sendViaNetwork drives one key through the real Network page input path: the
// outer Columns InputHandler (which runs the NetworkDisplay.handleInput input
// capture, then the column/child dispatch). This replicates how tview delivers
// a key to the focused Network page in the live app.
func sendViaNetwork(nd *NetworkDisplay, app *App, ev *tcell.EventKey) {
	h := nd.Widget().InputHandler()
	if h == nil {
		t := testing.T{}
		t.Fatal("Network Widget has no InputHandler")
	}
	h(ev, func(p tview.Primitive) { app.SetFocus(p) })
}

// nodeAnnounce builds a node-type AnnounceEntry (the 4-button Back/Connect/Msg
// Op/Save row) with a valid 32-hex source hash.
func nodeAnnounce(name string) AnnounceEntry {
	return AnnounceEntry{
		Timestamp:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		SourceHash:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
		AppData:     "Node app data",
		Type:        "node",
		DisplayName: name,
	}
}

// TestNetworkAnnounceInfoRightMovesButtonsNotPane pins the fix for the user
// report "on the Announce Info page, the cursor is on Back and I hit RightArrow
// to move to Connect but it doesn't move". The root cause was the outer Network
// Columns (urwidColumns) handling Right/Left BEFORE forwarding to its focused
// child, so it grabbed Right to jump to the browser pane instead of letting the
// nested AnnounceInfo button row move Back→Connect. With the left column marked
// self-managing while the detail view is open, Right forwards to the button row
// and moves Back→Connect; the outer Columns stays on the left pane.
func TestNetworkAnnounceInfoRightMovesButtonsNotPane(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	// Open a node AnnounceInfo directly (replicating showAnnounceDetailFor but
	// keeping a handle on the AnnounceInfo so the test can inspect its button
	// row). A node has 4 buttons: Back(0) Connect(2) MsgOp(4) Save(6); the
	// focusable set is [0,2,4,6].
	ann := nodeAnnounce("NodeOne")
	ai := newAnnounceInfoDisplay(nd, ann, AnnounceInfoData{
		DisplayStr: "<" + ann.SourceHash + ">",
		TrustStr:   "Unknown",
		TrustStyle: "list_unknown",
		OpStr:      "Unknown",
	})
	nd.setLeftList(ai.Widget(), "Announce Info")
	nd.setInfoView(true)
	nd.focusLeftList()

	// Sanity: the detail view marked the left column self-managing.
	if !nd.mainCols.selfManaging[0] {
		t.Fatal("left column not marked self-managing while AnnounceInfo open")
	}

	// The pile's focused (only selectable) item is the button row; its focus is
	// on Back (focusIndex 0).
	buttonRow, ok := ai.pile.focusedItem().(*urwidColumns)
	if !ok {
		t.Fatalf("pile focused item is %T, want *urwidColumns (button row)", ai.pile.focusedItem())
	}
	if buttonRow.FocusIndex() != 0 {
		t.Errorf("button row initial focus = %d, want 0 (Back)", buttonRow.FocusIndex())
	}

	// Right must move Back→Connect, not jump to the browser pane.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))

	if got := buttonRow.FocusIndex(); got != 2 {
		t.Errorf("after Right, button row focus = %d, want 2 (Connect)", got)
	}
	if got := nd.mainCols.FocusIndex(); got != 0 {
		t.Errorf("after Right, outer Columns focus = %d, want 0 (left pane, not browser)", got)
	}

	// Right again moves Connect→Msg Op (focusable index 4).
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := buttonRow.FocusIndex(); got != 4 {
		t.Errorf("after 2nd Right, button row focus = %d, want 4 (Msg Op)", got)
	}

	// Left moves back to Connect (index 2).
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if got := buttonRow.FocusIndex(); got != 2 {
		t.Errorf("after Left, button row focus = %d, want 2 (Connect)", got)
	}
	if got := nd.mainCols.FocusIndex(); got != 0 {
		t.Errorf("after Left, outer Columns focus = %d, want 0 (still left pane)", got)
	}
}

// TestNetworkListRightMovesToBrowser pins the counterpart: when the left pane
// holds a plain list (not a self-managing detail view), Right on it moves focus
// to the browser pane — the existing behavior, which the self-managing change
// must NOT regress. tview.List would otherwise consume Right by advancing its
// current item, so the outer Columns must keep grabbing Right for pane movement
// when the left column is not self-managing.
func TestNetworkListRightMovesToBrowser(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6", DisplayName: "Saved A"},
	})

	// Default view is the saved-nodes list; left column is NOT self-managing.
	if nd.mainCols.selfManaging[0] {
		t.Fatal("left column self-managing on the plain list; should not be")
	}
	nd.focusLeftList()

	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))

	if got := nd.mainCols.FocusIndex(); got != 1 {
		t.Errorf("after Right on list, outer Columns focus = %d, want 1 (browser pane)", got)
	}
}

// TestNetworkBrowserRightStaysInBrowser pins the self-managing browser column:
// once focus is in the right-pane browser, Left/Right belong to the browser's
// part-cursor model (and its Left-at-start focus release), so the outer Columns
// must NOT pane-wrap back to the left list on Right. Before the fix, Right on
// the browser wrapped the Columns focus to the left pane (column 0).
func TestNetworkBrowserRightStaysInBrowser(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6", DisplayName: "Saved A"},
	})

	// Move focus to the browser pane (Right on the list).
	nd.focusLeftList()
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := nd.mainCols.FocusIndex(); got != 1 {
		t.Fatalf("setup: Right did not move to browser, focus=%d", got)
	}

	// The browser column is self-managing: Right must stay in the browser, not
	// wrap back to the left list.
	if !nd.mainCols.selfManaging[1] {
		t.Fatal("browser column not marked self-managing")
	}
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := nd.mainCols.FocusIndex(); got != 1 {
		t.Errorf("after Right in browser, outer Columns focus = %d, want 1 (stay in browser)", got)
	}
	// Left also stays in the browser (the browser owns Left for its part cursor
	// + Left-at-start release), rather than the Columns moving to the left list.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if got := nd.mainCols.FocusIndex(); got != 1 {
		t.Errorf("after Left in browser, outer Columns focus = %d, want 1 (browser owns Left)", got)
	}
}

// TestNetworkAnnounceInfoEscapeStillWorks ensures the self-managing change did
// not break Esc dismissing the AnnounceInfo back to the list (Esc is handled by
// the pileFiller's Esc handler, forwarded as a non-arrow key).
func TestNetworkAnnounceInfoEscapeStillWorks(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	ann := nodeAnnounce("NodeOne")
	ai := newAnnounceInfoDisplay(nd, ann, AnnounceInfoData{
		DisplayStr: "<" + ann.SourceHash + ">",
		TrustStr:   "Unknown",
		TrustStyle: "list_unknown",
		OpStr:      "Unknown",
	})
	nd.setLeftList(ai.Widget(), "Announce Info")
	nd.setInfoView(true)
	nd.focusLeftList()
	if !nd.inInfoView {
		t.Fatal("AnnounceInfo not open")
	}

	// Esc: the page-level handleInput passes it through (inInfoView returns the
	// event), the column handler forwards it (non-arrow) to the left subtree,
	// and the pileFiller's Esc handler dismisses to the announce stream.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if nd.inInfoView {
		t.Errorf("Esc did not dismiss AnnounceInfo (inInfoView still true)")
	}
	if nd.mainCols.selfManaging[0] {
		t.Errorf("Esc did not clear the left column self-managing flag")
	}
}
