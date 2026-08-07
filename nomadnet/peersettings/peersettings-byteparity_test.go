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

package peersettings

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestSavePythonByteParity pins the hard parity requirement for the msgpack
// migration: Go's Save MUST emit byte-for-byte identical msgpack to Python
// NomadNet's save_peer_settings (which serialises an insertion-ordered dict
// via umsgpack). The existing TestSavePythonCompatRoundTrip only round-trips
// Go Save -> Go Load and checks field equality — it never compares the bytes
// to the Python golden, so it could not catch that the old vmihailenco codec
// chose different integer widths (signed int16/int32 vs Python's unsigned
// uint16/uint32) and would have diverged on announce_interval and
// last_lxmf_sync. This test embeds the Python-written goldens and asserts
// Save produces them exactly, for both a fully-populated record and an
// all-defaults record.
func TestSavePythonByteParity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    *Settings
	}{
		{
			name: "filled",
			s: &Settings{
				DisplayName:        "Test Peer",
				AnnounceInterval:   720,
				LastAnnounce:       1700000123.456,
				NodeLastAnnounce:   mustHex("00112233445566778899aabbccddeeff"),
				PropagationNode:    mustHex("ffeeddccbbaa99887766554433221100"),
				LastLXMFSync:       1700000200,
				NodeConnects:       42,
				ServedPageRequests: 100,
				ServedFileRequests: 7,
			},
		},
		{
			name: "defaults",
			s:    DefaultSettings(720),
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			want, err := peerFS.ReadFile("testdata/peersettings_" + c.name + ".bin")
			if err != nil {
				t.Fatalf("read golden .bin: %v", err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "peersettings")
			if err := Save(c.s, path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("Save(%v) bytes diverge from Python golden:\n got %x\n want %x", c.name, got, want)
			}
		})
	}
}