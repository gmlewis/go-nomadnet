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

// TestBrowserShortcutsWorkDuringLoading pins the user-reported regression:
// "while a node is 'Retrieving...', nomadnet still fully responds to the
// keyboard navigation whereas gonomadnet.sh does not."
//
// Root cause: showLoading swaps the focused bd.content OUT of bd.body and puts
// the centered "Retrieving" body (bd.loading) in its place, but does NOT move
// focus onto bd.loading. Application.SetFocus only blurs a single leaf, and
// Flex.RemoveItem does not blur the removed child, so after the swap the app's
// focused primitive is still the now-DISCONNECTED bd.content — no child of the
// browser page's widget tree has focus, so bd.Widget().HasFocus() is false.
// bodyPages.InputHandler dispatches a key only to a page that is BOTH visible
// AND HasFocus (body-pages.go:86), so during the whole fetch the browser page
// receives NO key events — the Ctrl-d/C-w/C-u/etc. shortcuts in handleInput
// (browser.go) never run, and the user is stranded on "Retrieving" with a dead
// keyboard. Python's BrowserFrame.keypress (Browser.py:21) always runs for the
// browser region regardless of the loading body, so nomadnet stays responsive.
//
// Fix: showLoading moves focus onto bd.loading when bd.content held it (so the
// browser page keeps a focused child and bodyPages keeps dispatching to it);
// showContent moves focus back onto bd.content when bd.loading held it.
func TestBrowserShortcutsWorkDuringLoading(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 10)

	// Stand the browser page up in a bodyPages exactly as the app does, so the
	// test dispatches keys through the same visible+HasFocus gate the real
	// input chain uses.
	pages := newBodyPages()
	pages.AddPage("browser", bd.Widget(), true, true)

	// The user is on the browser page with the content focused, then triggers a
	// load (Connect / link / C-u): displayURL -> showLoading swaps "Retrieving"
	// in for bd.content.
	app.SetFocus(bd.content)
	if !bd.content.HasFocus() {
		t.Fatal("setup: bd.content should be focused before loading")
	}
	bd.showLoading("cafe1234:/page/index.mu")

	// The browser page's widget tree must STILL have a focused child while
	// "Retrieving" is up — otherwise bodyPages drops every key (the bug).
	if !bd.Widget().HasFocus() {
		t.Errorf("bd.Widget().HasFocus() = false during loading — bodyPages will not dispatch keys to the browser (dead keyboard)")
	}

	// A Ctrl-* shortcut dispatched through bodyPages (the real input chain)
	// must reach the browser's handleInput and fire its callback during loading.
	disconnected := false
	bd.OnDisconnect = func() { disconnected = true }
	pages.InputHandler()(
		tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) },
	)
	if !disconnected {
		t.Errorf("Ctrl-W during loading did not fire OnDisconnect — the browser page did not receive the key (the Retrieving dead-keyboard bug)")
	}

	// When the page renders (showContent swaps bd.content back), focus must
	// return to bd.content so the loaded page is immediately navigable.
	bd.showContent()
	if !bd.content.HasFocus() {
		t.Errorf("bd.content not focused after showContent — focus not restored to the page on load complete")
	}
	if bd.loading != nil {
		t.Errorf("bd.loading not cleared by showContent")
	}
}

// TestBrowserLoadingDoesNotStealListFocus pins the Network-pane invariant: when
// the LEFT list (not the browser pane) holds focus and the user Connects, the
// right pane shows "Retrieving" but showLoading must NOT steal focus from the
// list — the user keeps navigating the announce stream during the fetch
// (matching nomadnet, where the network browser is not focused on Connect).
func TestBrowserLoadingDoesNotStealListFocus(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 10)

	// Something ELSE holds focus (the Network left list), not bd.content.
	other := newFocusRecorder()
	app.SetFocus(other)
	if !other.HasFocus() {
		t.Fatal("setup: other widget should hold focus")
	}
	bd.showLoading("cafe1234:/page/index.mu")

	if !other.HasFocus() {
		t.Errorf("showLoading stole focus to the browser pane — the left list must keep focus during a Network-page Connect (bd.content was not focused)")
	}
	if bd.loading == nil {
		t.Errorf("showLoading did not install the loading body")
	}
	// showContent must also not steal focus when the loading body never had it.
	bd.showContent()
	if !other.HasFocus() {
		t.Errorf("showContent stole focus back to the browser — it must only restore focus the loading body actually held")
	}
}
