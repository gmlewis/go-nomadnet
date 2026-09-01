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

// Ctrl-X must act on the conversation the cursor is ON, resolved against the
// RENDERED rows of the active trust tab. The original Python resolves the
// selected row object itself (self.ilb.get_selected_item().source_hash,
// Conversations.py:561-566, 573-575); the Go port broke this by letting the
// wiring layer index the FULL unfiltered conversation list with the filtered
// row index — with the Untrusted tab active, Ctrl-X named (and after "Yes"
// deleted) a TRUSTED conversation instead of the highlighted untrusted one.

// deleteSelectedModel puts three trusted conversations before one untrusted,
// so the raw index of any untrusted-tab row resolves to a TRUSTED peer when
// read against the unfiltered model (the exact reported failure).
func deleteSelectedModel() []ConversationInfo {
	return []ConversationInfo{
		{SourceHash: "<t1>", TrustLevel: "trusted", DisplayName: "Trusted One"},
		{SourceHash: "<t2>", TrustLevel: "trusted", DisplayName: "Trusted Two"},
		{SourceHash: "<t3>", TrustLevel: "trusted", DisplayName: "Trusted Three"},
		{SourceHash: "<u1>", TrustLevel: "untrusted", DisplayName: "Untrusted Peer"},
	}
}

func pressDelete(cd *ConversationsDisplay) (*ConversationInfo, bool) {
	var fired *ConversationInfo
	cd.OnDeleteConv = func(conv ConversationInfo) {
		fired = &conv
	}
	if got := cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone)); got != nil {
		return nil, false
	}
	return fired, fired != nil
}

func TestConversationsDeleteSelectedResolvesUntrustedTab(t *testing.T) {
	t.Parallel()

	cd := NewConversationsDisplay(newTestApp(), deleteSelectedModel())
	cd.SetShowTrusted(false)
	// Only "Untrusted Peer" is visible; the cursor is on it (row 0).
	cd.list.SetCurrentItem(0)

	conv, fired := pressDelete(cd)
	if !fired {
		t.Fatal("Ctrl-X on the Untrusted tab did not fire the delete callback")
	}
	if conv.SourceHash != "<u1>" {
		t.Errorf("Ctrl-X targeted %v (%v); want the highlighted UNTRUSTED peer <u1> "+
			"(\"Untrusted Peer\"): targeting a trusted conversation is the "+
			"delete-wrong-conversation regression", conv.SourceHash, conv.DisplayName)
	}
}

func TestConversationsDeleteSelectedResolvesLaterUntrustedRow(t *testing.T) {
	t.Parallel()

	// Two untrusted peers behind two trusted ones: the untrusted tab renders
	// only the last two, and cursor row 1 must map to <u2>, not to the full
	// model's row 1 (a trusted peer).
	model := deleteSelectedModel()
	model = append(model,
		ConversationInfo{SourceHash: "<u2>", TrustLevel: "untrusted", DisplayName: "Second Untrusted"},
	)
	cd := NewConversationsDisplay(newTestApp(), model)
	cd.SetShowTrusted(false)
	cd.list.SetCurrentItem(1)

	conv, fired := pressDelete(cd)
	if !fired {
		t.Fatal("Ctrl-X on the Untrusted tab did not fire the delete callback")
	}
	if conv.SourceHash != "<u2>" {
		t.Errorf("untrusted-tab row 1 targeted %v (%q); want <u2> (\"Second Untrusted\")",
			conv.SourceHash, conv.DisplayName)
	}
}

func TestConversationsDeleteSelectedResolvesTrustedTab(t *testing.T) {
	t.Parallel()

	cd := NewConversationsDisplay(newTestApp(), deleteSelectedModel())
	// Trusted tab (default): cursor on the second row = Trusted Two.
	cd.list.SetCurrentItem(1)

	conv, fired := pressDelete(cd)
	if !fired {
		t.Fatal("Ctrl-X on the Trusted tab did not fire the delete callback")
	}
	if conv.SourceHash != "<t2>" {
		t.Errorf("trusted-tab row 1 targeted %v (%q); want <t2> (\"Trusted Two\")",
			conv.SourceHash, conv.DisplayName)
	}
}

func TestConversationsDeleteSelectedWithNoSelection(t *testing.T) {
	t.Parallel()

	// An empty list has no selected row: Python returns without opening the
	// dialog (Conversations.py:562-563) and the callback must not fire.
	cd := NewConversationsDisplay(newTestApp(), nil)

	if _, fired := pressDelete(cd); fired {
		t.Error("Ctrl-X with an empty conversation list fired the delete callback")
	}
}

func TestConversationsDeleteSelectedFollowsRefresh(t *testing.T) {
	t.Parallel()

	// The selected CONVERSATION survives a background refresh (SetConversations
	// re-population), and Ctrl-X must keep targeting it — not the row that now
	// occupies the same index after the refresh reordered the model.
	cd := NewConversationsDisplay(newTestApp(), deleteSelectedModel())
	cd.SetShowTrusted(false)
	cd.list.SetCurrentItem(0)

	refreshed := deleteSelectedModel()
	refreshed[3].UnreadCount = 2
	cd.SetConversations(refreshed)

	if got := cd.list.GetCurrentItem(); got != 0 {
		t.Fatalf("highlight drifted to row %v after refresh; want 0", got)
	}
	conv, fired := pressDelete(cd)
	if !fired {
		t.Fatal("Ctrl-X after refresh did not fire the delete callback")
	}
	if conv.SourceHash != "<u1>" {
		t.Errorf("Ctrl-X after refresh targeted %v; want <u1>", conv.SourceHash)
	}
}
