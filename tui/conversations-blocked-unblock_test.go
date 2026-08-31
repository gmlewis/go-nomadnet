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
	if want := "× [blocked] Eve  <" + strings.Repeat("e", 32) + ">"; main0 != want {
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
