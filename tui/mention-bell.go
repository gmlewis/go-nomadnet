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
	"sync"
	"time"
)

// MentionBell tracks terminal bell cooldown per room to prevent
// excessive bell rings when multiple mentions arrive in quick succession.
type MentionBell struct {
	mu       sync.Mutex
	cooldown time.Duration
	lastRing map[string]time.Time
	lastMsg  map[string]int
}

// NewMentionBell creates a bell with the given cooldown duration.
func NewMentionBell(cooldown time.Duration) *MentionBell {
	return &MentionBell{
		cooldown: cooldown,
		lastRing: make(map[string]time.Time),
		lastMsg:  make(map[string]int),
	}
}

// ShouldRing returns true if the bell should ring for the given
// room and message ID. Implements per-room cooldown logic matching
// Python's mention bell (5-second cooldown per room).
func (mb *MentionBell) ShouldRing(room string, msgID int) bool {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	now := time.Now()

	// Always ring for a new message (different msgID)
	if mb.lastMsg[room] != msgID {
		mb.lastRing[room] = now
		mb.lastMsg[room] = msgID
		return true
	}

	// Same message — check cooldown
	if last, ok := mb.lastRing[room]; ok && now.Sub(last) < mb.cooldown {
		return false
	}

	mb.lastRing[room] = now
	return true
}
