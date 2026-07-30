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

	// empty display name becomes "Undefined"
	e2 := d2.Find([]byte{9, 9, 9, 9, 9, 9, 9, 9})
	if e2 == nil {
		t.Fatal("second entry not found")
	}
	if e2.DisplayName != "Undefined" {
		t.Errorf("empty DisplayName = %q, want Undefined", e2.DisplayName)
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

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nomadnet-directory-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
