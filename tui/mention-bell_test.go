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
	"bytes"
	"testing"
)

// TestMentionBellPythonParity is a LIVE cross-implementation check: it execs
// Python's real ChannelsDisplay._ring_mention_bell (nomadnet.ui.textui.
// Channels) with a mock time.monotonic (so the Go-injected `now` is used
// instead of real wall time) and a mock hub, and derives the expected
// ring/no-ring decision freshly on every run by observing whether Python wrote
// the 0x07 BEL byte. Go owns the call sequences (debounce on one key,
// independent keys per room, empty room); Python owns the reference behavior.
// The test SKIPs, not fails, when the Python reference is not importable.
//
// The bell rings only when at least 5.0 seconds have passed since the last
// ring for the (hub_hash, room) key; otherwise it is a no-op. Because the
// per-key last time defaults to 0.0, the very first ring at now=0 does NOT
// fire (0 - 0 < 5.0); the first fire requires now >= 5.0. Each Go sequence
// runs against a fresh MentionBell and the byte stream is checked to contain
// exactly one 0x07 BEL per ring (Go-internal output behavior).
func TestMentionBellPythonParity(t *testing.T) {
	t.Parallel()

	type bellCall struct {
		Hub  string  `json:"hub"`
		Room string  `json:"room"`
		Now  float64 `json:"now"`
	}
	// Each sequence runs against a fresh bell (fresh _mention_bell_last).
	sequences := [][]bellCall{
		{
			{"h1", "room1", 0.0},
			{"h1", "room1", 4.0},
			{"h1", "room1", 5.0}, // 5.0 - 0.0 == 5.0, not < 5.0 -> rings
			{"h1", "room1", 9.9},
			{"h1", "room1", 10.0},
			{"h1", "room1", 14.999},
			{"h1", "room1", 15.0},
		},
		{
			{"h1", "room1", 5.0}, // first fire
			{"h1", "room2", 3.0}, // room2 independent, still at 0.0 baseline
		},
		{
			{"h2", "", 0.0},
			{"h2", "", 3.0},
			{"h2", "", 5.0},
		},
	}

	const script = `
import sys, json, contextlib, io, types
import nomadnet.ui.textui.Channels as C
class Self: pass
class Hub:
    def __init__(self, h): self.hub_hash = h
seqs = json.load(sys.stdin)
results = []
for seq in seqs:
    s = Self(); s._mention_bell_last = {}
    rang = []
    for c in seq:
        now = c["now"]
        C.time = types.SimpleNamespace(monotonic=lambda now=now: now)
        hub = Hub(c["hub"])
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            C.ChannelsDisplay._ring_mention_bell(s, hub, c["room"])
        rang.append("\x07" in buf.getvalue())
    results.append(rang)
json.dump(results, sys.stdout)
`

	var want [][]bool
	runPythonNomadnet(t, sequences, script, &want)

	names := []string{"debounce sequence on one key", "independent keys per room", "empty room key"}
	for si, seq := range sequences {
		t.Run(names[si], func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			b := NewMentionBell()
			b.writer = &buf
			rangCount := 0
			for ci, c := range seq {
				got := b.Ring(c.Hub, c.Room, c.Now)
				if got != want[si][ci] {
					t.Errorf("Ring(hub=%q, room=%q, now=%v) = %v, want %v (Python)",
						c.Hub, c.Room, c.Now, got, want[si][ci])
				}
				if got {
					rangCount++
				}
			}
			if got := buf.Len(); got != rangCount {
				t.Errorf("bell wrote %v bytes, want %v (one BEL per ring)", got, rangCount)
			}
			for _, by := range buf.Bytes() {
				if by != 0x07 {
					t.Errorf("bell wrote byte %v, want 0x07", by)
				}
			}
		})
	}
}
