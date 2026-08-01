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