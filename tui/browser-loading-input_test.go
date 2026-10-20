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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// TestBrowserShortcutsWorkDuringLoading pins that the browser page still
// receives keys while the centered "Retrieving" body is up. showLoading swaps
// the focused bd.content out of bd.body, so it must move focus onto bd.loading
// when bd.content held it: bodyPages dispatches a key only to a page that is
// both visible and HasFocus (body-pages.go:86), and a page with no focused child
// would drop every shortcut. showContent moves focus back to bd.content when
// bd.loading held it. Python's BrowserFrame.keypress (Browser.py:21) always runs
// for the browser region, so nomadnet stays responsive during a fetch.
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
	// "Retrieving" is up, or bodyPages drops every key.
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
