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
)

// TestNavKeyAfterSetContentNoPanic pins the nav-state invariant of a page body
// that was never rendered from markup: SetContent (the fetch-failure callback)
// swaps the loading body out via showContent and re-shows bd.content with the
// error text, but does NOT run renderPage/initNavState, so the per-line nav
// model stays at its zero value (bd.lineCursors nil, bd.focusLine 0). An arrow
// key on that page must not index bd.lineCursors[0] with len==0 and crash the
// whole app.
//
// The same applies to Disconnect (C-w), which sets the "Disconnected." text on
// bd.content without renderPage. Both must reset the nav state to "no selectable
// line" (focusLine = -1) so the existing handleNavKey empty-page guard consumes
// the arrow keys instead of indexing an empty slice.
func TestNavKeyAfterSetContentNoPanic(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 4)

	// A fetch is in flight, then it fails (OnBrowserError -> SetContent). SetContent
	// swaps the loading body out and re-shows bd.content with the error text, but
	// (being the failure path, not renderPage) it does not rebuild the nav model.
	bd.showLoading("dead.node")
	bd.SetContent("[red]timed out[-]")

	app.SetFocus(bd.content)
	if !bd.content.HasFocus() {
		t.Fatal("bd.content not focused after SetContent")
	}
	assertNavKeysNoPanic(t, bd, "after SetContent (fetch failure)")
}

// TestNavKeyAfterDisconnectNoPanic pins the same invariant for the Disconnect
// path (C-w), which sets the "Disconnected." text on bd.content without
// renderPage.
func TestNavKeyAfterDisconnectNoPanic(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 4)

	bd.showLoading("dead.node:2")
	bd.Disconnect()

	app.SetFocus(bd.content)
	if !bd.content.HasFocus() {
		t.Fatal("bd.content not focused after Disconnect")
	}
	assertNavKeysNoPanic(t, bd, "after Disconnect (C-w)")
}

// assertNavKeysNoPanic drives each arrow + home/end/pg key through handleInput
// and fails the test if any of them panics or if the nav state is left in an
// indexable-but-empty inconsistency.
func assertNavKeysNoPanic(t *testing.T, bd *BrowserDisplay, where string) {
	t.Helper()
	// The error/disconnected page has no selectable lines, so the nav model
	// must report that: focusLine < 0 (or an empty lineCursors slice the guard
	// also accepts). If neither holds, the very first arrow key would index an
	// empty slice and panic.
	if bd.focusLine >= 0 && len(bd.lineCursors) == 0 {
		t.Fatalf("%v: inconsistent nav state focusLine=%v len(lineCursors)=0 — "+
			"an arrow key would index out of range", where, bd.focusLine)
	}
	for _, kk := range []tcell.Key{
		tcell.KeyLeft, tcell.KeyRight, tcell.KeyUp, tcell.KeyDown,
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyEnter,
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%v: handleInput(%v) panicked: %v", where, kk, r)
				}
			}()
			bd.handleInput(key(kk, 0))
		}()
	}
}
