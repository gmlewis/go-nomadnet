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

// focusTabBarFixture builds a ConversationsDisplay with one trusted
// conversation and NO untrusted ones — the live state where the Trusted tab
// was previously unreachable by arrow keys (empty untrusted list).
func focusConvFixture(t *testing.T) (*ConversationsDisplay, *ConversationInfo) {
	t.Helper()
	app := newTestApp()
	conv := &ConversationInfo{
		SourceHash:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DisplayName: "Friend",
		TrustLevel:  "trusted",
		LastTime:    time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
	}
	cd := NewConversationsDisplay(app, []ConversationInfo{*conv})
	return cd, conv
}

// pressKeyUp cds handleInput wrapper for readability, returning the resulting
// (possibly nil) event.
func pressKeyUp(cd *ConversationsDisplay) *tcell.EventKey {
	return cd.handleInput(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
}

// TestConversationsTabReachableFromEmptyList pins the arrow-key path to the
// Trusted conversation selector (bug report: with the Untrusted tab showing an
// EMPTY list, arrow keys could not reach the tab bar at all — Python's
// ilb.body_is_empty branch jumps an empty-list Up straight to the menubar,
// Conversations.py:108-111, which the Go port copied). This is a deliberate
// Go enhancement on top of that Python behavior: Up from an empty list now
// performs the SAME Pile traversal a non-empty list gets (checkbox → tab bar),
// and only a further Up from the tab bar reaches the menubar.
func TestConversationsTabReachableFromEmptyList(t *testing.T) {
	t.Parallel()
	cd, _ := focusConvFixture(t)

	// Land on the empty Untrusted tab.
	cd.SetShowTrusted(false)
	if got := cd.list.GetItemCount(); got != 0 {
		t.Fatalf("precondition: untrusted list = %v items, want 0", got)
	}
	if cd.pileHasItem(cd.showBlockedCheckbox) == false {
		t.Fatal("precondition: untrusted layout must include the show-blocked checkbox")
	}
	cd.app.SetFocus(cd.ilb)

	// Up from the empty list: the checkbox (below the tab bar in the untrusted
	// pile), NOT a jump to the menubar.
	if got := pressKeyUp(cd); got != nil {
		t.Fatalf("Up from an empty untrusted list was not consumed (got %v)", got)
	}
	if got := cd.app.GetFocus(); got != tview.Primitive(cd.showBlockedCheckbox) {
		t.Fatalf("focus after Up from empty list = %T, want the Show-blocked checkbox", got)
	}

	// Up again: the tab bar (the page's current tab keeps focus).
	if got := pressKeyUp(cd); got != nil {
		t.Fatalf("Up from the checkbox was not consumed (got %v)", got)
	}
	if got := cd.app.GetFocus(); !isButton(got, "Untrusted (0)") {
		t.Fatalf("focus after Up from checkbox = %T (%v), want the tab bar's focused tab", got, cd.app.GetFocus())
	}

	// Left moves to the Trusted tab — the arrow-key path the user asked for.
	cd.handleInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if got := cd.app.GetFocus(); !isButton(got, "Trusted (1)") {
		t.Fatalf("focus after Left = %T, want the Trusted tab", got)
	}
}

// TestConversationsTabReachableFromNonEmptyList pins the Python-parity A6
// behavior that must NOT regress: Up at the top of a NON-empty trusted list
// lands on the tab bar.
func TestConversationsTabReachableFromNonEmptyList(t *testing.T) {
	t.Parallel()
	cd, _ := focusConvFixture(t)
	cd.SetShowTrusted(true)

	cd.app.SetFocus(cd.ilb)
	if got := pressKeyUp(cd); got != nil {
		t.Fatalf("Up from the top of a non-empty list was not consumed (got %v)", got)
	}
	if got := cd.app.GetFocus(); !isButton(got, "Trusted (1)") {
		t.Fatalf("focus after Up = %T, want the Trusted tab", got)
	}

	// And a second Up from the tab bar still reaches the menubar.
	if got := pressKeyUp(cd); got != nil {
		t.Fatalf("Up from the Trusted tab was not consumed (got %v)", got)
	}
}

// TestBodyListAtTopEmptyConversationsList pins the dispatcher-side half of the
// fix: bodyListAtTop must report FALSE for the conversations list (even an
// EMPTY one), so the app-level capture defers the Up to the page's pile
// traversal instead of collapsing straight to the menu.
func TestBodyListAtTopEmptyConversationsList(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil) // empty trusted AND untrusted
	cd.SetShowTrusted(false)

	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	app.SetFocus(cd.ilb)
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Fatalf("precondition: focus = %T, want the conversations IndicativeListBox", got)
	}
	if got := md.bodyListAtTop(); got {
		t.Fatal("bodyListAtTop = true for the empty conversations list; the page must own Up so the tab bar stays reachable")
	}
}
