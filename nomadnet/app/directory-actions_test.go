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

package app

import (
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
)

func TestCreateDirectoryEntry(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{1, 2, 3, 4}
	entry := a.CreateDirectoryEntry(hash, "alice")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if got := a.Dir.Find(hash); got == nil || got.DisplayName != "alice" {
		t.Fatalf("entry not remembered: %+v", got)
	}
}

func TestSaveNodeAndForget(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{9, 9, 9}
	a.SaveNode(hash, "node1")
	if a.Dir.Find(hash) == nil {
		t.Fatal("SaveNode did not remember entry")
	}
	a.ForgetNode(hash)
	if a.Dir.Find(hash) != nil {
		t.Fatal("ForgetNode did not remove entry")
	}
}

func TestForgetNodeNilDir(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Dir = nil
	a.ForgetNode([]byte{1}) // must not panic
}

func TestSetPeerDisplayName(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{7, 7}
	a.CreateDirectoryEntry(hash, "")
	a.SetPeerDisplayName(hash, "bob")
	if got := a.PeerDisplayName(hash); got != "bob" {
		t.Fatalf("PeerDisplayName=%q want bob", got)
	}
	// SetPeerDisplayName on an unknown peer creates the entry.
	hash2 := []byte{8, 8}
	a.SetPeerDisplayName(hash2, "carol")
	if got := a.PeerDisplayName(hash2); got != "carol" {
		t.Fatalf("PeerDisplayName=%q want carol", got)
	}
}

// TestRememberPeerInfoRoundTrip verifies RememberPeerInfo writes the full set
// of editable Peer Info fields and PeerInfoLoad reads them back, mirroring
// Python's confirmed() save and the existing_entry pre-fill lookup
// (Conversations.py:844-929). It also confirms that the sort_rank (Pin) and
// HostsNode/IdentifyOnConnect flags round-trip correctly.
func TestRememberPeerInfoRoundTrip(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{0xaa, 0xbb, 0xcc}

	// A peer with no entry loads the Python defaults: Unknown trust, direct
	// delivery, unpinned, empty notes.
	loaded := a.PeerInfoLoad(hash)
	if loaded.TrustLevel != directory.TrustUnknown {
		t.Errorf("default TrustLevel=%x want %x", loaded.TrustLevel, directory.TrustUnknown)
	}
	if loaded.PreferredDelivery != directory.DeliveryDirect {
		t.Errorf("default PreferredDelivery=%x want %x", loaded.PreferredDelivery, directory.DeliveryDirect)
	}
	if loaded.Pinned {
		t.Error("default Pinned should be false")
	}
	if loaded.Notes != "" {
		t.Errorf("default Notes=%q want empty", loaded.Notes)
	}

	// Save the full peer-info edit.
	a.RememberPeerInfo(hash, PeerInfoData{
		DisplayName:       "Alice",
		TrustLevel:        directory.TrustTrusted,
		PreferredDelivery: directory.DeliveryPropagated,
		Pinned:            true,
		Notes:             "a note",
	})

	loaded = a.PeerInfoLoad(hash)
	if loaded.DisplayName != "Alice" {
		t.Errorf("DisplayName=%q want Alice", loaded.DisplayName)
	}
	if loaded.TrustLevel != directory.TrustTrusted {
		t.Errorf("TrustLevel=%x want %x", loaded.TrustLevel, directory.TrustTrusted)
	}
	if loaded.PreferredDelivery != directory.DeliveryPropagated {
		t.Errorf("PreferredDelivery=%x want %x", loaded.PreferredDelivery, directory.DeliveryPropagated)
	}
	if !loaded.Pinned {
		t.Error("Pinned should be true")
	}
	if loaded.Notes != "a note" {
		t.Errorf("Notes=%q want %q", loaded.Notes, "a note")
	}

	// The remembered entry carries the SortRank pointer for a pinned peer.
	entry := a.Dir.Find(hash)
	if entry == nil || entry.SortRank == nil {
		t.Fatal("pinned entry should have a non-nil SortRank")
	}
	if *entry.SortRank != 0 {
		t.Errorf("SortRank=%d want 0", *entry.SortRank)
	}

	// Unpinning the peer clears the SortRank (sort_rank=None in Python).
	a.RememberPeerInfo(hash, PeerInfoData{
		DisplayName:       "Alice",
		TrustLevel:        directory.TrustTrusted,
		PreferredDelivery: directory.DeliveryDirect,
		Pinned:            false,
		Notes:             "a note",
	})
	if entry := a.Dir.Find(hash); entry == nil || entry.SortRank != nil {
		t.Fatal("unpinned entry should have a nil SortRank")
	}
}

// TestRememberPeerInfoPreservesNodeFlags verifies that RememberPeerInfo
// preserves the HostsNode and IdentifyOnConnect flags of an existing entry,
// mirroring Python's remember() node-entry merge (Directory.py:198-202).
func TestRememberPeerInfoPreservesNodeFlags(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Dir = directory.New()
	hash := []byte{0x01, 0x02, 0x03}

	entry := directory.NewEntry(hash)
	entry.HostsNode = true
	entry.IdentifyOnConnect = true
	a.Dir.Remember(entry)

	a.RememberPeerInfo(hash, PeerInfoData{
		DisplayName: "NodePeer",
		TrustLevel:  directory.TrustTrusted,
	})
	after := a.Dir.Find(hash)
	if after == nil {
		t.Fatal("entry missing after RememberPeerInfo")
	}
	if !after.HostsNode {
		t.Error("HostsNode flag not preserved")
	}
	if !after.IdentifyOnConnect {
		t.Error("IdentifyOnConnect flag not preserved")
	}
}

func TestRemoveAnnounce(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Dir = directory.New()
	ts := 12345.6
	a.Dir.PeerAnnounceReceived(directory.Announce{
		Timestamp:  ts,
		SourceHash: []byte{1},
	}, false)
	if len(a.Dir.AnnounceStream()) != 1 {
		t.Fatalf("expected 1 announce, got %d", len(a.Dir.AnnounceStream()))
	}
	a.RemoveAnnounce(ts)
	if len(a.Dir.AnnounceStream()) != 0 {
		t.Fatalf("expected 0 announces after remove, got %d", len(a.Dir.AnnounceStream()))
	}
	// Nil-dir guard.
	a.Dir = nil
	a.RemoveAnnounce(ts) // must not panic
}

func TestSourceHashFromHex(t *testing.T) {
	t.Parallel()
	b, ok := SourceHashFromHex("01020304")
	if !ok || len(b) != 4 || b[0] != 1 {
		t.Fatalf("decode got %v ok=%v", b, ok)
	}
	if _, ok := SourceHashFromHex("not-hex"); ok {
		t.Fatal("expected invalid hex")
	}
}

func TestLXMFAddressHex(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if got := a.LXMFAddressHex(); got != "" {
		t.Fatalf("expected empty when LXMFDest is nil, got %q", got)
	}
}

// TestPeerStampCostNilGuards verifies PeerStampCost returns nil (omit the
// "Stamp:" segment, matching Python stamp_cost is None) when no router or
// transport is available and no app_data can be recalled. The realistic RNS
// resolution path is exercised by integration tests; this pins the guards.
func TestPeerStampCostNilGuards(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{0xaa, 0xbb, 0xcc}

	if got := a.PeerStampCost(hash); got != nil {
		t.Errorf("PeerStampCost with no router/transport = %v, want nil", *got)
	}
	if got := a.PeerStampCost(nil); got != nil {
		t.Errorf("PeerStampCost(nil) = %v, want nil", *got)
	}
}

// TestPeerHopsNilGuards verifies PeerHops returns nil ("unknown") when no
// transport is available. The PathfinderM→nil mapping (unknown path) is
// exercised by integration tests; this pins the no-transport guard.
func TestPeerHopsNilGuards(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{0xaa, 0xbb, 0xcc}

	if got := a.PeerHops(hash); got != nil {
		t.Errorf("PeerHops with no transport = %v, want nil", *got)
	}
	if got := a.PeerHops(nil); got != nil {
		t.Errorf("PeerHops(nil) = %v, want nil", *got)
	}
}
