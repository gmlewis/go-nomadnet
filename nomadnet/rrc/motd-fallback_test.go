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
	"time"
)

// The 2026-09-03 fleet captures showed the hub's greeting MOTD rendered on
// some nodes and missing on others. Two client-side causes: the roomless
// MOTD notice is attributed to the manager's ACTIVE room only, which on a
// fresh boot is empty, so the greeting lands in NO room buffer (Python drops
// it the same way); and the 600 s ephemeral-notice purge removes it ten
// minutes later. The greeting is room-agnostic hub info that rrcd re-sends
// on every WELCOME, so the fix attributes it to every joined room when no
// room is active (always visible somewhere the user opens) and pins it so
// the ephemeral purge never erases it mid-session.

// motdEnvelope builds one inbound roomless T_NOTICE envelope with a chosen
// timestamp (the greeting rides as a roomless notice from rrcd).
func motdEnvelope(t *testing.T, text, mid string, ts int64) []byte {
	t.Helper()
	env := MakeEnvelope(TypeNotice, []byte("hubsrc"), nil, nil, text, []byte(mid), ts)
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// roomedNoticeEnvelope builds one room-scoped T_NOTICE with a chosen
// timestamp.
func roomedNoticeEnvelope(t *testing.T, room, text, mid string, ts int64) []byte {
	t.Helper()
	env := MakeEnvelope(TypeNotice, []byte("hubsrc"), []byte(room), nil, text, []byte(mid), ts)
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// TestRoomlessMOTDWithoutActiveRoomFallsBackToJoinedRooms pins the fresh-boot
// fix: with no active room the roomless MOTD notice is recorded into EVERY
// joined room's buffer (and marks none of them unread), so the greeting is
// visible in whichever room the user opens.
func TestRoomlessMOTDWithoutActiveRoomFallsBackToJoinedRooms(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)

	const greeting = "Welcome to the RaspPi Local Hub!"
	hub.HandleData(motdEnvelope(t, greeting, "m1", NowMs()))

	for _, room := range []string{"test", "test3", "test4"} {
		msgs := hub.GetMessages(room)
		if len(msgs) != 1 {
			t.Fatalf("room %q buffer = %v entries, want the greeting notice", room, len(msgs))
		}
		if msgs[0].Text != greeting || msgs[0].Kind != "notice" {
			t.Errorf("room %q notice = (%v, %q), want (notice, %q)", room, msgs[0].Kind, msgs[0].Text, greeting)
		}
		if msgs[0].Room != room {
			t.Errorf("room %q notice attributed to %q", room, msgs[0].Room)
		}
	}
	// The greeting is hub info, not chat traffic: it must not flag the rooms
	// unread (the olive-unread mask hid the joined-room highlight on the
	// fleet).
	for room := range hub.UnreadRooms {
		t.Errorf("fallback MOTD marked room %q unread", room)
	}
	if hub.MOTD != greeting {
		t.Errorf("MOTD = %q, want %q", hub.MOTD, greeting)
	}
}

// TestRoomlessMOTDWithActiveRoomStaysSingleRoom pins Python parity when a
// room IS active: the greeting is attributed to that one room only.
func TestRoomlessMOTDWithActiveRoomStaysSingleRoom(t *testing.T) {
	t.Parallel()

	mgr, hub, _ := reconcileFixture(t)
	mgr.SetActive(hub, "test3")

	hub.HandleData(motdEnvelope(t, "Welcome!", "m2", NowMs()))

	if got := len(hub.GetMessages("test3")); got != 1 {
		t.Fatalf("active room buffer = %v entries, want 1", got)
	}
	for _, room := range []string{"test", "test4"} {
		if got := hub.GetMessages(room); len(got) != 0 {
			t.Errorf("room %q buffer = %v entries, want 0 (the active room keeps the attribution)", room, len(got))
		}
	}
}

// TestMOTDNoticeSurvivesEphemeralCleanup pins the pinning: the hub greeting
// is exempt from the ephemeral-notice purge that removes roomed system
// notices after the configured timeout.
func TestMOTDNoticeSurvivesEphemeralCleanup(t *testing.T) {
	t.Parallel()

	mgr, hub, _ := reconcileFixture(t)
	mgr.SetHistoryConfig(0, false, 1) // 1-second ephemeral window

	// recordNotice directly with an aged arrival time: HandleData stamps
	// every inbound notice with its arrival time (Python RRC.py:1043), so the
	// age the cleanup sees is the record time. The greeting is recorded
	// roomless with no active room → the all-rooms fallback, pinned; a roomed
	// system notice is ephemeral and must be purged.
	old := NowMs() - int64(5*time.Second/time.Millisecond)
	hub.recordNotice(&RRCMessage{Kind: "notice", Text: "Welcome to the RaspPi Local Hub!", Ts: old})
	hub.recordNotice(&RRCMessage{Kind: "notice", Room: "test", Text: "room test: unregistered; mode=(none); topic=(none)", Ts: old})

	// Age the cleaner past its interval so the pass actually runs.
	hub.lastHistoryClean.Store(time.Now().Unix() - CleanHistoryInterval - 1)
	hub.cleanHistory()

	if got := hub.GetMessages("test"); len(got) != 1 || got[0].Text != "Welcome to the RaspPi Local Hub!" {
		t.Fatalf("test buffer after cleanup = %v, want the pinned greeting only (the ephemeral roomed notice purged)", got)
	}
	if got := len(hub.GetMessages("test4")); got != 1 {
		t.Fatalf("test4 buffer after cleanup = %v entries, want the pinned greeting", got)
	}
}

// TestMOTDNoticeIsPinnedOnRecord pins the flag: the recorded greeting
// message carries Pinned so cleanHistory's filter can exempt it.
func TestMOTDNoticeIsPinnedOnRecord(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.HandleData(motdEnvelope(t, "Welcome!", "m4", NowMs()))

	msgs := hub.GetMessages("test3")
	if len(msgs) != 1 || !msgs[0].Pinned {
		t.Fatalf("greeting notice = %+v, want Pinned", msgs)
	}
}
