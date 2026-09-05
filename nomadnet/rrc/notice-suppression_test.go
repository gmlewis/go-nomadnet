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

import (
	"testing"
)

// rrcd's server-command acknowledgements ("room <name>: registered;
// mode=+nrt; topic=(none)") are protocol traffic, not conversation: they must
// update no room buffer. The hub's roomless greeting MOTD still renders in
// the open room (Python parity), and a user-typed /list reply renders too —
// only the auto_list sweep's reply is silently consumed (see
// list-reply-parity_test.go).

// TestControlNoticesAreSuppressed pins the suppression list: the registration
// ack family and the /mode and /topic acks are consumed without rendering.
func TestControlNoticesAreSuppressed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		room string
	}{
		{"registration ack", "room general: registered; mode=+nrt; topic=(none)", "general"},
		{"unregistration ack", "room teskeslab: unregistered; mode=(none); topic=(none)", "teskeslab"},
		{"mode ack", "room general: mode set to +nrt", "general"},
		{"topic ack", "room general: topic set to hello", "general"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mgr, hub := fanoutFixture(t)
			mgr.SetActive(hub, "test")

			hub.HandleData(noticeEnvelope(t, tc.room, tc.text, "ctl"+tc.name))

			for _, room := range []string{"test", tc.room} {
				if msgs := hub.GetMessages(room); len(msgs) != 0 {
					t.Errorf("room %q buffer = %v entries, want 0 (control notices never render)", room, len(msgs))
				}
			}
			if hub.MOTD != "" {
				t.Errorf("MOTD = %q, want unset (a control notice is not the MOTD)", hub.MOTD)
			}
		})
	}
}

// TestConversationNoticesStillRender pins that the suppression list does not
// over-match: real hub notices still render.
func TestConversationNoticesStillRender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		room string
	}{
		{"motd", "Welcome to the RNS Community RRC hub!", ""},
		{"plain roomed notice", "Hub going down for maintenance", "general"},
		{"not an ack", "roomy conversations tonight", "general"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mgr, hub := fanoutFixture(t)
			mgr.SetActive(hub, "test")

			hub.HandleData(noticeEnvelope(t, tc.room, tc.text, "live1"))

			// A roomed notice lands in its own room's buffer; the roomless
			// MOTD is attributed to the active room ("test").
			want := tc.room
			if want == "" {
				want = "test"
			}
			if msgs := hub.GetMessages(want); len(msgs) != 1 {
				t.Fatalf("room %q buffer = %v entries, want the rendered notice", want, len(msgs))
			}
		})
	}
}

// TestRoomListNoticePopulatesAdvertisedRooms pins that the /list reply still
// populates the hub's advertised-room set. A USER-typed /list reply also
// renders (Python RRC.py:1092-1101 — only the auto_list sweep's reply is
// silently consumed; see list-reply-parity_test.go for the full contract).
func TestRoomListNoticePopulatesAdvertisedRooms(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	hub.HandleData(noticeEnvelope(t, "", "Registered public rooms\ncatfacts - Cats\nchat-hispano - Chat", "list1"))

	rooms := hub.GetAvailableRoomList()
	if len(rooms) != 2 || rooms[0] != "catfacts" || rooms[1] != "chat-hispano" {
		t.Errorf("advertised rooms = %v, want [catfacts chat-hispano]", rooms)
	}
	if got := len(hub.GetMessages("test")); got != 1 {
		t.Errorf("room %q buffer = %v entries, want the rendered user-initiated /list reply", "test", got)
	}
}

// TestInternalListCommandDoesNotRender pins that the auto_list sweep's /list
// (Python T_WELCOME tail, RRC.py:910-919) stays silent end to end: the armed
// request is sent, the reply is consumed without rendering, and the counter
// unwinds so a later user-typed /list reply renders.
func TestInternalListCommandDoesNotRender(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)
	hub.AddRoom("general")

	hub.requestRoomList()

	if len(*sent) != 1 || (*sent)[0] != "/list" {
		t.Fatalf("sent command bodies = %v, want [\"/list\"]", *sent)
	}

	hub.HandleData(noticeEnvelope(t, "", "Registered public rooms\ncatfacts - Cats", "list2"))

	if len(hub.GetAvailableRoomList()) != 1 {
		t.Errorf("advertised rooms = %v, want [catfacts]", hub.GetAvailableRoomList())
	}
	if got := len(hub.GetMessages("general")); got != 0 {
		t.Errorf("room buffer after auto /list reply = %v entries, want 0 (consumed silently)", got)
	}
	if got := hub.silentListPending; got != 0 {
		t.Errorf("silentListPending after reply = %v, want 0", got)
	}
}
