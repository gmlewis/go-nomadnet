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

// Type 40 (ERROR) was previously dropped entirely. Per the Python T_ERROR
// branch (RRC.py:1145-1170) the text is recorded as an error notice in the
// affected room (or the active room when roomless), a pending join for that
// room rolls back, and per doc 4-RRC a refusal error ends the session.

// errorEnvelope builds one inbound T_ERROR envelope.
func errorEnvelope(t *testing.T, room, text, mid string) []byte {
	t.Helper()
	var roomAny []byte
	if room != "" {
		roomAny = []byte(room)
	}
	env := MakeEnvelope(TypeError, []byte("hubsrc"), roomAny, nil, text, []byte(mid), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// TestErrorRecordedAsErrorNotice pins the recording: the ERROR text lands in
// the affected room's buffer as Kind "error", never as chat.
func TestErrorRecordedAsErrorNotice(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	hub.HandleData(errorEnvelope(t, "test", "You are not the operator of room test", "err1"))

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("room test buffer = %v entries, want the error notice", len(msgs))
	}
	if msgs[0].Kind != "error" {
		t.Errorf("kind = %q, want error", msgs[0].Kind)
	}
	if msgs[0].Text != "You are not the operator of room test" {
		t.Errorf("text = %q, want the error text", msgs[0].Text)
	}
}

// TestErrorRoomlessGoesToActiveRoom pins Python's roomless-error attribution:
// the notice joins the manager's active room.
func TestErrorRoomlessGoesToActiveRoom(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	hub.HandleData(errorEnvelope(t, "", "Rate limit exceeded. Try again later.", "err2"))

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 || msgs[0].Kind != "error" {
		t.Fatalf("active room buffer = %v, want the roomless error notice", msgs)
	}
	if msgs[0].Room != "test" {
		t.Errorf("room = %q, want test (the roomless error is attributed to the active room)", msgs[0].Room)
	}
}

// TestErrorRollsBackPendingJoin pins Python's join rollback (RRC.py:1150-1162):
// an ERROR for a room with a pending join drops the pending marker and the
// room itself.
func TestErrorRollsBackPendingJoin(t *testing.T) {
	t.Parallel()

	_, hub := fanoutFixture(t)
	hub.AddRoom("general")

	hub.onSend = func(map[any]any) {}
	hub.JoinRoom("general", false)

	hub.HandleData(errorEnvelope(t, "general", "Room is full", "err3"))

	if hub.HasRoom("general") {
		t.Error("rolled-back join left the room joined")
	}
	hub.lock.Lock()
	_, pending := hub.pendingJoins["general"]
	hub.lock.Unlock()
	if pending {
		t.Error("pending join marker survived the ERROR rollback")
	}
}

// TestRefusalErrorFailsHub pins doc 4-RRC: an ERROR that clearly indicates
// refusal fails the hub (the client session is over); an ordinary error does
// not touch the status.
func TestRefusalErrorFailsHub(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		text    string
		refusal bool
	}{
		{"rate limit", "Rate limit exceeded. Try again later.", true},
		{"kicked", "You were kicked from room general", true},
		{"not operator", "You are not the operator of room test", false},
		{"plain text", "Something went wrong", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, hub := fanoutFixture(t)

			hub.HandleData(errorEnvelope(t, "test", tc.text, "err4"))

			got := hub.GetHubStatus()
			if tc.refusal {
				if got != StatusFailed {
					t.Errorf("status after refusal error = %v, want StatusFailed", got)
				}
				if statusText := hub.GetStatusText(); statusText != "Refused: "+tc.text {
					t.Errorf("status text = %q, want the refusal", statusText)
				}
			} else if got != StatusDisconnected {
				t.Errorf("status after non-refusal error = %v, want unchanged StatusDisconnected", got)
			}
		})
	}
}

// TestNonStringErrorBodyUsesPlaceholder pins Python's "(error)" placeholder
// (RRC.py:1148): a non-text ERROR body still records an error notice.
func TestNonStringErrorBodyRecordsPlaceholder(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	env := MakeEnvelope(TypeError, []byte("hubsrc"), []byte("test"), nil, map[any]any{0: "structured"}, []byte("err5"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	hub.HandleData(data)

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 || msgs[0].Kind != "error" || msgs[0].Text != "(error)" {
		t.Fatalf("recorded error = %v, want one \"(error)\" notice", msgs)
	}
}
