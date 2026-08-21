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

// TestRoomDraftsPythonParity is a LIVE cross-implementation check: it replays
// a save/restore sequence through Python's real ChannelsDisplay._save_room_draft
// / _restore_room_draft (nomadnet.ui.textui.Channels), keyed by _draft_key
// (hub_hash, dest_name, room), with a mock self holding the _room_drafts dict
// and a bound _draft_key, and mock room_widgets whose editor carries the
// save text / captures the restored text. Go owns the operation sequence;
// Python owns the reference behavior. The test SKIPs, not fails, when the
// Python reference is not importable.
//
// The crucial parity point is that a shared hub_hash with a differing
// dest_name is a distinct key (Python's key tuple includes dest_name).
func TestRoomDraftsPythonParity(t *testing.T) {
	t.Parallel()

	type draftOp struct {
		Op   string `json:"op"` // "save" | "restore"
		Hub  string `json:"hub"`
		Dest string `json:"dest"`
		Room string `json:"room"`
		Text string `json:"text"` // save only
	}
	ops := []draftOp{
		{"save", "h1", "dn1", "room1", "draft text"},
		{"restore", "h1", "dn1", "room1", ""},
		{"restore", "h1", "dn1", "room2", ""},
		{"save", "h1", "dn1", "room1", ""},
		{"restore", "h1", "dn1", "room1", ""},
		{"save", "h1", "dn1", "room1", "draft1"},
		{"save", "h1", "dn1", "room2", "draft2"},
		{"save", "h2", "dn1", "room1", "draft3"},
		{"restore", "h1", "dn1", "room1", ""},
		{"restore", "h1", "dn1", "room2", ""},
		{"restore", "h2", "dn1", "room1", ""},
		{"save", "h1", "dn1", "room1", "new draft"},
		{"restore", "h1", "dn1", "room1", ""},
		{"save", "h1", "dn2", "room1", "dn2draft"},
		{"restore", "h1", "dn2", "room1", ""},
		{"restore", "h1", "dn1", "room1", ""},
	}

	const script = `
import sys, json, types
import nomadnet.ui.textui.Channels as C
class Editor:
    def __init__(self, t=""): self._t = t
    def get_edit_text(self): return self._t
    def set_edit_text(self, t): self._t = t
    def set_edit_pos(self, p): pass
class Hub:
    def __init__(self, h, d): self.hub_hash = h; self.dest_name = d
class RoomWidget:
    def __init__(self, h, d, r, t=""): self.hub = Hub(h, d); self.room = r; self.editor = Editor(t)
class Self:
    pass
s = Self(); s._room_drafts = {}
s._draft_key = types.MethodType(C.ChannelsDisplay._draft_key, s)
ops = json.load(sys.stdin)
results = []
for op in ops:
    if op["op"] == "save":
        rw = RoomWidget(op["hub"], op["dest"], op["room"], op["text"])
        C.ChannelsDisplay._save_room_draft(s, rw)
    else:
        rw = RoomWidget(op["hub"], op["dest"], op["room"])
        C.ChannelsDisplay._restore_room_draft(s, rw)
        results.append(rw.editor._t)
json.dump(results, sys.stdout)
`

	var want []string
	runPythonNomadnet(t, ops, script, &want)

	// Replay the same sequence through Go and collect restore results.
	d := NewRoomDrafts()
	var got []string
	for _, op := range ops {
		if op.Op == "save" {
			d.Save(op.Hub, op.Dest, op.Room, op.Text)
		} else {
			got = append(got, d.Restore(op.Hub, op.Dest, op.Room))
		}
	}

	if len(got) != len(want) {
		t.Fatalf("restore count = %v, want %v (Python)", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("restore[%d] = %q, want %q (Python)", i, got[i], want[i])
		}
	}
}
