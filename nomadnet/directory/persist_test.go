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
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/testutils"
)

func TestDirectorySaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	path := filepath.Join(dir, "directory")

	d := New()
	rank := 7
	d.Remember(&Entry{
		SourceHash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
		DisplayName:       "Alice",
		TrustLevel:        TrustTrusted,
		HostsNode:         true,
		PreferredDelivery: DeliveryPropagated,
		IdentifyOnConnect: true,
		SortRank:          &rank,
		Notes:             "a note",
	})
	d.Remember(&Entry{
		SourceHash:  []byte{9, 9, 9, 9, 9, 9, 9, 9},
		DisplayName: "",
		TrustLevel:  TrustUnknown,
	})
	d.PeerAnnounceReceived(Announce{Timestamp: 100, SourceHash: []byte{1}, AppData: []byte("peerdata"), AnnounceType: "peer"}, false)
	d.NodeAnnounceReceived(Announce{Timestamp: 200, SourceHash: []byte{2}, AppData: []byte("nodedata"), AnnounceType: "node"}, false)
	d.PNAnnounceReceived(Announce{Timestamp: 300, SourceHash: []byte{3}, AppData: []byte("pndata"), AnnounceType: "pn"}, false)

	if err := d.SaveToDisk(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("directory file should exist")
	}

	d2 := New()
	if err := d2.LoadFromDisk(path); err != nil {
		t.Fatal(err)
	}

	// entries
	e := d2.Find([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if e == nil {
		t.Fatal("entry not found after load")
	}
	if e.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want Alice", e.DisplayName)
	}
	if e.TrustLevel != TrustTrusted {
		t.Errorf("TrustLevel = %v, want trusted", e.TrustLevel)
	}
	if !e.HostsNode {
		t.Error("HostsNode should be true")
	}
	if e.PreferredDelivery != DeliveryPropagated {
		t.Errorf("PreferredDelivery = %v, want propagated", e.PreferredDelivery)
	}
	if !e.IdentifyOnConnect {
		t.Error("IdentifyOnConnect should be true")
	}
	if e.SortRank == nil || *e.SortRank != 7 {
		t.Errorf("SortRank = %v, want 7", e.SortRank)
	}
	if e.Notes != "a note" {
		t.Errorf("Notes = %q, want 'a note'", e.Notes)
	}

	// An empty display name round-trips as an empty string: Python's
	// load_from_disk (Directory.py:116-117) substitutes "Undefined" only for
	// a nil display_name (saved None), never for a saved empty string, so an
	// empty stored name keeps falling back to the announced app-data name
	// (Conversation.py:151-152) after a restart.
	e2 := d2.Find([]byte{9, 9, 9, 9, 9, 9, 9, 9})
	if e2 == nil {
		t.Fatal("second entry not found")
	}
	if e2.DisplayName != "" {
		t.Errorf("empty DisplayName round-trip = %q, want empty", e2.DisplayName)
	}

	// announces split by type
	peers := d2.PeerAnnounces()
	if len(peers) != 1 || peers[0].AnnounceType != "peer" {
		t.Fatalf("peer announces = %v", peers)
	}
	nodes := d2.NodeAnnounces()
	if len(nodes) != 1 || nodes[0].AnnounceType != "node" {
		t.Fatalf("node announces = %v", nodes)
	}
	pns := d2.PNAnnounces()
	if len(pns) != 1 || pns[0].AnnounceType != "pn" {
		t.Fatalf("pn announces = %v", pns)
	}
}

// TestRememberEagerPersist pins the Python parity behavior that Remember
// saves the directory to disk eagerly when a persist path is configured (Python
// Directory.remember → save_to_disk, Directory.py:340-351). Without this, a Go
// process that exits without reaching Shutdown (Ctrl-C, SIGHUP, crash) loses
// every discovered node, which is the root cause of "gonomadnet doesn't retain
// node history across runs".
func TestRememberEagerPersist(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	path := filepath.Join(dir, "directory")

	d := New()
	d.SetPersistPath(path)

	// Before any Remember, no file is written.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("directory file should not exist before Remember, got err=%v", err)
	}

	// First Remember must write the file.
	d.Remember(&Entry{
		SourceHash:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		DisplayName: "Alice",
		TrustLevel:  TrustTrusted,
		HostsNode:   true,
	})
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("directory file should exist after Remember, got err=%v", err)
	}

	// The written file must round-trip: a fresh Directory loads the entry.
	d2 := New()
	if err := d2.LoadFromDisk(path); err != nil {
		t.Fatal(err)
	}
	if e := d2.Find([]byte{1, 2, 3, 4, 5, 6, 7, 8}); e == nil || e.DisplayName != "Alice" {
		t.Fatalf("after reload, entry = %+v, want Alice", e)
	}

	// A second Remember overwrites the entry and re-saves.
	d.Remember(&Entry{
		SourceHash:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		DisplayName: "Alicia",
		TrustLevel:  TrustTrusted,
	})
	d3 := New()
	if err := d3.LoadFromDisk(path); err != nil {
		t.Fatal(err)
	}
	if e := d3.Find([]byte{1, 2, 3, 4, 5, 6, 7, 8}); e == nil || e.DisplayName != "Alicia" {
		t.Fatalf("after second Remember + reload, entry = %+v, want Alicia", e)
	}

	// Remember must also persist the announce stream (Python save_to_disk
	// serializes entries + announce_stream together).
	d.NodeAnnounceReceived(Announce{Timestamp: 123, SourceHash: []byte{2}, AppData: []byte("nodedata"), AnnounceType: "node"}, false)
	d.Remember(&Entry{SourceHash: []byte{9, 9, 9, 9, 9, 9, 9, 9}, DisplayName: "Bob", TrustLevel: TrustTrusted})
	d4 := New()
	if err := d4.LoadFromDisk(path); err != nil {
		t.Fatal(err)
	}
	if nodes := d4.NodeAnnounces(); len(nodes) != 1 || string(nodes[0].AppData) != "nodedata" {
		t.Fatalf("announce stream not persisted with Remember, nodes=%v", nodes)
	}
}

// TestRememberNoPersistPath verifies that with no persist path configured,
// Remember only updates in-memory state and never writes a file (and does not
// panic), preserving the historical pre-SetPersistPath behavior for callers
// that manage persistence themselves (e.g. integration tests).
func TestRememberNoPersistPath(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	path := filepath.Join(dir, "directory")

	d := New() // no SetPersistPath
	d.Remember(&Entry{SourceHash: []byte{1, 2, 3, 4}, DisplayName: "Alice", TrustLevel: TrustTrusted})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("no persist path: file should not be written, got err=%v", err)
	}
	// In-memory state still updated.
	if e := d.Find([]byte{1, 2, 3, 4}); e == nil || e.DisplayName != "Alice" {
		t.Fatalf("in-memory entry = %+v, want Alice", e)
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	return testutils.TempDir(t, "nomadnet-directory-test")
}
