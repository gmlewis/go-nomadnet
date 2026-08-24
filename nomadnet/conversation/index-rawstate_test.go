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

package conversation

import (
	"os"
	"path/filepath"
	"testing"

	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// TestToIndexEntryStoresRawLXMFState verifies that ToIndexEntry stores the
// RAW LXMF wire state (e.g. 0x04 SENT, 0xFF FAILED) in the "state" field,
// matching Python's ConversationMessage.to_index_entry which stores
// self._cached_state = self.lxm.state (the raw LXMF constant). The Go port
// previously stored the MAPPED conversation MessageState (0-6), which is
// incompatible: when Python reads a Go-written .index it interprets the
// mapped values as raw LXMF constants, and since 3/5 match no LXMF state
// (SENT=4, DELIVERED=8, FAILED=255) every outbound header falls to the
// default "→" branch instead of "↑ →"/"✕ →".
func TestToIndexEntryStoresRawLXMFState(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "msg")
	writeFile(t, path, "x")

	cases := []struct {
		name     string
		rawState int
	}{
		{"sent", 0x04},
		{"delivered", 0x08},
		{"failed", 0xFF},
		{"propagated_sent", 0x04},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			msg := NewMessage(path)
			msg.CachedRawState = c.rawState
			st := mapLXMFState(c.rawState)
			msg.CachedState = &st

			entry := msg.ToIndexEntry()
			data, err := rnsmsgpack.Pack(entry)
			if err != nil {
				t.Fatalf("Pack: %v", err)
			}
			raw, err := rnsmsgpack.UnpackPreserveBinMapKeyOrder(data)
			if err != nil {
				t.Fatalf("Unpack: %v", err)
			}
			om := raw.(rnsmsgpack.OrderedMap)
			v, ok := om.Get("state")
			if !ok {
				t.Fatal("index entry missing 'state' key")
			}
			got, ok := toInt(v)
			if !ok {
				t.Fatalf("index state type = %T, want int", v)
			}
			if got != c.rawState {
				t.Errorf("index state = %d, want raw LXMF %d (got mapped value instead)", got, c.rawState)
			}
		})
	}
}

// TestRestoreFromIndexSetsRawState verifies RestoreFromIndex populates
// CachedRawState from the index "state" (raw LXMF) and derives the mapped
// CachedState from it, so DisplayMessages can use the index-restored raw
// state for header rendering without loading from disk (matching Python's
// lazy get_state which returns _cached_state without loading).
func TestRestoreFromIndexSetsRawState(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "msg")
	writeFile(t, path, "x")

	entry := rnsmsgpack.OrderedMap{
		{Key: "state", Value: uint(0xFF)}, // raw LXMF FAILED
		{Key: "method", Value: uint(3)},
	}
	msg := NewMessage(path)
	msg.RestoreFromIndex(entry)

	if msg.CachedRawState != 0xFF {
		t.Errorf("CachedRawState = %#x, want %#x (raw LXMF FAILED)", msg.CachedRawState, 0xFF)
	}
	if msg.CachedState == nil || *msg.CachedState != StateFailed {
		got := "nil"
		if msg.CachedState != nil {
			got = stateName(*msg.CachedState)
		}
		t.Errorf("CachedState = %v, want StateFailed (mapped from raw 0xFF)", got)
	}
}

func stateName(s MessageState) string {
	switch s {
	case StateDraft:
		return "StateDraft"
	case StateGenerating:
		return "StateGenerating"
	case StatePending:
		return "StatePending"
	case StateSent:
		return "StateSent"
	case StateDelivered:
		return "StateDelivered"
	case StateFailed:
		return "StateFailed"
	case StatePaper:
		return "StatePaper"
	}
	return "unknown"
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
