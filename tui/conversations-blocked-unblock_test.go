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
)

// TestConversationsDisplayBlockedRowRendersUnblockLabel pins the blocked-row
// rendering (Python _blocked_row_widget, Conversations.py:332-341): the
// ✖-[blocked]-name-<hash> label as the row's main text with NO secondary
// relative-time text, while normal rows keep their regular rendering.
func TestConversationsDisplayBlockedRowRendersUnblockLabel(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	blocked := ConversationInfo{
		SourceHash:  strings.Repeat("e", 32),
		DisplayName: "Eve",
		TrustLevel:  "blocked",
	}
	normal := ConversationInfo{
		SourceHash:  strings.Repeat("a", 32),
		DisplayName: "Friend",
		TrustLevel:  "untrusted",
		LastTime:    time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
	}
	cd := NewConversationsDisplay(app, []ConversationInfo{blocked, normal})
	cd.showTrusted = false
	cd.showBlocked = true
	cd.populateList()

	if got := cd.list.GetItemCount(); got != 2 {
		t.Fatalf("list items = %v, want 2 (blocked + untrusted)", got)
	}
	main0, secondary0 := cd.list.GetItemText(0)
	// The stored label carries the tview-escaped "[blocked]" marker
	// ([blocked[] in this fork's Escape scheme) so the markup engine renders
	// the marker literally.
	if want := "× [blocked[] Eve  <" + strings.Repeat("e", 32) + ">"; main0 != want {
		t.Errorf("blocked row main = %q, want %q", main0, want)
	}
	if secondary0 != "" {
		t.Errorf("blocked row secondary = %q, want empty", secondary0)
	}
	main1, secondary1 := cd.list.GetItemText(1)
	if strings.Contains(main1, "[blocked]") {
		t.Errorf("normal row rendered as blocked: %q", main1)
	}
	if secondary1 == "" {
		t.Error("normal untrusted row lost its secondary (relative-time) text")
	}
}

// TestConversationsDisplayBlockedRowActivatesUnblock pins the row-activation
// dispatch (Python blocked-row click → _unblock_dialog, Conversations.py:332-
// 347): activating a blocked row fires OnUnblockPeer instead of opening the
// conversation; activating any other row still opens it.
func TestConversationsDisplayBlockedRowActivatesUnblock(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	blockedHash := strings.Repeat("e", 32)
	blocked := ConversationInfo{SourceHash: blockedHash, DisplayName: "Eve", TrustLevel: "blocked"}
	normalHash := strings.Repeat("a", 32)
	normal := ConversationInfo{SourceHash: normalHash, DisplayName: "Friend", TrustLevel: "untrusted"}
	cd := NewConversationsDisplay(app, []ConversationInfo{blocked, normal})
	cd.showTrusted = false
	cd.showBlocked = true
	cd.populateList()

	var unblocked []string
	cd.OnUnblockPeer = func(sourceHash string) { unblocked = append(unblocked, sourceHash) }
	var opened []string
	open := func(sourceHash string) { opened = append(opened, sourceHash) }

	// Blocked row → unblock hook, no conversation open.
	cd.activateListRow(0, open)
	if len(opened) != 0 {
		t.Fatalf("blocked row opened conversations %v; want the unblock flow", opened)
	}
	if len(unblocked) != 1 || unblocked[0] != blockedHash {
		t.Fatalf("unblock hook fired with %v, want [%v]", unblocked, blockedHash)
	}

	// Normal row → conversation opens through the same dispatch.
	cd.activateListRow(1, open)
	if len(opened) != 1 || opened[0] != normalHash {
		t.Fatalf("opened = %v, want [%v]", opened, normalHash)
	}

	// No hook wired → the blocked row stays inert (no conversation opens).
	cd.OnUnblockPeer = nil
	opened = nil
	cd.activateListRow(0, open)
	if len(opened) != 0 {
		t.Fatalf("blocked row opened a conversation without an unblock hook: %v", opened)
	}
}

// TestConversationsDisplayBlockedRowRequiresShowBlocked pins the filter
// parity (Python update_listbox, Conversations.py:483-488): blocked rows are
// appended to the untrusted list ONLY when "Show blocked" is checked — the
// filter predicate itself (Conversations.py:444-452) never accepts them.
func TestConversationsDisplayBlockedRowRequiresShowBlocked(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	blockedHash := strings.Repeat("e", 32)
	blocked := ConversationInfo{SourceHash: blockedHash, DisplayName: "Eve", TrustLevel: "blocked"}
	normalHash := strings.Repeat("a", 32)
	normal := ConversationInfo{SourceHash: normalHash, DisplayName: "Friend", TrustLevel: "untrusted"}
	cd := NewConversationsDisplay(app, []ConversationInfo{blocked, normal})
	cd.SetShowTrusted(false)

	// Checkbox unchecked (the default): the blocked row is hidden.
	if got := cd.list.GetItemCount(); got != 1 {
		t.Fatalf("untrusted tab shows %v rows with Show blocked unchecked; want 1 (only the untrusted row)", got)
	}
	main0, _ := cd.list.GetItemText(0)
	if strings.Contains(main0, "blocked") {
		t.Errorf("hidden blocked row leaked into the list: %q", main0)
	}

	// Checking "Show blocked" reveals it.
	cd.SetShowBlocked(true)
	if got := cd.list.GetItemCount(); got != 2 {
		t.Fatalf("untrusted tab shows %v rows with Show blocked checked; want 2 (untrusted + blocked)", got)
	}

	// The trusted tab never shows blocked rows, checked or not.
	cd.SetShowBlocked(false)
	cd.SetShowTrusted(true)
	cd.SetShowBlocked(true)
	if got := cd.list.GetItemCount(); got != 0 {
		t.Fatalf("trusted tab shows %v rows with Show blocked checked; want 0", got)
	}
}

// TestConversationsDisplayShowBlockedCountLabel pins the "Show blocked (N)"
// count (Python update_listbox, Conversations.py:472-477): N is the number of
// ignored-list entries, refreshed on every list rebuild. It previously stayed
// at the constructor's hardcoded "(0)" even with ignored entries present.
func TestConversationsDisplayShowBlockedCountLabel(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	model := []ConversationInfo{
		{SourceHash: strings.Repeat("e", 32), DisplayName: "Eve", TrustLevel: "blocked"},
		{SourceHash: strings.Repeat("a", 32), DisplayName: "Friend", TrustLevel: "untrusted"},
		{SourceHash: strings.Repeat("f", 32), DisplayName: "Malone", TrustLevel: "blocked"},
		{SourceHash: strings.Repeat("b", 32), DisplayName: "Other", TrustLevel: "untrusted"},
	}
	cd := NewConversationsDisplay(app, model)
	// The live path rebuilds the list through SetConversations (refreshConvs).
	cd.SetConversations(model)

	if got, want := cd.showBlockedCheckbox.GetLabel(), "Show blocked (2)"; got != want {
		t.Errorf("checkbox label = %q, want %q", got, want)
	}

	// An ignored-free model resets the count to zero.
	cd.SetConversations(model[1:2])
	if got, want := cd.showBlockedCheckbox.GetLabel(), "Show blocked (0)"; got != want {
		t.Errorf("checkbox label after clearing ignores = %q, want %q", got, want)
	}
}

// TestTabButtonLabelsExcludeBlocked pins the tab-count parity edge Python's
// harness cannot represent: Python's app.conversations() enumerates on-disk
// conversations only, so its Untrusted count (UNTRUSTED/WARNING/UNKNOWN,
// Conversations.py:452-455) never sees an ignored-list row. Go's model does
// carry "blocked" rows (refreshConvs appends them), and counting them made
// the Untrusted tab advertise an entry its filtered list never shows.
func TestTabButtonLabelsExcludeBlocked(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: strings.Repeat("a", 32), TrustLevel: "trusted", Unread: true},
		{SourceHash: strings.Repeat("b", 32), TrustLevel: "untrusted"},
		{SourceHash: strings.Repeat("e", 32), TrustLevel: "blocked", Unread: true},
	}
	trusted, untrusted := tabButtonLabels(convs, "✉")
	if want := "Trusted (1) ✉ 1"; trusted != want {
		t.Errorf("trusted label = %q, want %q", trusted, want)
	}
	if want := "Untrusted (1)"; untrusted != want {
		t.Errorf("untrusted label = %q, want %q (blocked rows count toward neither tab)", untrusted, want)
	}
}
