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
	"encoding/hex"
	"testing"
)

// fanoutBurstFixture replays the REAL rrcd 0.3.2 fanout burst of the
// 2026-09-03 12:26:55 A1 forward (~/rrcd-run.log:5039-5041: "Forwarded
// t=20 peer=0a8b370a62de nick=None room=test recipients=6" — six per-member
// copies with per-copy rewritten source hashes and registry nicks; the
// registry nicks come from the log's JOIN lines). Copies with an empty nick
// are the rrcd registry-miss case the A2 capture symptom rode on.
// zeroPad extends an rrcd 12-hex registry peer id to a full 32-byte
// destination hash (the wire form the JOINED fanout body carries).
const zeroPad = "0000000000000000000000000000000000000000000000000000"

func fanoutBurst(t *testing.T, h *RRCHub) {
	t.Helper()

	envelopeTs := NowMs() - 5_000
	copies := []struct{ src12, nick string }{
		{"464360ee59ed", ""}, // the first-arrived copy (the A2 symptom: no nick)
		{"c59ad005b13f", "Go port of NomadNet on RaspPi"},
		{"1ab1ed2d6293", ""}, // rrcd's registry had nick=None
		{"d252c38c2e7b", "Go port of NomadNet on PixelBook"},
		{"0a8b370a62de", "Go port of NomadNet on Mac M2 Max"},
		{"9554d5d4cb3b", "Go port of NomadNet on Mac Mini M2"},
	}
	for i, c := range copies {
		src, err := hex.DecodeString(c.src12)
		if err != nil {
			t.Fatalf("decode src: %v", err)
		}
		h.HandleData(fanoutCopy(string(src), c.nick, "Message A2 body",
			"burst-mid-"+c.src12, envelopeTs+int64(i)*10))
	}
}

// TestFanoutBurstLearning pins Python RRCHub._record_message's bookkeeping
// (RRC.py:1031-1035) replayed through the burst: every copy with a non-empty
// nick learns nicks[src] = nick AND adds src to the room's member set (an
// empty-nick copy learns NOTHING — the learning block is gated on the nick),
// and the collapse still renders the burst ONCE (the user-ordered deviation)
// with the backfilled nick.
func TestFanoutBurstLearningReplay(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")
	fanoutBurst(t, hub)

	nicked := map[string]string{
		"c59ad005b13f": "Go port of NomadNet on RaspPi",
		"d252c38c2e7b": "Go port of NomadNet on PixelBook",
		"0a8b370a62de": "Go port of NomadNet on Mac M2 Max",
		"9554d5d4cb3b": "Go port of NomadNet on Mac Mini M2",
	}

	// GetMessages takes h.lock itself — read the buffer BEFORE the manual
	// lock (a non-reentrant double Lock would self-deadlock).
	msgs := hub.GetMessages("test")

	hub.lock.Lock()
	defer hub.lock.Unlock()

	for srcHex, wantNick := range nicked {
		if got := hub.Nicks[srcHex]; got != wantNick {
			t.Errorf("nicks[%v] = %q, want %q (Python learns from every nicked copy, RRC.py:1031-1035)", srcHex, got, wantNick)
		}
		if !hub.Members["test"][srcHex] {
			t.Errorf("members[test] misses %v (Python adds every nicked copy's src)", srcHex)
		}
	}
	// The empty-nick copies learn nothing (the learning block's nick gate).
	for _, srcHex := range []string{"464360ee59ed", "1ab1ed2d6293"} {
		if got, ok := hub.Nicks[srcHex]; ok {
			t.Errorf("nicks[%v] = %q, want absent (an empty-nick copy carries no nick to learn)", srcHex, got)
		}
	}

	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (the burst collapses to one render)", len(msgs))
	}
	if msgs[0].Nick != "Go port of NomadNet on RaspPi" {
		t.Errorf("kept copy nick = %q, want the backfilled %q", msgs[0].Nick, "Go port of NomadNet on RaspPi")
	}
}

// TestJoinedHealsMemberSet pins Python's JOINED bookkeeping (RRC.py:944-948,
// reached via the fanout JOINED body): a JOINED whose body carries the FULL
// member list adds EVERY body hash to the room's member set, healing the
// whole set client-side.
func TestJoinedHealsMemberSet(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	// A JOINED fanout body: the full member list as 32-byte hash entries
	// (the body-hash list the JOINED fanout carries).
	fullHashes := []string{
		"464360ee59ed" + zeroPad,
		"c59ad005b13f" + zeroPad,
		"1ab1ed2d6293" + zeroPad,
		"d252c38c2e7b" + zeroPad,
		"0a8b370a62de" + zeroPad,
		"9554d5d4cb3b" + zeroPad,
	}
	full := make([][]byte, 0, len(fullHashes))
	for _, h := range fullHashes {
		full = append(full, mustHex(t, h))
	}
	env := MakeEnvelope(TypeJoined, []byte("hubsrc"), []byte("test"), nil, full, []byte("joined-mid-1"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	hub.HandleData(data)

	hub.lock.Lock()
	defer hub.lock.Unlock()
	for k := range hub.Members["test"] {
		t.Logf("member: %v", k)
	}
	for _, fullHex := range fullHashes {
		if !hub.Members["test"][fullHex] {
			t.Errorf("members[test] misses %v after the full-list JOINED", fullHex)
		}
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex %q: %v", s, err)
	}
	return b
}
