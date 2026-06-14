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
	"testing"
)

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

	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	entry := NewEntry(hash)

	if len(entry.SourceHash) != 8 {
		t.Errorf("SourceHash len = %d, want 8", len(entry.SourceHash))
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
	hash := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	entry := NewEntry(hash)
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
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
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
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
	entry.DisplayName = "Alice"

	d.Remember(entry)

	name := d.DisplayName(hash)
	if name != "Alice" {
		t.Errorf("DisplayName = %q, want %q", name, "Alice")
	}

	// Unknown hash returns empty
	unknown := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	if name := d.DisplayName(unknown); name != "" {
		t.Errorf("DisplayName for unknown = %q, want empty", name)
	}
}

func TestTrustLevel(t *testing.T) {
	t.Parallel()

	d := New()
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
	entry.TrustLevel = TrustTrusted
	d.Remember(entry)

	// Direct lookup
	level := d.TrustLevel(hash, nil)
	if level != TrustTrusted {
		t.Errorf("TrustLevel = 0x%02X, want 0x%02X", level, TrustTrusted)
	}

	// Unknown hash returns UNKNOWN
	unknown := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	level = d.TrustLevel(unknown, nil)
	if level != TrustUnknown {
		t.Errorf("TrustLevel for unknown = 0x%02X, want 0x%02X", level, TrustUnknown)
	}
}

func TestTrustLevelNameCollision(t *testing.T) {
	t.Parallel()

	d := New()

	// Add two entries with different hashes but same display name
	hash1 := []byte{0x01, 0x02, 0x03, 0x04}
	entry1 := NewEntry(hash1)
	entry1.DisplayName = "SharedName"
	entry1.TrustLevel = TrustUntrusted
	d.Remember(entry1)

	hash2 := []byte{0x05, 0x06, 0x07, 0x08}
	entry2 := NewEntry(hash2)
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
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
	entry.PreferredDelivery = DeliveryPropagated
	d.Remember(entry)

	delivery := d.PreferredDelivery(hash)
	if delivery != DeliveryPropagated {
		t.Errorf("PreferredDelivery = 0x%02X, want 0x%02X", delivery, DeliveryPropagated)
	}

	// Unknown hash returns DIRECT
	unknown := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	delivery = d.PreferredDelivery(unknown)
	if delivery != DeliveryDirect {
		t.Errorf("PreferredDelivery for unknown = 0x%02X, want 0x%02X", delivery, DeliveryDirect)
	}
}

func TestShouldIdentifyOnConnect(t *testing.T) {
	t.Parallel()

	d := New()
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
	entry.IdentifyOnConnect = true
	d.Remember(entry)

	if !d.ShouldIdentifyOnConnect(hash) {
		t.Error("ShouldIdentifyOnConnect = false, want true")
	}

	unknown := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	if d.ShouldIdentifyOnConnect(unknown) {
		t.Error("ShouldIdentifyOnConnect for unknown = true, want false")
	}
}

func TestSetIdentifyOnConnect(t *testing.T) {
	t.Parallel()

	d := New()
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
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
	unknown := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	got := d.SimplestDisplayStr(unknown)
	want := "<fffefdfc>"
	if got != want {
		t.Errorf("SimplestDisplayStr unknown = %q, want %q", got, want)
	}

	// Trusted entry with name
	hash1 := []byte{0x01, 0x02, 0x03, 0x04}
	entry1 := NewEntry(hash1)
	entry1.DisplayName = "Alice"
	entry1.TrustLevel = TrustTrusted
	d.Remember(entry1)

	got = d.SimplestDisplayStr(hash1)
	if got != "Alice" {
		t.Errorf("SimplestDisplayStr trusted = %q, want %q", got, "Alice")
	}

	// Untrusted entry with name
	hash2 := []byte{0x05, 0x06, 0x07, 0x08}
	entry2 := NewEntry(hash2)
	entry2.DisplayName = "Bob"
	entry2.TrustLevel = TrustUntrusted
	d.Remember(entry2)

	got = d.SimplestDisplayStr(hash2)
	want = "Bob <05060708>"
	if got != want {
		t.Errorf("SimplestDisplayStr untrusted = %q, want %q", got, want)
	}

	// Trusted entry without name
	hash3 := []byte{0x09, 0x0A, 0x0B, 0x0C}
	entry3 := NewEntry(hash3)
	entry3.TrustLevel = TrustTrusted
	entry3.DisplayName = ""
	d.Remember(entry3)

	got = d.SimplestDisplayStr(hash3)
	want = "<090a0b0c>"
	if got != want {
		t.Errorf("SimplestDisplayStr no name = %q, want %q", got, want)
	}
}

func TestAllegedDisplayStr(t *testing.T) {
	t.Parallel()

	d := New()
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	entry := NewEntry(hash)
	entry.DisplayName = "Bob"
	d.Remember(entry)

	got := d.AllegedDisplayStr(hash)
	if got != "Bob" {
		t.Errorf("AllegedDisplayStr = %q, want %q", got, "Bob")
	}

	unknown := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	got = d.AllegedDisplayStr(unknown)
	if got != "" {
		t.Errorf("AllegedDisplayStr unknown = %q, want empty", got)
	}
}

func TestKnownNodes(t *testing.T) {
	t.Parallel()

	d := New()

	// Add non-node entry
	hash1 := []byte{0x01, 0x02, 0x03, 0x04}
	entry1 := NewEntry(hash1)
	entry1.DisplayName = "Peer"
	d.Remember(entry1)

	// Add node entry
	hash2 := []byte{0x05, 0x06, 0x07, 0x08}
	entry2 := NewEntry(hash2)
	entry2.DisplayName = "Node A"
	entry2.HostsNode = true
	entry2.TrustLevel = TrustTrusted
	d.Remember(entry2)

	// Add another node entry
	hash3 := []byte{0x09, 0x0A, 0x0B, 0x0C}
	entry3 := NewEntry(hash3)
	entry3.DisplayName = "Node B"
	entry3.HostsNode = true
	entry3.TrustLevel = TrustUntrusted
	d.Remember(entry3)

	nodes := d.KnownNodes()
	if len(nodes) != 2 {
		t.Fatalf("KnownNodes len = %d, want 2", len(nodes))
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
		SourceHash:   []byte{0x01},
		AnnounceType: "node",
	}, false)

	d.PeerAnnounceReceived(Announce{
		Timestamp:    2.0,
		SourceHash:   []byte{0x02},
		AnnounceType: "peer",
	}, false)

	d.PNAnnounceReceived(Announce{
		Timestamp:    3.0,
		SourceHash:   []byte{0x03},
		AnnounceType: "pn",
	}, false)

	stream := d.AnnounceStream()
	if len(stream) != 3 {
		t.Fatalf("AnnounceStream len = %d, want 3", len(stream))
	}
}

func TestAnnounceStreamCompact(t *testing.T) {
	t.Parallel()

	d := New()
	hash := []byte{0x01, 0x02, 0x03, 0x04}

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
		t.Fatalf("PeerAnnounces len = %d, want 1 (compact)", len(stream))
	}
	if stream[0].Timestamp != 2.0 {
		t.Errorf("Remaining announce timestamp = %f, want 2.0", stream[0].Timestamp)
	}
}

func TestAnnounceStreamMaxLen(t *testing.T) {
	t.Parallel()

	d := New()

	// Add more than max announces
	for i := 0; i < AnnounceStreamMaxLen+10; i++ {
		d.PeerAnnounceReceived(Announce{
			Timestamp:  float64(i),
			SourceHash: []byte{byte(i)},
		}, false)
	}

	stream := d.PeerAnnounces()
	if len(stream) != AnnounceStreamMaxLen {
		t.Errorf("PeerAnnounces len = %d, want %d", len(stream), AnnounceStreamMaxLen)
	}
}

func TestRemoveAnnounceWithTimestamp(t *testing.T) {
	t.Parallel()

	d := New()

	d.NodeAnnounceReceived(Announce{
		Timestamp:  1.0,
		SourceHash: []byte{0x01},
	}, false)
	d.NodeAnnounceReceived(Announce{
		Timestamp:  2.0,
		SourceHash: []byte{0x02},
	}, false)

	d.RemoveAnnounceWithTimestamp(1.0)

	stream := d.NodeAnnounces()
	if len(stream) != 1 {
		t.Fatalf("NodeAnnounces len = %d, want 1", len(stream))
	}
	if stream[0].Timestamp != 2.0 {
		t.Errorf("Remaining announce timestamp = %f, want 2.0", stream[0].Timestamp)
	}
}

func TestEntries(t *testing.T) {
	t.Parallel()

	d := New()

	for i := 0; i < 5; i++ {
		hash := []byte{byte(i), 0x02, 0x03, 0x04}
		entry := NewEntry(hash)
		d.Remember(entry)
	}

	entries := d.Entries()
	if len(entries) != 5 {
		t.Errorf("Entries len = %d, want 5", len(entries))
	}
}

func TestLen(t *testing.T) {
	t.Parallel()

	d := New()
	if d.Len() != 0 {
		t.Errorf("Len = %d, want 0", d.Len())
	}

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	d.Remember(NewEntry(hash))
	if d.Len() != 1 {
		t.Errorf("Len = %d, want 1", d.Len())
	}

	d.Forget(hash)
	if d.Len() != 0 {
		t.Errorf("Len after forget = %d, want 0", d.Len())
	}
}
