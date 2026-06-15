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
)

func TestRoomDraftsSaveRestore(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "room1", "draft text")
	got := drafts.Restore("hub1", "room1")
	if got != "draft text" {
		t.Errorf("Restore = %q, want %q", got, "draft text")
	}
}

func TestRoomDraftsEmptyRestore(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	got := drafts.Restore("hub1", "room1")
	if got != "" {
		t.Errorf("Restore on missing = %q, want empty", got)
	}
}

func TestRoomDraftsClearOnEmpty(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "room1", "text")
	drafts.Save("hub1", "room1", "")
	got := drafts.Restore("hub1", "room1")
	if got != "" {
		t.Errorf("Restore after empty save = %q, want empty", got)
	}
}

func TestRoomDraftsMultipleRooms(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "room1", "draft1")
	drafts.Save("hub1", "room2", "draft2")
	drafts.Save("hub2", "room1", "draft3")

	if got := drafts.Restore("hub1", "room1"); got != "draft1" {
		t.Errorf("hub1/room1 = %q, want %q", got, "draft1")
	}
	if got := drafts.Restore("hub1", "room2"); got != "draft2" {
		t.Errorf("hub1/room2 = %q, want %q", got, "draft2")
	}
	if got := drafts.Restore("hub2", "room1"); got != "draft3" {
		t.Errorf("hub2/room1 = %q, want %q", got, "draft3")
	}
}

func TestRoomDraftsOverwrite(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "room1", "old draft")
	drafts.Save("hub1", "room1", "new draft")
	got := drafts.Restore("hub1", "room1")
	if got != "new draft" {
		t.Errorf("Restore = %q, want %q", got, "new draft")
	}
}

func TestRoomDraftsRemove(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "room1", "draft")
	drafts.Remove("hub1", "room1")
	got := drafts.Restore("hub1", "room1")
	if got != "" {
		t.Errorf("Restore after remove = %q, want empty", got)
	}
}
