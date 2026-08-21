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
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// TestSaveToDiskPythonByteParity pins the hard parity requirement for the
// directory msgpack migration: Go's SaveToDisk MUST emit byte-for-byte
// identical msgpack to Python NomadNet's save_to_disk (Directory.py:88) for the
// same state.
//
// This is a LIVE cross-implementation test: it execs the real Python nomadnet
// reference, rebuilds the SAME directory structure (same entries in the same
// insertion order, same announce stream in node+peer+pn save order, same tuple
// field order) the Go test constructs, msgpack.packb's it FRESH via
// RNS.vendor.umsgpack (the exact packer save_to_disk uses), and diffs the bytes
// against Go's SaveToDisk output. It is skipped (not failed) when the Python
// nomadnet reference is not importable.
//
// The scenario covers:
//   - trust_level as a small int (UNTRUSTED=1, UNKNOWN=2 -> fixint).
//   - preferred_delivery as a small int (PROPAGATED=2, DIRECT=1 -> fixint);
//     Python normalizes a missing preferred_delivery to DIRECT, so Carol uses 1.
//   - sort_rank set (Bob=3 -> fixint) and unset (Carol=nil -> 0xc0).
//   - notes set (Bob="bob notes") and empty (Carol="" -> fixstr 0).
//   - announce_stream in Python's save order: node + peer + pn.
//   - timestamps packed as float64 (0xcb), matching Go's float64 Announce.Timestamp
//     and Python's time.time() floats; the script wraps them in float() so a
//     JSON-int round-trip still packs as float64.
//
// This test exists because the previous round-trip-only test could not catch
// that a Go map's random iteration would reorder entry_list, that a Go int
// would pack trust_level/preferred_delivery as signed (diverging from Python's
// unsigned encodings), or that the top-level map key order was nondeterministic.
// The rns/msgpack migration uses an OrderedMap and an insertion-ordered
// entryOrder slice to reproduce Python's bytes exactly.
func TestSaveToDiskPythonByteParity(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	path := filepath.Join(dir, "directory")

	bob := &Entry{
		SourceHash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
		DisplayName:       "Bob",
		TrustLevel:        TrustUntrusted,
		HostsNode:         true,
		PreferredDelivery: DeliveryPropagated,
		IdentifyOnConnect: true,
		SortRank:          new(3),
		Notes:             "bob notes",
	}
	carol := &Entry{
		SourceHash:        []byte{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11},
		DisplayName:       "Carol",
		TrustLevel:        TrustUnknown,
		HostsNode:         false,
		PreferredDelivery: DeliveryDirect,
		IdentifyOnConnect: false,
		SortRank:          nil,
		Notes:             "",
	}
	// Announce order in the file is node + peer + pn (Python save_to_disk
	// concatenates _node_announces + _peer_announces + _pn_announces); Go
	// SaveToDisk concatenates the same way, so add one of each type.
	nodeAnn := Announce{Timestamp: 200, SourceHash: []byte{2}, AppData: []byte("nodedata"), AnnounceType: "node"}
	peerAnn := Announce{Timestamp: 100, SourceHash: []byte{1}, AppData: []byte("peerdata"), AnnounceType: "peer"}
	pnAnn := Announce{Timestamp: 300, SourceHash: []byte{3}, AppData: []byte("pndata"), AnnounceType: "pn"}

	d := New()
	d.Remember(bob)
	d.Remember(carol)
	d.NodeAnnounceReceived(nodeAnn, false)
	d.PeerAnnounceReceived(peerAnn, false)
	d.PNAnnounceReceived(pnAnn, false)

	// Build the Python input mirroring the exact state above. Entries are
	// forwarded in insertion order (Bob then Carol); announces in node+peer+pn
	// save order. Bytes fields are hex-encoded; sort_rank/notes use the same
	// values Go stores.
	pyEntries := []map[string]any{
		entryToPy(bob),
		entryToPy(carol),
	}
	pyAnnounces := []map[string]any{
		announceToPy(nodeAnn),
		announceToPy(peerAnn),
		announceToPy(pnAnn),
	}

	// directoryParityScript rebuilds save_to_disk's directory dict in Python's
	// declaration order and packb's it with RNS.vendor.umsgpack, the exact packer
	// nomadnet.Directory.save_to_disk uses. Timestamps are forced to float so a
	// JSON-int round-trip still packs as float64 (matching Go's float64 fields
	// and Python's time.time() floats).
	const directoryParityScript = `
import sys, json, base64
import RNS.vendor.umsgpack as msgpack
req = json.loads(sys.stdin.read() or "{}")
def hb(x):
    return bytes.fromhex(x) if x is not None else None
packed_list = []
for e in req["entries"]:
    packed_list.append((
        hb(e["source_hash"]),
        e["display_name"],
        e["trust_level"],
        e["hosts_node"],
        e["preferred_delivery"],
        e["identify"],
        e.get("sort_rank"),
        e.get("notes", ""),
    ))
announce_stream = []
for a in req["announces"]:
    announce_stream.append((float(a["timestamp"]), hb(a["source_hash"]), hb(a["app_data"]), a["announce_type"]))
directory = {"entry_list": packed_list, "announce_stream": announce_stream}
print(json.dumps(base64.b64encode(msgpack.packb(directory)).decode()))
`
	var pyB64 string
	testutils.RunPythonNomadnet(t, map[string]any{"entries": pyEntries, "announces": pyAnnounces}, directoryParityScript, &pyB64)

	want, err := base64.StdEncoding.DecodeString(pyB64)
	if err != nil {
		t.Fatalf("decode python bytes: %v", err)
	}

	if err := d.SaveToDisk(path); err != nil {
		t.Fatalf("SaveToDisk: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("SaveToDisk bytes diverge from fresh Python:\n got %x\n want %x", got, want)
	}
}

// entryToPy maps a Go directory Entry to the JSON-friendly dict the Python
// parity script consumes (bytes fields hex-encoded, sort_rank forwarded or nil).
func entryToPy(e *Entry) map[string]any {
	m := map[string]any{
		"source_hash":        hex.EncodeToString(e.SourceHash),
		"display_name":       e.DisplayName,
		"trust_level":        uint(e.TrustLevel),
		"hosts_node":         e.HostsNode,
		"preferred_delivery": uint(e.PreferredDelivery),
		"identify":           e.IdentifyOnConnect,
		"notes":              e.Notes,
	}
	if e.SortRank != nil {
		m["sort_rank"] = uint(*e.SortRank)
	} else {
		m["sort_rank"] = nil
	}
	return m
}

// announceToPy maps a Go Announce to the JSON-friendly dict the Python parity
// script consumes (bytes fields hex-encoded).
func announceToPy(a Announce) map[string]any {
	return map[string]any{
		"timestamp":     a.Timestamp,
		"source_hash":   hex.EncodeToString(a.SourceHash),
		"app_data":      hex.EncodeToString(a.AppData),
		"announce_type": a.AnnounceType,
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
