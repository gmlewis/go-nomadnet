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
	"sync"
	"testing"
)

// blockedTestHashes returns two distinct 16-byte destination-hash fixtures.
func blockedTestHashes() (blocked, other []byte) {
	return bytes.Repeat([]byte{0xab}, 16), bytes.Repeat([]byte{0xcd}, 16)
}

// announceFixture builds a stream announce for the given source hash.
func announceFixture(sourceHash []byte, announceType string) Announce {
	return Announce{
		Timestamp:    1700000000.5,
		SourceHash:   append([]byte(nil), sourceHash...),
		AppData:      []byte("app data"),
		AnnounceType: announceType,
	}
}

// TestBlockedFilterRejectsNewAnnounces pins the Go-only blocked announce filter
// (SetBlockedFilter): announces from a blocked source are never recorded, on
// any stream type, and associated-peer remembering is likewise suppressed.
// There is no Python SOT counterpart for this filter — see the SetBlockedFilter
// doc comment for the full divergence note.
func TestBlockedFilterRejectsNewAnnounces(t *testing.T) {
	t.Parallel()
	blocked, other := blockedTestHashes()
	d := New()
	once := sync.Once{}
	d.SetBlockedFilter(func(h []byte) bool {
		once.Do(func() {}) // capture nothing; predicate below
		return bytes.Equal(h, blocked)
	})

	d.NodeAnnounceReceived(announceFixture(blocked, "node"), true)
	d.NodeAnnounceReceived(announceFixture(other, "node"), true)
	d.PeerAnnounceReceived(announceFixture(blocked, "peer"), true)
	d.PNAnnounceReceived(announceFixture(blocked, "pn"), true)

	if got := d.NodeAnnounces(); len(got) != 1 {
		t.Fatalf("NodeAnnounces len = %v, want 1 (only the unblocked announce)", len(got))
	}
	if !bytes.Equal(d.NodeAnnounces()[0].SourceHash, other) {
		t.Errorf("node stream contains blocked source, want only the unblocked one")
	}
	if got := d.AnnounceStream(); len(got) != 1 {
		t.Fatalf("AnnounceStream len = %v, want 1 (blocked announce dropped)", len(got))
	}
	if got := len(d.PeerAnnounces()); got != 0 {
		t.Fatalf("peer stream has %v announces, want 0 (blocked source)", got)
	}
	if got := len(d.PNAnnounces()); got != 0 {
		t.Fatalf("pn stream has %v announces, want 0", got)
	}
}

// TestBlockedFilterHidesExistingAnnounces verifies that setting the filter also
// hides announces recorded BEFORE the filter was installed (so blocking a node
// whose announces the session already received clears it from every stream
// getter immediately), and that clearing the filter reveals them again.
func TestBlockedFilterHidesExistingAnnounces(t *testing.T) {
	t.Parallel()
	blocked, _ := blockedTestHashes()

	d := New()
	d.NodeAnnounceReceived(announceFixture(blocked, "node"), true)
	d.PeerAnnounceReceived(announceFixture(blocked, "peer"), true)
	if got := len(d.AnnounceStream()); got != 2 {
		t.Fatalf("precondition: stream should hold both announces, got %v", got)
	}

	d.SetBlockedFilter(func(h []byte) bool { return bytes.Equal(h, blocked) })
	if got := len(d.AnnounceStream()); got != 0 {
		t.Fatalf("AnnounceStream held %v entries after SetBlockedFilter, want 0", got)
	}
	if got := len(d.NodeAnnounces()); got != 0 {
		t.Fatalf("node stream held %v entries, want 0", got)
	}
	if got := len(d.PeerAnnounces()); got != 0 {
		t.Fatalf("peer stream held %v entries, want 0", got)
	}

	d.SetBlockedFilter(nil)
	if got := len(d.AnnounceStream()); got != 2 {
		t.Fatalf("after clearing filter stream = %v entries, want 2", got)
	}
}

// TestBlockedFilterNilIsPassthrough verifies a nil filter (the default)
// records and returns everything, and that nil-ing the filter after use
// restores the passthrough.
func TestBlockedFilterNilIsPassthrough(t *testing.T) {
	t.Parallel()
	blocked, _ := blockedTestHashes()
	d := New()
	d.NodeAnnounceReceived(announceFixture(blocked, "node"), true)
	if got := len(d.NodeAnnounces()); got != 1 {
		t.Fatalf("with no filter set, node announces = %v, want 1", got)
	}

	d.SetBlockedFilter(func([]byte) bool { return true }) // block everything
	d.NodeAnnounceReceived(announceFixture([]byte{1, 2}, "node"), true)
	if got := len(d.AnnounceStream()); got != 0 {
		t.Fatalf("announces visible through filter = %v, want 0", got)
	}
	if got := len(d.NodeAnnounces()); got != 0 {
		t.Fatalf("node stream with filter = %v, want 0 (pre-existing entry hidden)", got)
	}
	d.SetBlockedFilter(nil)
	if got := len(d.AnnounceStream()); got != 1 {
		t.Fatalf("after clearing filter stream = %v, want 1", got)
	}
}

// TestBlockedFilterSkipsPeerRemember verifies NodeAnnounceReceivedPeer does
// not remember a directory entry for a blocked node even when the associated
// peer is trusted.
func TestBlockedFilterSkipsAssociatedPeerRemember(t *testing.T) {
	t.Parallel()
	blocked, _ := blockedTestHashes()
	associated := bytes.Repeat([]byte{0x99}, 16)
	peerEntry := &Entry{SourceHash: associated, TrustLevel: TrustTrusted}
	d := New()
	d.blockedFilter = func(h []byte) bool { return bytes.Equal(h, blocked) } // direct, pre-SetBlockedFilter
	d.Remember(peerEntry)

	d.NodeAnnounceReceivedPeer(announceFixture(blocked, "node"), true, associated)

	if d.Find(blocked) != nil {
		t.Error("blocked node was remembered in the directory")
	}
	if got := len(d.NodeAnnounces()); got != 0 {
		t.Errorf("node stream held %v entries, want 0", got)
	}
}
