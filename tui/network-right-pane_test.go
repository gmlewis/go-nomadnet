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
	"github.com/rivo/tview"
)

// dispatchApp mimics tview's Application.input key dispatch (application.go
// :415-451): the app-level input capture (MainDisplay.handleInput) runs FIRST
// and may consume the event (return nil); if it returns the event, root.
// InputHandler() is invoked with setFocus=Application.SetFocus. Tests that
// drive keys through md.Root() (the inner Flex) but skip the DialogManager's
// tview.Pages root — which App.SetRoot mounts as the real a.root — miss bugs
// that live in that outer Pages layer. This helper dispatches through the
// actual app root (the DialogManager Pages) the way Application.input does.
func dispatchApp(t *testing.T, app *App, ev *tcell.EventKey) {
	t.Helper()
	if capture := app.GetInputCapture(); capture != nil {
		ev = capture(ev)
		if ev == nil {
			return
		}
	}
	root := app.Dialogs.Pages()
	if root == nil {
		t.Fatal("nil root (DialogManager.Pages)")
	}
	handler := root.InputHandler()
	if handler == nil {
		t.Fatal("root has no InputHandler")
	}
	handler(ev, func(p tview.Primitive) { app.SetFocus(p) })
}

// TestNetworkRightEntersBrowserPane reproduces tmux-suite bug B1: after
// connecting to a node from the Announce Info view, focus returns to the
// Announce Stream list, and pressing Right must move focus into the right-hand
// browser pane so subsequent Downs scroll the rendered page and follow links
// — not traverse the left list. The tmux suite recorded 0 browser screenfuls
// and 0 links followed across 7 connects in gonomadnet (vs 13 screenfuls / 40
// links in nomadnet) because Right never left the announce list.
//
// This test drives the FULL connect flow (open Announce Info, Right to the
// Connect button, Enter) and dispatches every key through the real app-level
// path (app capture + root) that Application.input uses — not directly into
// mainCols. The plain-list Right→browser routing is already pinned by
// TestNetworkListRightMovesToBrowser; B1 is specifically the post-Connect
// announce-stream state, so the test rebuilds that state faithfully.
func TestNetworkRightEntersBrowserPane(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	md := app.Main

	// One node announce so the Announce Stream list has a focusable row.
	ann := nodeAnnounce("TestNode")
	nd := NewNetworkDisplay(app, []AnnounceEntry{ann}, nil)

	// onNavigate fires on Connect; the real wiring loads the browser page. For
	// this focus test a no-op is sufficient — B1 is about where focus lands
	// after Connect, not the rendered page.
	nd.SetNavigateCallback(func(string string) {})

	md.SetDisplay("network", nd.Widget())
	md.SelectPage("network")
	md.Root().SetRect(0, 0, 200, 50)

	// Mount the real app root the way the live app does: App.SetRoot installs
	// the DialogManager's tview.Pages (wrapping md.frame) as Application.root
	// and descends focus into the visible page. dispatchApp dispatches through
	// that same Pages root, so the test exercises the full live dispatch chain
	// (app capture -> DialogManager Pages -> frame -> bodyPages -> mainCols)
	// rather than stopping at md.Root() one layer short.
	app.SetRoot()

	// Show the Announce Stream (showingNodes starts true; toggle to the stream).
	nd.toggleList() // showingNodes -> false, left list = announce stream, focused.

	// Open the Announce Info detail for the node (as Enter on the list does).
	nd.showAnnounceDetail(0)
	if !nd.inInfoView {
		t.Fatalf("setup: showAnnounceDetail did not open info view")
	}

	// Announce Info: button row focus starts on Back(0). Right -> Connect(2).
	dispatchApp(t, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if !nd.inInfoView {
		t.Fatalf("Right on Announce Info closed the info view (should stay open)")
	}

	// Enter on Connect triggers connectToNode -> showAnnounceStream (closes the
	// info view, restores the announce stream list, focuses it) + onNavigate.
	dispatchApp(t, app, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if nd.inInfoView {
		t.Fatalf("after Connect, info view should be closed")
	}

	// Post-Connect state: left column NOT self-managing, focus on the list.
	if nd.mainCols.selfManaging[0] {
		t.Fatalf("after Connect, left column should not be self-managing")
	}
	if !nd.leftPanel.HasFocus() {
		t.Fatalf("after Connect, left panel should have focus, got %T", app.GetFocus())
	}

	// THE BUG: Right must move focus from the announce list to the browser pane.
	dispatchApp(t, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))

	if got := nd.mainCols.FocusIndex(); got != 1 {
		t.Errorf("after Right, mainCols focus = %d, want 1 (browser pane); focus=%T", got, app.GetFocus())
	}
	if !nd.browser.Widget().HasFocus() {
		t.Errorf("after Right, browser pane should have focus, got %T (leftPanel hasFocus=%v)", app.GetFocus(), nd.leftPanel.HasFocus())
	}

	// B1 core: focus must be EXCLUSIVE. The left column (an Announce-Stream
	// pileFiller wrapping the list) must NOT retain focus after Right moves
	// focus to the browser. In the live app a.focus is the pileFiller (a
	// container, because pileFiller.moveFocus registers the pile itself with
	// the app via setFocus(p), and tview's Box.Focus delegate chain does not
	// descend further). When app.SetFocus(browser) then calls a.focus.Blur(),
	// pileFiller falls back to Box.Blur, which clears only the pile's own
	// hasFocus flag — NOT the focused list child's. The list keeps
	// hasFocus=true, so BOTH columns have focus and subsequent Downs dispatch
	// to the left list instead of scrolling the browser (0 screenfuls / 0
	// links followed across 7 connects in the tmux suite). pileFiller must
	// blur its focused child so the left column releases focus cleanly.
	if nd.leftPanel.HasFocus() {
		t.Errorf("after Right, left panel must NOT retain focus (B1: pileFiller.Blur did not blur its focused child); leftPanel hasFocus=%v, browser hasFocus=%v",
			nd.leftPanel.HasFocus(), nd.browser.Widget().HasFocus())
	}

	// Consequence check: a Down dispatched after Right must go to the browser,
	// not advance the left list selection. Before the fix the leftover list
	// focus steals the key.
	listBefore := nd.announcesList.List.GetCurrentItem()
	dispatchApp(t, app, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := nd.announcesList.List.GetCurrentItem(); got != listBefore {
		t.Errorf("after Right, Down advanced the left list selection %d->%d (should have gone to the browser)", listBefore, got)
	}
}