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
)

// TestSetConversationsRefreshesOpenConversationPeerInfo pins the header's
// agreement with the list row it sits beside.
//
// A peer's display name arrives only with its announce, and a conversation (or a
// whole pairing) is routinely opened before that announce is heard — the New
// Conversation dialog knows nothing but the address it was given. The list row
// resolves the name as soon as the announce lands, because the row is rebuilt
// from App.ConversationList, whose app-data fallback reads the peer's announce
// (nomadnet/app/app.go). The peer-info HEADER is resolved only when the
// conversation is displayed, so without a re-resolve the row and the header
// disagree, and the header keeps its "<hex>…" fallback.
//
// Python has no such split: _update_peer_info runs inside update_widget_list, so
// every announce-driven list refresh re-resolves the open conversation's header
// too (Conversations.py:622, 1909). SetConversations is the Go port's equivalent
// entry point, so the refresh belongs there rather than in each caller.
func TestSetConversationsRefreshesOpenConversationPeerInfo(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }

	const peer = "aabbccddeeff00112233445566778899"

	cd.SetConversations([]ConversationInfo{{SourceHash: peer}})
	cd.DisplayConversation(peer)
	if cd.currentWidget == nil {
		t.Fatal("no conversation widget after DisplayConversation")
	}
	if got := cd.currentWidget.peerInfoBar.GetText(false); !strings.Contains(got, "<"+peer+">") {
		t.Fatalf("precondition: header = %q, want the <hex> fallback while the name is unknown", got)
	}

	// The announce lands, and the next list refresh carries the resolved name.
	// The open conversation's header must follow its row, without the
	// conversation being closed and reopened.
	cd.SetConversations([]ConversationInfo{{SourceHash: peer, DisplayName: "Anonymous Peer"}})
	got := cd.currentWidget.peerInfoBar.GetText(false)
	if !strings.Contains(got, "Anonymous Peer") {
		t.Errorf("header = %q after the list refresh, want it to carry the resolved name; the row and the header must not disagree", got)
	}
}

// TestSetConversationsRefreshesOpenConversationHops covers the other two
// RNS-derived fields in the same bar. They are resolved through the display's
// OnHops/OnStampCost hooks, which are live lookups, so a refresh must re-run
// them: a peed peer's hop count is exactly what changes when its announce and
// path finally arrive.
func TestSetConversationsRefreshesOpenConversationHops(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }

	const peer = "aabbccddeeff00112233445566778899"
	hops := -1
	cd.OnHops = func(string) *int {
		if hops < 0 {
			return nil
		}
		return &hops
	}

	cd.SetConversations([]ConversationInfo{{SourceHash: peer}})
	cd.DisplayConversation(peer)
	if got := cd.currentWidget.peerInfoBar.GetText(false); !strings.Contains(got, "unknown") {
		t.Fatalf("precondition: header = %q, want \"unknown\" hops before a path is known", got)
	}

	hops = 2
	cd.SetConversations([]ConversationInfo{{SourceHash: peer}})
	if got := cd.currentWidget.peerInfoBar.GetText(false); !strings.Contains(got, "2 hops") {
		t.Errorf("header = %q after the list refresh, want \"2 hops\" once the path is known", got)
	}
}
