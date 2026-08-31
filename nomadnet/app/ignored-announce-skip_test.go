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
	"github.com/gmlewis/go-reticulum/rns"
)

// TestHandleNodeAnnounceSkipsBlockedAutoRemember verifies the Go-only
// announce-blocking enhancement end to end at the app layer: a node announce
// from an ignored (blocked) destination is never added to the directory's
// announce stream, and its auto-remember of a trusted-operator directory
// entry is skipped even when the associated peer is trusted. This guard has
// NO Python SOT counterpart (Python nomadnet cannot block nodes), so it must
// not be removed when auditing parity.
func TestHandleNodeAnnounceSkipsBlockedAutoRemember(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Dir = directory.New()

	nodeID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	nodeHash := rns.CalculateHash(nodeID, "nomadnetwork", "node")

	// Blocked: the stream rejects the announce and no directory entry lands.
	a.IgnoredList = append(a.IgnoredList, nodeHash)
	a.Dir.SetBlockedFilter(a.IsIgnored)
	a.handleNodeAnnounce(nodeHash, nodeID, []byte("BlockedNode"), false)

	if got := a.Dir.NodeAnnounces(); len(got) != 0 {
		t.Errorf("blocked node announce recorded: %v entries, want 0", len(got))
	}
	if a.Dir.Find(nodeHash) != nil {
		t.Error("blocked node was auto-remembered in the directory")
	}

	// An unblocked node announce still lands in the stream (control).
	otherID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherHash := rns.CalculateHash(otherID, "nomadnetwork", "node")
	a.handleNodeAnnounce(otherHash, otherID, []byte("OtherNode"), false)
	if got := len(a.Dir.AnnounceStream()); got != 1 {
		t.Fatalf("unblocked announce not recorded: %v entries, want 1", got)
	}
}
