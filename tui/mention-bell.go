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
	"io"
	"os"
)

// MentionBell debounces the terminal mention bell per (hub, room), matching
// Python's _ring_mention_bell (Channels.py:2273). The bell rings only when at
// least mentionBellCooldown seconds of monotonic time have passed since the
// last ring for the (hubHash, room) key; otherwise it is a no-op. When it
// rings, it writes the 0x07 BEL character to writer (stdout by default).
type MentionBell struct {
	last   map[[2]string]float64
	writer io.Writer
}

// mentionBellCooldown is the minimum spacing between rings for a single
// (hub, room) key, in seconds. It mirrors Python's hardcoded 5.0 threshold.
const mentionBellCooldown = 5.0

// NewMentionBell creates a mention bell that writes BEL to stdout.
func NewMentionBell() *MentionBell {
	return &MentionBell{
		last:   make(map[[2]string]float64),
		writer: os.Stdout,
	}
}

// Ring rings the mention bell for (hubHash, room) if at least
// mentionBellCooldown seconds have passed since the last ring for that key.
// now is a monotonic timestamp in seconds (Python uses time.monotonic()). A
// nil/empty room collapses to "" like Python's `room or ""`. Returns whether
// the bell actually rang.
func (b *MentionBell) Ring(hubHash, room string, now float64) bool {
	key := [2]string{hubHash, room}
	last := b.last[key]
	if now-last < mentionBellCooldown {
		return false
	}
	b.last[key] = now
	if b.writer != nil {
		_, _ = b.writer.Write([]byte{0x07})
	}
	return true
}
