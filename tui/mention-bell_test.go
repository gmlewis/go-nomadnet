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
	"testing"
	"time"
)

func TestMentionBellFirstMention(t *testing.T) {
	t.Parallel()

	bell := NewMentionBell(5 * time.Second)
	if !bell.ShouldRing("room1", 1) {
		t.Error("first mention should ring")
	}
}

func TestMentionBellCooldown(t *testing.T) {
	t.Parallel()

	bell := NewMentionBell(5 * time.Second)
	bell.ShouldRing("room1", 1)

	// Same message again — should not ring
	if bell.ShouldRing("room1", 1) {
		t.Error("same message should not ring again")
	}
}

func TestMentionBellDifferentRoom(t *testing.T) {
	t.Parallel()

	bell := NewMentionBell(5 * time.Second)
	bell.ShouldRing("room1", 1)

	// Different room — should ring
	if !bell.ShouldRing("room2", 1) {
		t.Error("different room should ring")
	}
}

func TestMentionBellDifferentMessage(t *testing.T) {
	t.Parallel()

	bell := NewMentionBell(5 * time.Second)
	bell.ShouldRing("room1", 1)

	// Different message in same room — should ring
	if !bell.ShouldRing("room1", 2) {
		t.Error("different message should ring")
	}
}

func TestMentionBellResetCooldown(t *testing.T) {
	t.Parallel()

	bell := NewMentionBell(1 * time.Millisecond)
	bell.ShouldRing("room1", 1)

	time.Sleep(2 * time.Millisecond)

	// After cooldown expires — should ring again
	if !bell.ShouldRing("room1", 2) {
		t.Error("should ring after cooldown")
	}
}

func TestMentionBellMultipleRooms(t *testing.T) {
	t.Parallel()

	bell := NewMentionBell(5 * time.Second)
	bell.ShouldRing("room1", 1)
	bell.ShouldRing("room2", 1)
	bell.ShouldRing("room3", 1)

	// Each room independent — same msg in same room should not ring
	if bell.ShouldRing("room1", 1) {
		t.Error("room1 should still be in cooldown")
	}
	if bell.ShouldRing("room2", 1) {
		t.Error("room2 should still be in cooldown")
	}
	if bell.ShouldRing("room3", 1) {
		t.Error("room3 should still be in cooldown")
	}
}
