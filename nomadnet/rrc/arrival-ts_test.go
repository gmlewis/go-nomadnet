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

// TestInboundArrivalTimestamp pins Python's timestamp source (RRC.py:1043):
// EVERY inbound message (msg/action/notice) is stamped with the receiver's
// own _now_ms() ARRIVAL time — the sender's envelope ts is only kept as the
// fanout-dedupe window key (the copies share it). Measured live on the
// 2026-09-03 12:32 full-fleet capture: A1 shows [12:26:56] on the Python SOT
// while the Go nodes showed the sender's envelope [12:26:55] — a real,
// visible 1s skew (worse over relays).
func TestInboundArrivalTimestamp(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	envelopeTs := NowMs() - 60_000
	before := NowMs()
	hub.HandleData(fanoutCopy("peerA", "Nick A", "arrive msg", "mid-a1", envelopeTs))
	hub.HandleData(actionCopy("peerA", "Nick A", "/me arrives", "mid-a2", envelopeTs))
	hub.HandleData(noticeEnvelope(t, "test", "arrive notice", "mid-a3"))
	after := NowMs()

	for _, tt := range []struct {
		kind, text string
	}{
		{"msg", "arrive msg"},
		{"action", "/me arrives"},
		{"notice", "arrive notice"},
	} {
		msgs := hub.GetMessages("test")
		var got *RRCMessage
		for _, m := range msgs {
			if m.Text == tt.text {
				got = m
				break
			}
		}
		if got == nil {
			t.Fatalf("%v: message %q not recorded", tt.kind, tt.text)
		}
		if got.Kind != tt.kind {
			t.Errorf("%v: kind = %q, want %q", tt.kind, got.Kind, tt.kind)
		}
		if got.Ts < before || got.Ts > after {
			t.Errorf("%v: recorded Ts = %v, want the arrival time in [%v,%v] (the envelope's %v must not be used)", tt.kind, got.Ts, before, after, envelopeTs)
		}
	}
}

// actionCopy builds one inbound T_ACTION envelope like rrcd relays.
func actionCopy(src, nick, body, mid string, ts int64) []byte {
	env := MakeEnvelope(TypeAction, []byte(src), []byte("test"), []byte(nick), body, []byte(mid), ts)
	data, err := EncodeEnvelope(env)
	if err != nil {
		panic(err)
	}
	return data
}

// TestInboundArrivalTimestampKeepsCollapseWindow pins that the fanout-dedupe
// window still keys on the shared ENVELOPE ts (rrcd's per-member fanout
// copies share the sender's envelope timestamp, so the window stays glued to
// it even though the recorded message carries the arrival time).
func TestInboundArrivalTimestampKeepsCollapseWindow(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	envelopeTs := NowMs() - 60_000
	hub.HandleData(fanoutCopy("peerA", "", "burst body", "mid-b1", envelopeTs))
	hub.HandleData(fanoutCopy("srcB", "Nick N", "burst body", "mid-b2", envelopeTs))
	hub.HandleData(fanoutCopy("srcC", "", "burst body", "mid-b3", envelopeTs))

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (copies sharing the envelope ts still collapse)", len(msgs))
	}
	if msgs[0].Nick != "Nick N" {
		t.Errorf("kept copy nick = %q, want %q (the backfill still applies)", msgs[0].Nick, "Nick N")
	}
}
