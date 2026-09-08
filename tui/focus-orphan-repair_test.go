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

	"github.com/gdamore/tcell/v2"
)

// setupNetworkTestApp initializes an App with MainDisplay, NetworkDisplay mounted,
// and screen size established so widgets layout properly.
func setupNetworkTestApp(t *testing.T) (*App, *NetworkDisplay) {
	t.Helper()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DisplayName: "Node Alpha"},
		{SourceHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", DisplayName: "Node Beta"},
	})
	app.Main.SetDisplay("network", nd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("network")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	return app, nd
}

// TestZombieUrwidButtonOnSetLeftList verifies that replacing the left list
// when an item inside it is focused does not leave a detached zombie button
// as app.GetFocus() with root.HasFocus() == false.
func TestZombieUrwidButtonOnSetLeftList(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app, nd := setupNetworkTestApp(t)

	// Put a button into the left list and focus it.
	btn := NewUrwidButton("Sample Action")
	nd.setLeftList(btn, "Test List")
	app.SetFocus(btn)

	root := app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("precondition: root must have focus when btn is focused, got rootHasFocus=%v", root != nil && root.HasFocus())
	}
	if app.GetFocus() != btn {
		t.Fatalf("precondition: app focus must be btn, got %T", app.GetFocus())
	}

	// Mutate the left list (e.g. refreshNodesView or switching views).
	nd.setLeftList(nd.nodesView(), "Saved Nodes")

	// Post-condition: root MUST still have focus, and app focus must NOT be the detached button.
	root = app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("after setLeftList: root.HasFocus() = false (zombie focus stranded on %T)", app.GetFocus())
	}
	if app.GetFocus() == btn {
		t.Fatalf("after setLeftList: app focus is still the detached button!")
	}

	// Any subsequent key dispatched to MainDisplay should produce zero invariant violations.
	app.Main.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if len(*dumps) > 0 {
		t.Fatalf("unexpected focus invariant dump after setLeftList: %v", (*dumps)[0])
	}
}

// TestCloseLocalPeerStatusRepairsFocus verifies that closing the local peer status
// dialog (which contains an "OK" UrwidButton) properly restores focus to the live
// tree without leaving focus stranded on the detached OK button.
func TestCloseLocalPeerStatusRepairsFocus(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app, nd := setupNetworkTestApp(t)

	nd.ShowLocalPeerStatus("Announce sent successfully", 1)
	if nd.statusInPeerSlot == nil {
		t.Fatal("ShowLocalPeerStatus did not set statusInPeerSlot")
	}

	root := app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("precondition: root must have focus after ShowLocalPeerStatus, got rootHasFocus=%v", root != nil && root.HasFocus())
	}
	focused := app.GetFocus()
	if focused == nil {
		t.Fatalf("precondition: focus should not be nil")
	}

	// Close the status dialog (mirrors user activating OK button).
	nd.CloseLocalPeerStatus()

	// Invariant: root.HasFocus() must be true, focus must not be detached.
	root = app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("after CloseLocalPeerStatus: root.HasFocus() = false (focus stranded on %T)", app.GetFocus())
	}
	if app.GetFocus() == focused && focused != nil {
		t.Fatalf("after CloseLocalPeerStatus: app focus remained on closed dialog primitive %T", app.GetFocus())
	}

	// Dispatch a key through handleInput; no invariant dump should occur.
	app.Main.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if len(*dumps) > 0 {
		t.Fatalf("unexpected focus invariant dump after CloseLocalPeerStatus: %v", (*dumps)[0])
	}
}

// TestShowLocalPeerFromNodeInfoBackRepairsFocus verifies that clicking/activating
// "Back" on NodeInfoDisplay repairs focus so it does not remain stranded on the
// now-detached "Back" UrwidButton.
func TestShowLocalPeerFromNodeInfoBackRepairsFocus(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app, nd := setupNetworkTestApp(t)

	nd.ShowNodeInfo(NodeInfoData{HasNode: false})
	if nd.nodeInfo == nil {
		t.Fatal("ShowNodeInfo did not set nodeInfo")
	}

	// Focus the "Back" button in NodeInfoDisplay.
	backBtn := nd.nodeInfo.backBtn
	if backBtn == nil {
		t.Fatal("NodeInfoDisplay backBtn is nil")
	}
	app.SetFocus(backBtn)

	root := app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("precondition: root must have focus when backBtn is focused, got rootHasFocus=%v", root != nil && root.HasFocus())
	}
	if app.GetFocus() != backBtn {
		t.Fatalf("precondition: app focus must be backBtn, got %T", app.GetFocus())
	}

	// Activating Back invokes nd.ShowLocalPeer().
	nd.ShowLocalPeer()

	// Invariant: root.HasFocus() must be true, focus must not be stranded on the old backBtn.
	root = app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("after ShowLocalPeer: root.HasFocus() = false (focus stranded on %T)", app.GetFocus())
	}
	if app.GetFocus() == backBtn {
		t.Fatalf("after ShowLocalPeer: focus remained stranded on detached backBtn")
	}

	// Dispatch a key; no violation dump.
	app.Main.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if len(*dumps) > 0 {
		t.Fatalf("unexpected focus invariant dump after ShowLocalPeer: %v", (*dumps)[0])
	}
}

// TestFocusBodyRejectsDetachedLastBodyFocus verifies that if md.lastBodyFocus was
// pointing to an orphaned/detached primitive, FocusBody falls back to contentArea
// rather than setting focus on a zombie and escalating immediately to the menu.
func TestFocusBodyRejectsDetachedLastBodyFocus(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app, _ := setupNetworkTestApp(t)

	// Create an orphan button never added to the tree.
	orphanBtn := NewUrwidButton("Orphan")
	app.Main.mu.Lock()
	app.Main.lastBodyFocus = orphanBtn
	app.Main.mu.Unlock()

	// Call FocusBody. It should recognize orphanBtn is not in root, clear lastBodyFocus,
	// and fall back to contentArea (landing on a valid body child in network).
	app.Main.FocusBody()

	root := app.GetRoot()
	if root == nil || !root.HasFocus() {
		t.Fatalf("after FocusBody with detached lastBodyFocus: root.HasFocus() = false")
	}
	if app.GetFocus() == orphanBtn {
		t.Fatalf("after FocusBody: focus was set to detached orphanBtn")
	}
	if app.Main.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want \"body\"", app.Main.focusRegion)
	}
	if app.GetFocus() == app.Main.menuBar {
		t.Errorf("focus escalated to menuBar unnecessarily; should have fallen back to body contentArea")
	}
	if len(*dumps) > 0 {
		t.Fatalf("unexpected focus invariant dump: %v", (*dumps)[0])
	}
}

// TestKeyDispatchSurvivesMutationWithoutViolation verifies that after multiple panel
// mutations in NetworkDisplay, dispatching real key events through the root input
// handler produces zero focus invariant violations and the active widget receives keys.
func TestKeyDispatchSurvivesMutationWithoutViolation(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app, nd := setupNetworkTestApp(t)

	// Step 1: Open NodeInfo, simulate pressing "Back" to return to local peer.
	nd.ShowNodeInfo(NodeInfoData{HasNode: false})
	if nd.nodeInfo != nil && nd.nodeInfo.backBtn != nil {
		app.SetFocus(nd.nodeInfo.backBtn)
	}
	nd.ShowLocalPeer()
	app.Main.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))

	// Step 2: Open status dialog, close it.
	nd.ShowLocalPeerStatus("Status message", 1)
	nd.CloseLocalPeerStatus()
	app.Main.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))

	// Step 3: Refresh nodes view with item focused.
	btn := NewUrwidButton("Btn")
	nd.setLeftList(btn, "Title")
	app.SetFocus(btn)
	nd.refreshNodesView()
	app.Main.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))

	if len(*dumps) > 0 {
		t.Fatalf("captured %d focus invariant violations during key dispatch: %v", len(*dumps), (*dumps)[0])
	}
}
