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
	drafts.Save("hub1", "dn1", "room1", "draft text")
	got := drafts.Restore("hub1", "dn1", "room1")
	if got != "draft text" {
		t.Errorf("Restore = %q, want %q", got, "draft text")
	}
}

func TestRoomDraftsEmptyRestore(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	got := drafts.Restore("hub1", "dn1", "room1")
	if got != "" {
		t.Errorf("Restore on missing = %q, want empty", got)
	}
}

func TestRoomDraftsClearOnEmpty(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "dn1", "room1", "text")
	drafts.Save("hub1", "dn1", "room1", "")
	got := drafts.Restore("hub1", "dn1", "room1")
	if got != "" {
		t.Errorf("Restore after empty save = %q, want empty", got)
	}
}

func TestRoomDraftsMultipleRooms(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "dn1", "room1", "draft1")
	drafts.Save("hub1", "dn1", "room2", "draft2")
	drafts.Save("hub2", "dn1", "room1", "draft3")

	if got := drafts.Restore("hub1", "dn1", "room1"); got != "draft1" {
		t.Errorf("hub1/room1 = %q, want %q", got, "draft1")
	}
	if got := drafts.Restore("hub1", "dn1", "room2"); got != "draft2" {
		t.Errorf("hub1/room2 = %q, want %q", got, "draft2")
	}
	if got := drafts.Restore("hub2", "dn1", "room1"); got != "draft3" {
		t.Errorf("hub2/room1 = %q, want %q", got, "draft3")
	}
}

func TestRoomDraftsOverwrite(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "dn1", "room1", "old draft")
	drafts.Save("hub1", "dn1", "room1", "new draft")
	got := drafts.Restore("hub1", "dn1", "room1")
	if got != "new draft" {
		t.Errorf("Restore = %q, want %q", got, "new draft")
	}
}

func TestRoomDraftsRemove(t *testing.T) {
	t.Parallel()

	drafts := NewRoomDrafts()
	drafts.Save("hub1", "dn1", "room1", "draft")
	drafts.Remove("hub1", "dn1", "room1")
	got := drafts.Restore("hub1", "dn1", "room1")
	if got != "" {
		t.Errorf("Restore after remove = %q, want empty", got)
	}
}

// TestRoomDraftsPythonParity locks the room-draft store against Python's
// _save_room_draft / _restore_room_draft (Channels.py:1506,1519) keyed by
// _draft_key (hub_hash, dest_name, room) (Channels.py:1500). Expected restore
// results were captured from /tmp/draft_ref.py. The crucial parity point is
// that a shared hub_hash with differing dest_name is a distinct key.
func TestRoomDraftsPythonParity(t *testing.T) {
	t.Parallel()

	d := NewRoomDrafts()
	d.Save("h1", "dn1", "room1", "draft text")
	if got := d.Restore("h1", "dn1", "room1"); got != "draft text" {
		t.Errorf("restore h1/dn1/room1 = %q, want %q", got, "draft text")
	}
	if got := d.Restore("h1", "dn1", "room2"); got != "" {
		t.Errorf("restore missing = %q, want empty", got)
	}
	d.Save("h1", "dn1", "room1", "")
	if got := d.Restore("h1", "dn1", "room1"); got != "" {
		t.Errorf("after empty save = %q, want empty", got)
	}
	d.Save("h1", "dn1", "room1", "draft1")
	d.Save("h1", "dn1", "room2", "draft2")
	d.Save("h2", "dn1", "room1", "draft3")
	if got := d.Restore("h1", "dn1", "room1"); got != "draft1" {
		t.Errorf("multi room1 = %q, want %q", got, "draft1")
	}
	if got := d.Restore("h1", "dn1", "room2"); got != "draft2" {
		t.Errorf("multi room2 = %q, want %q", got, "draft2")
	}
	if got := d.Restore("h2", "dn1", "room1"); got != "draft3" {
		t.Errorf("multi h2 room1 = %q, want %q", got, "draft3")
	}
	d.Save("h1", "dn1", "room1", "new draft")
	if got := d.Restore("h1", "dn1", "room1"); got != "new draft" {
		t.Errorf("overwrite = %q, want %q", got, "new draft")
	}
	// Same hub_hash, different dest_name -> distinct keys.
	d.Save("h1", "dn2", "room1", "dn2draft")
	if got := d.Restore("h1", "dn2", "room1"); got != "dn2draft" {
		t.Errorf("same hash diff dest dn2 = %q, want %q", got, "dn2draft")
	}
	if got := d.Restore("h1", "dn1", "room1"); got != "new draft" {
		t.Errorf("same hash diff dest dn1 = %q, want %q", got, "new draft")
	}
}
