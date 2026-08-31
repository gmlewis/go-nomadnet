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

// blockedNodeURL returns a canonical nomadnet node URL made of a full
// 32-hex-char (16-byte) destination hash built from the given rune.
func blockedNodeURL(r rune) string {
	hash := ""
	for range 32 {
		hash += string(r)
	}
	return hash
}

// confirmModalConnect drives the open warning modal from its initial (default)
// Cancel focus: Tab → Connect → Enter. Returns the focus after Tab so tests
// can assert the button order.
func confirmModalConnect(app *App) {
	if got := app.GetFocus(); !isButton(got, "Cancel") {
		app.Dialogs.DismissTop()
		return
	}
	pressKeyThroughFocus(app, tcell.KeyTab) // Cancel → Connect
	pressKeyThroughFocus(app, tcell.KeyEnter)
}

// TestBrowserDisplayLoadURLBlockedNodeWarns pins the Go-only blocked-connect
// guard on BrowserDisplay.LoadURL (no Python SOT counterpart): navigating to a
// blocked destination defers the fetch behind the "blocked node" warning modal
// (default focus = Cancel), Cancel fetches nothing, and an explicit Connect
// proceeds with the original URL. Unblocked destinations navigate directly and
// a nil check hook preserves the historical behavior.
func TestBrowserDisplayLoadURLBlockedNodeWarns(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	blockedHash := blockedNodeURL('a')
	otherHash := blockedNodeURL('b')
	bd := NewBrowserDisplay(app)
	bd.OnBlockedConnectCheck = func(hash string) (string, bool) {
		if hash == blockedHash {
			return "Bad Node", true
		}
		return "", false
	}
	var fetched []string
	bd.OnRetrieveURL = func(url string, _ map[string]string) { fetched = append(fetched, url) }

	// Not blocked → navigates immediately.
	bd.LoadURL(otherHash)
	if len(fetched) != 1 {
		t.Fatalf("unblocked URL fetches = %v, want 1", len(fetched))
	}

	// Blocked → no immediate fetch; modal is up with Cancel focused.
	bd.LoadURL(blockedHash)
	if len(fetched) != 1 {
		t.Fatalf("blocked URL fetched immediately: %v fetches, want 1 (initial only)", len(fetched))
	}
	if !app.Dialogs.Open() {
		t.Fatal("blocked navigation must raise the warning modal")
	}
	if got := app.GetFocus(); !isButton(got, "Cancel") {
		t.Fatalf("modal initial focus = %T, want Cancel", got)
	}

	// Enter on the default (Cancel) → nothing fetched.
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if len(fetched) != 1 {
		t.Fatalf("cancel fetched anyway: %v fetches, want 1", len(fetched))
	}

	// Retry and confirm via the modal's Connect button → original URL loads.
	bd.LoadURL(blockedHash)
	confirmModalConnect(app)
	if len(fetched) != 2 || fetched[1] != blockedHash {
		t.Fatalf("after confirm fetched = %v, want the blocked URL appended", fetched)
	}
	if app.Dialogs.Open() {
		t.Error("warning modal should be dismissed after confirming")
	}
}

// TestBrowserDisplayLoadURLBlockedNilHookUnchanged pins that with no
// OnBlockedConnectCheck wired, LoadURL behaves exactly as before (no dialog,
// immediate navigation).
func TestBrowserDisplayLoadURLBlockedNilHookUnchanged(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	var fetched int
	bd.OnRetrieveURL = func(_ string, _ map[string]string) { fetched++ }
	bd.LoadURL(blockedNodeURL('d'))
	if fetched != 1 {
		t.Fatalf("fetches = %v, want 1", fetched)
	}
	if app.Dialogs.Open() {
		t.Error("no warning modal should open without the guard hook")
	}
}

// TestBrowserDisplayHandleLinkBlockedNode pins the same guard on link clicks
// targeting a blocked node destination: nothing is fetched, no eager history
// entry survives the intercept, and confirming through the modal proceeds with
// the full link flow (eager push + fetch).
func TestBrowserDisplayHandleLinkBlockedNode(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	blockedHash := blockedNodeURL('c')
	bd.OnBlockedConnectCheck = func(hash string) (string, bool) {
		if hash == blockedHash {
			return "Bad Node", true
		}
		return "", false
	}
	var fetched []string
	bd.OnRetrieveURL = func(url string, _ map[string]string) { fetched = append(fetched, url) }

	bd.HandleLink(blockedHash, "")
	if len(fetched) != 0 {
		t.Fatalf("blocked link click fetched immediately: %v fetches, want 0", len(fetched))
	}
	if bd.pendingLinkHist {
		t.Error("blocked link left a pending history entry")
	}
	if !app.Dialogs.Open() {
		t.Fatal("blocked link click must raise the warning modal")
	}

	confirmModalConnect(app)
	if len(fetched) != 1 || fetched[0] != blockedHash {
		t.Fatalf("after confirm fetched = %v, want the blocked link target", fetched)
	}
	if got := bd.CurrentURL(); got != blockedHash {
		t.Errorf("CurrentURL = %q, want %q (eager push on confirm)", got, blockedHash)
	}
}
