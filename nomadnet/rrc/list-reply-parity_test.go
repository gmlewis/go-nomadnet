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

package rrc

import "testing"

// The /list reply NOTICE handling mirrors Python RRCHub (RRC.py:910-919 and
// RRC.py:1087-1139): ONLY the auto_list sweep's reply is silently consumed
// (via _silent_list_pending); a user-typed /list reply is recorded as a
// rendered notice in the room buffer. Verified live against the Python SOT:
// a user-typed /list reply lands in hub.messages[active_room]
// ("Registered public rooms\n...") while the auto-list reply does not.

const listReplyBody = "Registered public rooms\ncatfacts - Cats\nchat-hispano - Chat"

// TestUserListReplyRenders pins the Bug#1 parity contract: a USER-typed
// /list (no auto-list request pending) renders the hub's reply as a notice
// in the active room (Python RRC.py:1092-1101: silent is False, the notice
// falls through to _record_notice) while still populating the advertised
// room set.
func TestUserListReplyRenders(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	hub.HandleData(noticeEnvelope(t, "", listReplyBody, "list-user"))

	if rooms := hub.GetAvailableRoomList(); len(rooms) != 2 {
		t.Errorf("advertised rooms = %v, want [catfacts chat-hispano]", rooms)
	}
	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("active room buffer = %v entries, want the rendered /list reply (Python RRC.py:1138 _record_notice)", len(msgs))
	}
	if msgs[0].Kind != "notice" || msgs[0].Text != listReplyBody {
		t.Errorf("rendered notice = (%v, %q), want (notice, %q)", msgs[0].Kind, msgs[0].Text, listReplyBody)
	}
}

// TestAutoListReplySilentThenUserReplyRenders pins the counter semantics
// (Python _silent_list_pending, RRC.py:256,910-919,1092-1101): the auto_list
// sweep arms the counter BEFORE its send, its reply is consumed without
// rendering, the counter unwinds, and a LATER user-typed /list reply renders.
func TestAutoListReplySilentThenUserReplyRenders(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)

	hub.requestRoomList()

	if len(*sent) != 1 || (*sent)[0] != "/list" {
		t.Fatalf("auto-list sent = %v, want [\"/list\"]", *sent)
	}
	if got := hub.silentListPending; got != 1 {
		t.Fatalf("silentListPending after request = %v, want 1 (armed before the send)", got)
	}

	// The auto-requested reply updates the advertised set but never renders.
	hub.HandleData(noticeEnvelope(t, "", listReplyBody, "list-auto"))
	if rooms := hub.GetAvailableRoomList(); len(rooms) != 2 {
		t.Errorf("advertised rooms after auto reply = %v, want [catfacts chat-hispano]", rooms)
	}
	if got := len(hub.GetMessages("test")); got != 0 {
		t.Errorf("auto-list reply rendered %v entries, want 0 (consumed silently)", got)
	}
	if got := hub.silentListPending; got != 0 {
		t.Errorf("silentListPending after reply = %v, want 0 (consumed)", got)
	}

	// The next reply is user-initiated (no pending auto request) and renders.
	hub.HandleData(noticeEnvelope(t, "", listReplyBody, "list-user"))
	msgs := hub.GetMessages("test")
	if len(msgs) != 1 || msgs[0].Text != listReplyBody {
		t.Fatalf("user /list reply after the auto sweep = %+v, want the rendered notice", msgs)
	}
}

// TestRequestRoomListSendsRoomlessList pins the wire shape (Python
// RRC.py:914: send_command("/list", room=None) — the auto-list request
// carries NO room field).
func TestRequestRoomListSendsRoomlessList(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)

	hub.requestRoomList()

	if len(*sent) != 1 || (*sent)[0] != "/list" {
		t.Fatalf("auto-list sent = %v, want [\"/list\"]", *sent)
	}
}
