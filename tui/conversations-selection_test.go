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
)

// The conversation list must select by CONVERSATION, not by raw row index:
// every background refresh (SetConversations under an announce firehose)
// repopulates the tview.List, and the previous Clear()+re-Add reset the
// cursor to row 0 while the user was navigating — the highlight drifted to
// the top with no input, and Enter/click resolving a model row that differed
// from the highlighted row opened the WRONG conversation under the firehose.

func selectionConvs() []ConversationInfo {
	return []ConversationInfo{
		{SourceHash: "<a>", TrustLevel: "trusted", DisplayName: "Alpha"},
		{SourceHash: "<b>", TrustLevel: "trusted", DisplayName: "Beta"},
		{SourceHash: "<c>", TrustLevel: "trusted", DisplayName: "Gamma"},
	}
}

func TestConversationsSelectionPreservedAcrossRefresh(t *testing.T) {
	t.Parallel()
	cd := NewConversationsDisplay(newTestApp(), selectionConvs())

	// Navigate to the last visible row.
	cd.list.SetCurrentItem(2)
	if got := cd.visibleList()[2].SourceHash; got != "<c>" {
		t.Fatalf("precondition: row 2 shows %v, want <c>", got)
	}

	// A background refresh replaces the conversation infos (times change).
	refreshed := selectionConvs()
	refreshed[2].UnreadCount = 3
	cd.SetConversations(refreshed)

	if got := cd.list.GetCurrentItem(); got != 2 {
		t.Errorf("highlight after refresh = row %v, want 2 (drifted to top)", got)
	}
	conv, ok := cd.GetSelectedConversation()
	if !ok || conv.SourceHash != "<c>" {
		t.Errorf("selected conversation = %+v (ok=%v), want <c>", conv, ok)
	}
}

func TestConversationsSelectionFollowsVisibleRows(t *testing.T) {
	t.Parallel()
	// Mixed trust: the trusted tab renders only Alpha and Gamma.
	cd := NewConversationsDisplay(newTestApp(), []ConversationInfo{
		{SourceHash: "<a>", TrustLevel: "trusted", DisplayName: "Alpha"},
		{SourceHash: "<b>", TrustLevel: "untrusted", DisplayName: "Beta"},
		{SourceHash: "<c>", TrustLevel: "trusted", DisplayName: "Gamma"},
	})

	// The default tab is Trusted: only Alpha and Gamma are rendered, so the
	// model row 1 IS the rendered row 1 — but cd.conversations[1] is the
	// untrusted Beta. Selection must resolve against the RENDERED rows, or
	// Enter/Welcome-the-click opens the "Undefined" conversation (the model
	// row differing from the highlighted row).
	cd.list.SetCurrentItem(1)
	conv, ok := cd.GetSelectedConversation()
	if !ok {
		t.Fatalf("no selected conversation")
	}
	if conv.SourceHash != "<c>" {
		t.Errorf("row 1 resolves to %v, want <c> (the rendered Gamma row)", conv.SourceHash)
	}

	// Same resolution for the Enter-open path.
	hash := ""
	cd.selectVisibleConversation(1, func(sourceHash string) { hash = sourceHash })
	if hash != "<c>" {
		t.Errorf("opening rendered row 1 opened %v, want <c>", hash)
	}
}

func TestConversationsSelectionDroppedWhenRowDisappears(t *testing.T) {
	t.Parallel()
	cd := NewConversationsDisplay(newTestApp(), selectionConvs())

	// Select Gamma (row 2 on the trusted tab), then a refresh where Gamma
	// leaves the visible set (its trust changed to unknown): the selection
	// degrades gracefully to the remaining rows instead of pointing at a
	// stale index.
	cd.list.SetCurrentItem(2)
	refreshed := selectionConvs()
	refreshed[2].TrustLevel = "unknown"
	cd.SetConversations(refreshed)

	if got := cd.list.GetCurrentItem(); got < 0 || got >= cd.list.GetItemCount() {
		t.Errorf("current item %v out of range after refresh (%v rows)", got, cd.list.GetItemCount())
	}
	if conv, ok := cd.GetSelectedConversation(); !ok || conv.SourceHash == "<c>" {
		t.Errorf("stale selection kept after its row vanished: %+v ok=%v", conv, ok)
	}
}

func TestConversationsUntrustedTabSelection(t *testing.T) {
	t.Parallel()
	cd := NewConversationsDisplay(newTestApp(), []ConversationInfo{
		{SourceHash: "<a>", TrustLevel: "trusted", DisplayName: "Alpha"},
		{SourceHash: "<b>", TrustLevel: "untrusted", DisplayName: "Beta"},
		{SourceHash: "<c>", TrustLevel: "trusted", DisplayName: "Gamma"},
	})

	cd.SetShowTrusted(false)
	// Only Beta (untrusted) is visible now.
	cd.list.SetCurrentItem(0)
	conv, ok := cd.GetSelectedConversation()
	if !ok || conv.SourceHash != "<b>" {
		t.Errorf("untrusted tab row 0 = %+v ok=%v, want <b>", conv, ok)
	}
}
