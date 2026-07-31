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

// TestMentionBellPythonParity verifies MentionBell.Ring against Python's
// _ring_mention_bell (Channels.py:2273). The bell rings (writes the 0x07 BEL
// character and records the time) only when at least mentionBellCooldown
// seconds of monotonic time have passed since the last ring for the
// (hub_hash, room) key; otherwise it is a no-op. Because the per-key last
// time defaults to 0.0, the very first ring at now=0 does NOT fire
// (0 - 0 < 5.0); the first fire requires now >= 5.0. Expected ring/no-ring
// decisions were captured from /tmp/bell_ref.py, which replays the Python
// debounce on a single key, across rooms, and with empty rooms.
func TestMentionBellPythonParity(t *testing.T) {
	t.Parallel()

	t.Run("debounce sequence on one key", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		b := NewMentionBell()
		b.writer = &buf
		seq := []struct {
			now  float64
			ring bool
		}{
			{0.0, false},
			{4.0, false},
			{5.0, true}, // 5.0 - 0.0 == 5.0, not < 5.0 -> rings
			{9.9, false},
			{10.0, true},
			{14.999, false},
			{15.0, true},
		}
		rangCount := 0
		for _, s := range seq {
			got := b.Ring("h1", "room1", s.now)
			if got != s.ring {
				t.Errorf("Ring(now=%v) = %v, want %v", s.now, got, s.ring)
			}
			if got {
				rangCount++
			}
		}
		if got := buf.Len(); got != rangCount {
			t.Errorf("bell wrote %d bytes, want %d (one BEL per ring)", got, rangCount)
		}
		for _, by := range buf.Bytes() {
			if by != 0x07 {
				t.Errorf("bell wrote byte %v, want 0x07", by)
			}
		}
	})

	t.Run("independent keys per room", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		b := NewMentionBell()
		b.writer = &buf
		if got := b.Ring("h1", "room1", 5.0); !got {
			t.Errorf("Ring(room1, now=5.0) = false, want true (first fire)")
		}
		// room2 on same hub is independent and still at its 0.0 baseline.
		if got := b.Ring("h1", "room2", 3.0); got {
			t.Errorf("Ring(room2, now=3.0) = true, want false (3.0 - 0.0 < 5.0)")
		}
	})

	t.Run("empty room key", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		b := NewMentionBell()
		b.writer = &buf
		if got := b.Ring("h2", "", 0.0); got {
			t.Errorf("Ring(empty, now=0.0) = true, want false (0.0 - 0.0 < 5.0)")
		}
		if got := b.Ring("h2", "", 3.0); got {
			t.Errorf("Ring(empty, now=3.0) = true, want false")
		}
		if got := b.Ring("h2", "", 5.0); !got {
			t.Errorf("Ring(empty, now=5.0) = false, want true")
		}
	})
}
