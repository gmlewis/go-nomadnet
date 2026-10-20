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
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// hash16 builds a 16-byte source hash from the given leading bytes, zero
// filled. Directory entries require a full truncated RNS identity hash
// (SourceHashLength; Python DirectoryEntry raises for anything else,
// Directory.py:419,433-434), so test fixtures use this rather than a short
// literal.
func hash16(leading ...byte) []byte {
	out := make([]byte, SourceHashLength)
	copy(out, leading)
	return out
}

func TestTrustLevels(t *testing.T) {
	t.Parallel()

	if TrustWarning != 0x00 {
		t.Errorf("TrustWarning = 0x%02X, want 0x00", TrustWarning)
	}
	if TrustUntrusted != 0x01 {
		t.Errorf("TrustUntrusted = 0x%02X, want 0x01", TrustUntrusted)
	}
	if TrustUnknown != 0x02 {
		t.Errorf("TrustUnknown = 0x%02X, want 0x02", TrustUnknown)
	}
	if TrustTrusted != 0xFF {
		t.Errorf("TrustTrusted = 0x%02X, want 0xFF", TrustTrusted)
	}
}

func TestDeliveryTypes(t *testing.T) {
	t.Parallel()

	if DeliveryDirect != 0x01 {
		t.Errorf("DeliveryDirect = 0x%02X, want 0x01", DeliveryDirect)
	}
	if DeliveryPropagated != 0x02 {
		t.Errorf("DeliveryPropagated = 0x%02X, want 0x02", DeliveryPropagated)
	}
}

func TestNewEntry(t *testing.T) {
	t.Parallel()

	hash := hash16(0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}

	if len(entry.SourceHash) != SourceHashLength {
		t.Errorf("SourceHash len = %v, want %v", len(entry.SourceHash), SourceHashLength)
	}
	if entry.TrustLevel != TrustUnknown {
		t.Errorf("TrustLevel = 0x%02X, want 0x%02X", entry.TrustLevel, TrustUnknown)
	}
	if entry.PreferredDelivery != DeliveryDirect {
		t.Errorf("PreferredDelivery = 0x%02X, want 0x%02X", entry.PreferredDelivery, DeliveryDirect)
	}
	if entry.HostsNode {
		t.Error("HostsNode = true, want false")
	}
	if entry.IdentifyOnConnect {
		t.Error("IdentifyOnConnect = true, want false")
	}
	if entry.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty", entry.DisplayName)
	}
}

func TestRememberAndFind(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0xAA, 0xBB, 0xCC, 0xDD)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	entry.DisplayName = "Test Peer"
	entry.TrustLevel = TrustTrusted

	d.Remember(entry)

	found := d.Find(hash)
	if found == nil {
		t.Fatal("Find returned nil for remembered entry")
	}
	if found.DisplayName != "Test Peer" {
		t.Errorf("DisplayName = %q, want %q", found.DisplayName, "Test Peer")
	}
	if found.TrustLevel != TrustTrusted {
		t.Errorf("TrustLevel = 0x%02X, want 0x%02X", found.TrustLevel, TrustTrusted)
	}
}

func TestForget(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	d.Remember(entry)

	d.Forget(hash)

	found := d.Find(hash)
	if found != nil {
		t.Error("Find returned non-nil after Forget")
	}
}

func TestDisplayName(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	entry.DisplayName = "Alice"

	d.Remember(entry)

	name := d.DisplayName(hash)
	if name != "Alice" {
		t.Errorf("DisplayName = %q, want %q", name, "Alice")
	}

	// Unknown hash returns empty
	unknown := hash16(0xFF, 0xFE, 0xFD, 0xFC)
	if name := d.DisplayName(unknown); name != "" {
		t.Errorf("DisplayName for unknown = %q, want empty", name)
	}
}

func TestTrustLevel(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	entry.TrustLevel = TrustTrusted
	d.Remember(entry)

	// Direct lookup
	level := d.TrustLevel(hash, nil)
	if level != TrustTrusted {
		t.Errorf("TrustLevel = 0x%02X, want 0x%02X", level, TrustTrusted)
	}

	// Unknown hash returns UNKNOWN
	unknown := hash16(0xFF, 0xFE, 0xFD, 0xFC)
	level = d.TrustLevel(unknown, nil)
	if level != TrustUnknown {
		t.Errorf("TrustLevel for unknown = 0x%02X, want 0x%02X", level, TrustUnknown)
	}
}

func TestTrustLevelNameCollision(t *testing.T) {
	t.Parallel()

	d := New()

	// Add two entries with different hashes but same display name
	hash1 := hash16(0x01, 0x02, 0x03, 0x04)
	entry1, err := NewEntry(hash1)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash1, err)
	}
	entry1.DisplayName = "SharedName"
	entry1.TrustLevel = TrustUntrusted
	d.Remember(entry1)

	hash2 := hash16(0x05, 0x06, 0x07, 0x08)
	entry2, err := NewEntry(hash2)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash2, err)
	}
	entry2.DisplayName = "SharedName"
	entry2.TrustLevel = TrustUnknown
	d.Remember(entry2)

	// Check trust level with announced name collision
	name := "SharedName"
	level := d.TrustLevel(hash2, &name)
	if level != TrustWarning {
		t.Errorf("TrustLevel with name collision = 0x%02X, want 0x%02X", level, TrustWarning)
	}

	// TRUSTED entries should not trigger WARNING for name collisions
	entry1.TrustLevel = TrustTrusted
	level = d.TrustLevel(hash1, &name)
	if level != TrustTrusted {
		t.Errorf("TrustLevel for trusted = 0x%02X, want 0x%02X", level, TrustTrusted)
	}
}

func TestPreferredDelivery(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	entry.PreferredDelivery = DeliveryPropagated
	d.Remember(entry)

	delivery := d.PreferredDelivery(hash)
	if delivery != DeliveryPropagated {
		t.Errorf("PreferredDelivery = 0x%02X, want 0x%02X", delivery, DeliveryPropagated)
	}

	// Unknown hash returns DIRECT
	unknown := hash16(0xFF, 0xFE, 0xFD, 0xFC)
	delivery = d.PreferredDelivery(unknown)
	if delivery != DeliveryDirect {
		t.Errorf("PreferredDelivery for unknown = 0x%02X, want 0x%02X", delivery, DeliveryDirect)
	}
}

func TestShouldIdentifyOnConnect(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	entry.IdentifyOnConnect = true
	d.Remember(entry)

	if !d.ShouldIdentifyOnConnect(hash) {
		t.Error("ShouldIdentifyOnConnect = false, want true")
	}

	unknown := hash16(0xFF, 0xFE, 0xFD, 0xFC)
	if d.ShouldIdentifyOnConnect(unknown) {
		t.Error("ShouldIdentifyOnConnect for unknown = true, want false")
	}
}

func TestSetIdentifyOnConnect(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	d.Remember(entry)

	d.SetIdentifyOnConnect(hash, true)
	if !d.ShouldIdentifyOnConnect(hash) {
		t.Error("SetIdentifyOnConnect(true) did not take effect")
	}

	d.SetIdentifyOnConnect(hash, false)
	if d.ShouldIdentifyOnConnect(hash) {
		t.Error("SetIdentifyOnConnect(false) did not take effect")
	}
}

func TestSimplestDisplayStr(t *testing.T) {
	t.Parallel()

	d := New()

	// Unknown hash
	unknown := hash16(0xFF, 0xFE, 0xFD, 0xFC)
	got := d.SimplestDisplayStr(unknown)
	want := "<" + hex.EncodeToString(unknown) + ">"
	if got != want {
		t.Errorf("SimplestDisplayStr unknown = %q, want %q", got, want)
	}

	// Trusted entry with name
	hash1 := hash16(0x01, 0x02, 0x03, 0x04)
	entry1, err := NewEntry(hash1)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash1, err)
	}
	entry1.DisplayName = "Alice"
	entry1.TrustLevel = TrustTrusted
	d.Remember(entry1)

	got = d.SimplestDisplayStr(hash1)
	if got != "Alice" {
		t.Errorf("SimplestDisplayStr trusted = %q, want %q", got, "Alice")
	}

	// Untrusted entry with name
	hash2 := hash16(0x05, 0x06, 0x07, 0x08)
	entry2, err := NewEntry(hash2)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash2, err)
	}
	entry2.DisplayName = "Bob"
	entry2.TrustLevel = TrustUntrusted
	d.Remember(entry2)

	got = d.SimplestDisplayStr(hash2)
	want = "Bob <" + hex.EncodeToString(hash2) + ">"
	if got != want {
		t.Errorf("SimplestDisplayStr untrusted = %q, want %q", got, want)
	}

	// Trusted entry without name
	hash3 := hash16(0x09, 0x0A, 0x0B, 0x0C)
	entry3, err := NewEntry(hash3)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash3, err)
	}
	entry3.TrustLevel = TrustTrusted
	entry3.DisplayName = ""
	d.Remember(entry3)

	got = d.SimplestDisplayStr(hash3)
	want = "<" + hex.EncodeToString(hash3) + ">"
	if got != want {
		t.Errorf("SimplestDisplayStr no name = %q, want %q", got, want)
	}
}

func TestAllegedDisplayStr(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash, err)
	}
	entry.DisplayName = "Bob"
	d.Remember(entry)

	got := d.AllegedDisplayStr(hash)
	if got != "Bob" {
		t.Errorf("AllegedDisplayStr = %q, want %q", got, "Bob")
	}

	unknown := hash16(0xFF, 0xFE, 0xFD, 0xFC)
	got = d.AllegedDisplayStr(unknown)
	if got != "" {
		t.Errorf("AllegedDisplayStr unknown = %q, want empty", got)
	}
}

func TestKnownNodes(t *testing.T) {
	t.Parallel()

	d := New()

	// Add non-node entry
	hash1 := hash16(0x01, 0x02, 0x03, 0x04)
	entry1, err := NewEntry(hash1)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash1, err)
	}
	entry1.DisplayName = "Peer"
	d.Remember(entry1)

	// Add node entry
	hash2 := hash16(0x05, 0x06, 0x07, 0x08)
	entry2, err := NewEntry(hash2)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash2, err)
	}
	entry2.DisplayName = "Node A"
	entry2.HostsNode = true
	entry2.TrustLevel = TrustTrusted
	d.Remember(entry2)

	// Add another node entry
	hash3 := hash16(0x09, 0x0A, 0x0B, 0x0C)
	entry3, err := NewEntry(hash3)
	if err != nil {
		t.Fatalf("NewEntry(%v): %v", hash3, err)
	}
	entry3.DisplayName = "Node B"
	entry3.HostsNode = true
	entry3.TrustLevel = TrustUntrusted
	d.Remember(entry3)

	nodes := d.KnownNodes()
	if len(nodes) != 2 {
		t.Fatalf("KnownNodes len = %v, want 2", len(nodes))
	}
	// Sorted by trust level descending: Trusted first
	if nodes[0].DisplayName != "Node A" {
		t.Errorf("KnownNodes[0] = %q, want %q", nodes[0].DisplayName, "Node A")
	}
	if nodes[1].DisplayName != "Node B" {
		t.Errorf("KnownNodes[1] = %q, want %q", nodes[1].DisplayName, "Node B")
	}
}

func TestAnnounceStream(t *testing.T) {
	t.Parallel()

	d := New()

	d.NodeAnnounceReceived(Announce{
		Timestamp:    1.0,
		SourceHash:   hash16(0x01),
		AnnounceType: "node",
	}, false)

	d.PeerAnnounceReceived(Announce{
		Timestamp:    2.0,
		SourceHash:   hash16(0x02),
		AnnounceType: "peer",
	}, false)

	d.PNAnnounceReceived(Announce{
		Timestamp:    3.0,
		SourceHash:   hash16(0x03),
		AnnounceType: "pn",
	}, false)

	stream := d.AnnounceStream()
	if len(stream) != 3 {
		t.Fatalf("AnnounceStream len = %v, want 3", len(stream))
	}
}

func TestAnnounceStreamCompact(t *testing.T) {
	t.Parallel()

	d := New()
	hash := hash16(0x01, 0x02, 0x03, 0x04)

	// Add two peer announces from same source
	d.PeerAnnounceReceived(Announce{
		Timestamp:  1.0,
		SourceHash: hash,
	}, false)
	d.PeerAnnounceReceived(Announce{
		Timestamp:  2.0,
		SourceHash: hash,
	}, true)

	stream := d.PeerAnnounces()
	if len(stream) != 1 {
		t.Fatalf("PeerAnnounces len = %v, want 1 (compact)", len(stream))
	}
	if stream[0].Timestamp != 2.0 {
		t.Errorf("Remaining announce timestamp = %f, want 2.0", stream[0].Timestamp)
	}
}

func TestAnnounceStreamMaxLen(t *testing.T) {
	t.Parallel()

	d := New()

	// Add more than max announces
	for i := range AnnounceStreamMaxLen + 10 {
		d.PeerAnnounceReceived(Announce{
			Timestamp:  float64(i),
			SourceHash: hash16(byte(i)),
		}, false)
	}

	stream := d.PeerAnnounces()
	if len(stream) != AnnounceStreamMaxLen {
		t.Errorf("PeerAnnounces len = %v, want %v", len(stream), AnnounceStreamMaxLen)
	}
}

func TestRemoveAnnounceWithTimestamp(t *testing.T) {
	t.Parallel()

	d := New()

	d.NodeAnnounceReceived(Announce{
		Timestamp:  1.0,
		SourceHash: hash16(0x01),
	}, false)
	d.NodeAnnounceReceived(Announce{
		Timestamp:  2.0,
		SourceHash: hash16(0x02),
	}, false)

	d.RemoveAnnounceWithTimestamp(1.0)

	stream := d.NodeAnnounces()
	if len(stream) != 1 {
		t.Fatalf("NodeAnnounces len = %v, want 1", len(stream))
	}
	if stream[0].Timestamp != 2.0 {
		t.Errorf("Remaining announce timestamp = %f, want 2.0", stream[0].Timestamp)
	}
}

func TestEntries(t *testing.T) {
	t.Parallel()

	d := New()

	for i := range 5 {
		hash := hash16(byte(i), 0x02, 0x03, 0x04)
		entry, err := NewEntry(hash)
		if err != nil {
			t.Fatalf("NewEntry(%v): %v", hash, err)
		}
		d.Remember(entry)
	}

	entries := d.Entries()
	if len(entries) != 5 {
		t.Errorf("Entries len = %v, want 5", len(entries))
	}
}

func TestLen(t *testing.T) {
	t.Parallel()

	d := New()
	if d.Len() != 0 {
		t.Errorf("Len = %v, want 0", d.Len())
	}

	hash := hash16(0x01, 0x02, 0x03, 0x04)
	entry, err := NewEntry(hash)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	d.Remember(entry)
	if d.Len() != 1 {
		t.Errorf("Len = %v, want 1", d.Len())
	}

	d.Forget(hash)
	if d.Len() != 0 {
		t.Errorf("Len after forget = %v, want 0", d.Len())
	}
}

// TestKnownNodesSortKey pins Python known_nodes' sort key (Directory.py:393):
// `sort_rank if not None else 2^32` — 2^32 is XOR, i.e. 34 — then
// TRUSTED-trust, then the display name with None sorting as "_".
func TestKnownNodesSortKey(t *testing.T) {
	t.Parallel()

	h := func(b byte) []byte { return bytes.Repeat([]byte{b}, 16) }
	rank := func(n int) *int { return &n }

	d := New()
	d.Remember(&Entry{SourceHash: h(1), DisplayName: "fifty", SortRank: rank(50), TrustLevel: TrustUnknown, HostsNode: true})
	d.Remember(&Entry{SourceHash: h(2), DisplayName: "no-rank", TrustLevel: TrustUnknown, HostsNode: true})
	d.Remember(&Entry{SourceHash: h(3), DisplayName: "five", SortRank: rank(5), TrustLevel: TrustUnknown, HostsNode: true})
	d.Remember(&Entry{SourceHash: h(4), DisplayName: "Zed", TrustLevel: TrustUnknown, HostsNode: true})
	d.Remember(&Entry{SourceHash: h(5), DisplayName: "", TrustLevel: TrustUnknown, HostsNode: true})

	var names []string
	for _, e := range d.KnownNodes() {
		names = append(names, e.DisplayName)
	}
	want := []string{"five", "Zed", "", "no-rank", "fifty"}
	if len(names) != len(want) {
		t.Fatalf("KnownNodes names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("KnownNodes names = %v, want %v (nil rank sorts at 34, not last)", names, want)
		}
	}
}

// TestDirectoryRejectsInvalidSourceHash pins Python DirectoryEntry's TypeError
// (Directory.py:419,433-434) at both ways a hash can enter the Go directory:
// the constructor, and the store (a Go struct literal bypasses the
// constructor, so Remember refuses it too).
func TestDirectoryRejectsInvalidSourceHash(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 1, 4, 8, 15, 17, 32} {
		if _, err := NewEntry(bytes.Repeat([]byte{1}, n)); !errors.Is(err, ErrInvalidSourceHash) {
			t.Errorf("NewEntry(%v bytes) err = %v, want ErrInvalidSourceHash", n, err)
		}
	}
	if _, err := NewEntry(bytes.Repeat([]byte{1}, SourceHashLength)); err != nil {
		t.Errorf("NewEntry(%v bytes) err = %v, want nil", SourceHashLength, err)
	}

	d := New()
	d.Remember(&Entry{SourceHash: []byte{1, 2, 3, 4}, DisplayName: "short"})
	if d.Find([]byte{1, 2, 3, 4}) != nil {
		t.Error("Remember stored an entry with a 4-byte source hash")
	}
	full := hash16(0x01)
	d.Remember(&Entry{SourceHash: full, DisplayName: "full"})
	if d.Find(full) == nil {
		t.Error("Remember refused a valid 16-byte entry")
	}
}

// TestLoadFromDiskRejectsInvalidSourceHash pins Python's load_from_disk
// behaviour (Directory.py:144,157-159): building a DirectoryEntry for a short
// hash raises, the exception aborts the whole load, and no entry is assigned.
func TestLoadFromDiskRejectsInvalidSourceHash(t *testing.T) {
	t.Parallel()

	path := filepath.Join(tempDir(t), "directory")
	bad := rnsmsgpack.OrderedMap{
		{Key: "entry_list", Value: []any{
			[]any{[]byte{1, 2, 3, 4}, "Short", uint64(TrustUnknown)},
		}},
		{Key: "announce_stream", Value: []any{}},
	}
	data, err := rnsmsgpack.Pack(bad)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := New()
	d.Remember(&Entry{SourceHash: hash16(0x09), DisplayName: "kept"})
	if err := d.LoadFromDisk(path); !errors.Is(err, ErrInvalidSourceHash) {
		t.Fatalf("LoadFromDisk err = %v, want ErrInvalidSourceHash", err)
	}
	if d.Find(hash16(0x09)) == nil {
		t.Error("a failed load must leave the existing directory untouched (Directory.py:157-159)")
	}
	if d.Find([]byte{1, 2, 3, 4}) != nil {
		t.Error("LoadFromDisk stored an entry with a 4-byte source hash")
	}
}
