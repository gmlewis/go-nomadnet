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

// Regression tests for the 2026-09-01 fleet radio-group bugs (#6, #7): the
// Peer Info dialog must open with the radio group reflecting the entry's
// ACTUAL trust/delivery state (Python passes explicit states,
// Conversations.py:903-909), and the New Conversation dialog opens with
// exactly one pre-checked radio (owner-requested deviation from Python's
// two-checked urwid quirk — see showNewConversationDialog).

// fireDialogKey sends one key to the slot dialog's focused primitive through
// the same dispatch tview uses (wireDialogNav captures live on the items).
func fireDialogKey(t *testing.T, app *App, key tcell.Key) {
	t.Helper()
	p := app.GetFocus()
	if p == nil {
		t.Fatalf("no focused primitive to receive %v", key)
	}
	h := p.InputHandler()
	if h == nil {
		t.Fatalf("focused primitive %T has no InputHandler", p)
	}
	h(tcell.NewEventKey(key, 0, tcell.ModNone), func(pr tview.Primitive) { app.SetFocus(pr) })
}

// TestPeerInfoDialogShowsEntryTrust is the regression test for fleet bug #6:
// a Trusted entry opened with Ctrl-e rendered "(X) Untrusted" — the firstTrue
// constructor flag forced Untrusted checked (first radio, empty group) and
// Trusted unchecked, and saving the dialog silently DOWNGRADED the peer to
// Untrusted. Python's dialog checks exactly one radio: the entry's trust
// (Conversations.py:903-909, explicit state= arguments).
func TestPeerInfoDialogShowsEntryTrust(t *testing.T) {
	t.Parallel()

	hooks := PeerInfoDialogHooks{IsKnown: func(string) bool { return true }}
	cases := []struct {
		name     string
		entry    PeerInfoEntry
		checked  string
		unwanted []string
	}{
		{
			name:     "trusted entry",
			entry:    PeerInfoEntry{SourceHash: "2a6105f57145860441a62fe3b2a1352c", TrustLevel: TrustTrusted, PreferredDelivery: "direct"},
			checked:  "(X) Trusted",
			unwanted: []string{"(X) Untrusted", "(X) Unknown"},
		},
		{
			name:     "untrusted entry",
			entry:    PeerInfoEntry{SourceHash: "2a6105f57145860441a62fe3b2a1352c", TrustLevel: TrustUntrusted, PreferredDelivery: "direct"},
			checked:  "(X) Untrusted",
			unwanted: []string{"(X) Unknown", "(X) Trusted"},
		},
		{
			name:     "unknown entry",
			entry:    PeerInfoEntry{SourceHash: "2a6105f57145860441a62fe3b2a1352c", TrustLevel: TrustUnknown, PreferredDelivery: "direct"},
			checked:  "(X) Unknown",
			unwanted: []string{"(X) Untrusted", "(X) Trusted"},
		},
		{
			name:     "propagated delivery entry",
			entry:    PeerInfoEntry{SourceHash: "2a6105f57145860441a62fe3b2a1352c", TrustLevel: TrustTrusted, PreferredDelivery: "propagated"},
			checked:  "(X) Use propagation nodes",
			unwanted: []string{"(X) Deliver directly"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp()
			cd := NewConversationsDisplay(app, nil)
			cd.ShowPeerInfoDialog(tc.entry, hooks, nil)
			rows := contentRows(t, cd, 80, 24)
			if !containsRow(rows, tc.checked) {
				t.Errorf("radio %q not checked for entry %v (rows: %v)", tc.checked, tc.name, rows)
			}
			for _, unwanted := range tc.unwanted {
				if containsRow(rows, unwanted) {
					t.Errorf("radio %q unexpectedly checked for %v — the radio group is not reflecting the entry state", unwanted, tc.name)
				}
			}
		})
	}
}

// TestPeerInfoSavePreservesTrust pins the save path of the peer-info dialog:
// opening a Trusted+propagated entry and pressing Save without touching
// anything must save the entry UNCHANGED (the read order matches Python's
// confirmed(): unknown first, then trusted, default Untrusted — only correct
// when the radios reflect the entry state, which the firstTrue flag broke).
func TestPeerInfoSavePreservesTrust(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	entry := PeerInfoEntry{
		SourceHash:        "2a6105f57145860441a62fe3b2a1352c",
		TrustLevel:        TrustTrusted,
		PreferredDelivery: "propagated",
		Pinned:            true,
		Notes:             "note",
	}
	var saved PeerInfoEntry
	savedCount := 0
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{IsKnown: func(string) bool { return true }}, func(e PeerInfoEntry) {
		saved = e
		savedCount++
	})
	rows := contentRows(t, cd, 80, 24)
	if !anyRowContains(rows, "(X) Trusted") {
		t.Fatalf("trusted radio not pre-checked — cannot exercise the save path")
	}
	if !anyRowContains(rows, "(X) Use propagation nodes") {
		t.Fatalf("propagated radio not pre-checked (delivery state lost)")
	}

	// Walk the wireDialogNav chain to the Save button and fire it. The peer
	// info items: eName, eCopy, Untrusted, Unknown, Trusted, Direct,
	// Propagated, Pin, Notes, Ping, Block, LXMF, Save, Back.
	for range 12 {
		fireDialogKey(t, app, tcell.KeyTab)
	}
	fireDialogKey(t, app, tcell.KeyEnter) // Save
	if savedCount != 1 {
		t.Fatalf("Save fired %d times, want 1", savedCount)
	}
	if saved.TrustLevel != TrustTrusted {
		t.Errorf("saved trust = %v, want Trusted — an untouched save must not change the trust level", saved.TrustLevel)
	}
	if saved.PreferredDelivery != "propagated" {
		t.Errorf("saved delivery = %q, want propagated", saved.PreferredDelivery)
	}
	if !saved.Pinned || saved.Notes != "note" {
		t.Errorf("saved entry lost pin/notes: %+v", saved)
	}
}

// TestNewConversationDialogSingleCheckedRadio is the regression test for fleet
// bug #7: the dialog must open with exactly ONE radio checked. (Python's
// urwid "first True" quirk checks both Untrusted and Unknown — verified
// True/True/False against the installed urwid — but the owner asked for one;
// this pins the deliberate deviation, see showNewConversationDialog.)
func TestNewConversationDialogSingleCheckedRadio(t *testing.T) {
	t.Parallel()
	rows := renderNewConversationDialog(t, false)

	if !containsRow(rows, "(X) Unknown") {
		t.Errorf("(X) Unknown missing — the default radio must be pre-checked")
	}
	if containsRow(rows, "(X) Untrusted") {
		t.Errorf("(X) Untrusted must NOT be pre-checked (only one radio may show (X))")
	}
	if containsRow(rows, "(X) Trusted") {
		t.Errorf("(X) Trusted must NOT be pre-checked")
	}
}

// TestNewConversationCreateTrustSelection pins the Create handler's trust
// mapping end to end: focusing the "Trusted" radio, checking it, and pressing
// Create must report "trusted" (the wiring stores TrustTrusted); a no-toggle
// Create reports "unknown" (Python's confirmed() order: unknown, trusted,
// default untrusted).
func TestNewConversationCreateTrustSelection(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var gotTrust string
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool {
		gotTrust = trust
		return true
	})

	// The dialog items are eID, eName, Untrusted, Unknown, Trusted, Create,
	// Back (wireDialogNav order). Tab ×4 lands on Trusted, Enter checks it
	// (and unchecks Unknown), Tab moves to Create, Enter fires Create.
	for range 4 {
		fireDialogKey(t, app, tcell.KeyTab)
	}
	fireDialogKey(t, app, tcell.KeyEnter) // check Trusted
	rows := contentRows(t, cd, 80, 24)
	if !containsRow(rows, "(X) Trusted") {
		t.Fatalf("(X) Trusted not checked after selection (rows: %v)", rows)
	}
	if containsRow(rows, "(X) Unknown") {
		t.Errorf("Unknown still checked after selecting Trusted — mutual exclusion broken")
	}

	fireDialogKey(t, app, tcell.KeyTab) // → Create
	fireDialogKey(t, app, tcell.KeyEnter)
	if gotTrust != "trusted" {
		t.Errorf("Create after selecting Trusted reported trust=%q, want %q", gotTrust, "trusted")
	}
}
