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

package directory

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

//go:embed testdata/py-directory-save.msgpack
var pyDirectorySaveGolden []byte

// TestSaveToDiskPythonByteParity pins the hard parity requirement for the
// directory msgpack migration: Go's SaveToDisk MUST emit byte-for-byte
// identical msgpack to Python NomadNet's save_to_disk for the same state.
//
// The golden (testdata/py-directory-save.msgpack) was produced by replicating
// nomadnet Directory.save_to_disk (Directory.py:88-101) through msgpack.packb
// with default options: an insertion-ordered map {"entry_list": [...],
// "announce_stream": [...]} where each entry is the 8-tuple
// (source_hash, display_name, trust_level, hosts_node, preferred_delivery,
// identify, sort_rank, notes) and each announce is the 4-tuple
// (timestamp, source_hash, app_data, announce_type). The scenario covers:
//   - trust_level as a small int (UNTRUSTED=1, UNKNOWN=2 -> fixint).
//   - preferred_delivery as a small int (PROPAGATED=2, DIRECT=1 -> fixint);
//     Python normalizes a missing preferred_delivery to DIRECT, so Carol uses 1.
//   - sort_rank set (Bob=3 -> fixint) and unset (Carol=nil -> 0xc0).
//   - notes set (Bob="bob notes") and empty (Carol="" -> fixstr 0).
//   - announce_stream in Python's save order: node + peer + pn.
//
// This test exists because the previous round-trip-only test could not catch
// that a Go map's random iteration would reorder entry_list, that a Go int
// would pack trust_level/preferred_delivery as signed (diverging from
// Python's unsigned encodings), or that the top-level map key order was
// nondeterministic. The rns/msgpack migration uses an OrderedMap and an
// insertion-ordered entryOrder slice to reproduce Python's bytes exactly.
func TestSaveToDiskPythonByteParity(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	path := filepath.Join(dir, "directory")

	d := New()
	rank := 3
	d.Remember(&Entry{
		SourceHash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
		DisplayName:       "Bob",
		TrustLevel:        TrustUntrusted,
		HostsNode:         true,
		PreferredDelivery: DeliveryPropagated,
		IdentifyOnConnect: true,
		SortRank:          &rank,
		Notes:             "bob notes",
	})
	d.Remember(&Entry{
		SourceHash:        []byte{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11},
		DisplayName:       "Carol",
		TrustLevel:        TrustUnknown,
		HostsNode:         false,
		PreferredDelivery: DeliveryDirect,
		IdentifyOnConnect: false,
		SortRank:          nil,
		Notes:             "",
	})
	// Announce order in the file is node + peer + pn (Python save_to_disk
	// concatenates _node_announces + _peer_announces + _pn_announces); Go
	// SaveToDisk concatenates the same way, so add one of each type.
	d.NodeAnnounceReceived(Announce{Timestamp: 200, SourceHash: []byte{2}, AppData: []byte("nodedata"), AnnounceType: "node"}, false)
	d.PeerAnnounceReceived(Announce{Timestamp: 100, SourceHash: []byte{1}, AppData: []byte("peerdata"), AnnounceType: "peer"}, false)
	d.PNAnnounceReceived(Announce{Timestamp: 300, SourceHash: []byte{3}, AppData: []byte("pndata"), AnnounceType: "pn"}, false)

	if err := d.SaveToDisk(path); err != nil {
		t.Fatalf("SaveToDisk: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, pyDirectorySaveGolden) {
		t.Fatalf("SaveToDisk bytes diverge from Python golden:\n got %x\n want %x", got, pyDirectorySaveGolden)
	}
}

// TestSaveToDiskLoadFromDiskOrderPreserved pins the insertion-order tracking:
// after loading a Python-written file and re-saving, the entry_list order is
// preserved (matching Python, whose insertion-ordered dict keeps file order
// across a load/save cycle). The committed synthetic golden has two entries;
// re-saving must keep them in the same order and produce stable bytes across
// repeated save rounds.
func TestSaveToDiskLoadFromDiskOrderPreserved(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	path := filepath.Join(dir, "directory")

	d := New()
	if err := d.LoadFromDisk("testdata/py-directory.msgpack"); err != nil {
		t.Fatalf("LoadFromDisk: %v", err)
	}
	// Capture the entry order right after load.
	firstOrder := append([]string(nil), d.entryOrder...)
	if len(firstOrder) != 2 {
		t.Fatalf("expected 2 entries loaded, got %v", len(firstOrder))
	}

	if err := d.SaveToDisk(path); err != nil {
		t.Fatalf("SaveToDisk: %v", err)
	}
	if !reflect.DeepEqual(d.entryOrder, firstOrder) {
		t.Errorf("entryOrder changed across save: got %v, want %v", d.entryOrder, firstOrder)
	}

	// Reload the re-saved file; order must still match the original.
	d2 := New()
	if err := d2.LoadFromDisk(path); err != nil {
		t.Fatalf("LoadFromDisk reloaded: %v", err)
	}
	if !reflect.DeepEqual(d2.entryOrder, firstOrder) {
		t.Errorf("entryOrder after reload changed: got %v, want %v", d2.entryOrder, firstOrder)
	}
}
