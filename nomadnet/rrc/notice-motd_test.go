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

// TestRoomlessNoticeJoinsActiveRoom pins Python RRCHub._record_notice
// (RRC.py:817-839) via the T_NOTICE branch (RRC.py:1128-1138): a ROOMLESS
// notice sets the hub's MOTD (RRC.py:1128-1131) and is then attributed to
// the manager's ACTIVE room, appended to that room's buffer with
// Kind="notice" — rrcd's global "󰙎 Welcome to the RaspPi Local Hub!" MOTD
// notice renders IN the #test room on the Python SOT (2026-09-03 12:32
// full-fleet capture, mac row 24) while the Go nodes only showed the
// unregistered notice. Roomed notices route through the same
// _record_notice call (RRC.py:1138) unchanged.
func TestRoomlessNoticeJoinsActiveRoom(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	const welcome = "Welcome to the RaspPi Local Hub!"
	hub.HandleData(noticeEnvelope(t, "", welcome, "motd1"))

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("active room buffer len = %v, want 1 (the roomless MOTD notice must join the active room)", len(msgs))
	}
	if msgs[0].Kind != "notice" {
		t.Errorf("kind = %q, want notice", msgs[0].Kind)
	}
	if msgs[0].Text != welcome {
		t.Errorf("text = %q, want %q", msgs[0].Text, welcome)
	}
	// _record_notice rewrites msg.room to the target room (RRC.py:822-824).
	if msgs[0].Room != "test" {
		t.Errorf("room = %q, want test (the roomless notice is attributed to the active room)", msgs[0].Room)
	}

	// The MOTD is still set (Python RRC.py:1128-1131: motd = body).
	if hub.MOTD != welcome {
		t.Errorf("MOTD = %q, want %q", hub.MOTD, welcome)
	}
}

// TestRoomedNoticeRoutesThroughRecordNotice pins that a ROOMED notice still
// lands in its room's buffer through the same _record_notice path
// (RRC.py:1138) — the collapse, MOTD and attribution behaviors stay.
func TestRoomedNoticeRoutesThroughRecordNotice(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	hub.HandleData(noticeEnvelope(t, "test", "room notice", "n1"))
	msgs := hub.GetMessages("test")
	if len(msgs) != 1 || msgs[0].Kind != "notice" || msgs[0].Text != "room notice" {
		t.Fatalf("roomed notice = %+v, want one notice \"room notice\"", msgs)
	}
	if hub.MOTD != "" {
		t.Errorf("MOTD = %q, want unset for a roomed notice", hub.MOTD)
	}
}

// noticeEnvelope builds one inbound T_NOTICE envelope like rrcd sends: room
// may be nil for the hub's global (MOTD) notices.
func noticeEnvelope(t *testing.T, room, text, mid string) []byte {
	t.Helper()
	var roomAny []byte
	if room != "" {
		roomAny = []byte(room)
	}
	env := MakeEnvelope(TypeNotice, []byte("hubsrc"), roomAny, nil, text, []byte(mid), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}
