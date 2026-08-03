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

import "testing"

func TestNodeAnnounceReceivedPeerCreatesTrustedEntry(t *testing.T) {
	t.Parallel()
	d := New()
	peerHash := []byte{0x11, 0x22, 0x33, 0x44}
	nodeHash := []byte{0x55, 0x66, 0x77, 0x88}
	d.Remember(&Entry{SourceHash: peerHash, DisplayName: "Trusted Peer", TrustLevel: TrustTrusted})

	d.NodeAnnounceReceivedPeer(Announce{
		Timestamp:    100,
		SourceHash:   nodeHash,
		AppData:      []byte("MyNode"),
		AnnounceType: "node",
	}, true, peerHash)

	e := d.Find(nodeHash)
	if e == nil {
		t.Fatal("trusted node entry should be created")
	}
	if e.DisplayName != "MyNode" {
		t.Errorf("DisplayName = %q, want MyNode", e.DisplayName)
	}
	if e.TrustLevel != TrustTrusted {
		t.Errorf("TrustLevel = %v, want trusted", e.TrustLevel)
	}
	if !e.HostsNode {
		t.Error("HostsNode should be true")
	}
}

func TestNodeAnnounceReceivedPeerUntrustedNoEntry(t *testing.T) {
	t.Parallel()
	d := New()
	peerHash := []byte{0x11, 0x22, 0x33, 0x44}
	nodeHash := []byte{0x55, 0x66, 0x77, 0x88}
	d.Remember(&Entry{SourceHash: peerHash, DisplayName: "Random", TrustLevel: TrustUnknown})

	d.NodeAnnounceReceivedPeer(Announce{
		Timestamp:    100,
		SourceHash:   nodeHash,
		AppData:      []byte("MyNode"),
		AnnounceType: "node",
	}, true, peerHash)

	if d.Find(nodeHash) != nil {
		t.Fatal("no entry should be created for untrusted peer")
	}
	// announce still recorded
	if len(d.NodeAnnounces()) != 1 {
		t.Fatalf("node announce should be recorded, got %v", len(d.NodeAnnounces()))
	}
}

func TestNodeAnnounceReceivedPeerExistingEntryNotOverwritten(t *testing.T) {
	t.Parallel()
	d := New()
	peerHash := []byte{0x11, 0x22, 0x33, 0x44}
	nodeHash := []byte{0x55, 0x66, 0x77, 0x88}
	d.Remember(&Entry{SourceHash: peerHash, TrustLevel: TrustTrusted})
	d.Remember(&Entry{SourceHash: nodeHash, DisplayName: "Existing", TrustLevel: TrustUntrusted})

	d.NodeAnnounceReceivedPeer(Announce{
		Timestamp:    100,
		SourceHash:   nodeHash,
		AppData:      []byte("NewName"),
		AnnounceType: "node",
	}, true, peerHash)

	e := d.Find(nodeHash)
	if e.DisplayName != "Existing" {
		t.Errorf("DisplayName = %q, want Existing (not overwritten)", e.DisplayName)
	}
}
