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
	"bytes"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestAppBlockPeer pins BlockPeer as the Peer Info dialog's Block action:
// it blocks the destination through the same path as BlockDestination
// (blackhole + ignored list + router ignore).
func TestAppBlockPeer(t *testing.T) {
	t.Parallel()

	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := bytes.Repeat([]byte{0x42}, 16)

	if a.IsIgnored(hash) {
		t.Fatal("peer ignored before BlockPeer")
	}
	if !a.BlockPeer(hash, "spam") {
		t.Fatal("BlockPeer = false, want true")
	}
	if !a.IsIgnored(hash) {
		t.Error("BlockPeer did not add the peer to the ignored list")
	}
	if a.BlockPeer(nil, "spam") {
		t.Error("BlockPeer(nil) = true, want false")
	}
}

// TestAppQueryForPeer pins QueryForPeer: it asks the transport for the peer's
// path (Python Conversation.query_for_peer → RNS.Transport.request_path), and
// is a no-op with no transport or hash.
func TestAppQueryForPeer(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	a := NewAppWithTransport(tempDir(t), WithTransport(ts))
	a.setupPaths()
	hash := bytes.Repeat([]byte{0x55}, 16)

	if ts.PathRequestPending(hash) {
		t.Fatal("path request already pending before QueryForPeer")
	}
	a.QueryForPeer(hash)
	if !ts.PathRequestPending(hash) {
		t.Error("QueryForPeer did not ask the transport for the path")
	}

	// Guards: nil hash and no transport are silent.
	a.QueryForPeer(nil)
	empty := NewApp(tempDir(t), "", false, false)
	empty.setupPaths()
	empty.QueryForPeer(hash)
}

// TestAppNewSendDeps pins NewSendDeps' basic shape: the adapter targets the
// requested source hash and reports no destination when the identity is not
// known to the transport (Python send_destination=None).
func TestAppNewSendDeps(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	a := NewAppWithTransport(tempDir(t), WithTransport(ts))
	a.setupPaths()

	const sourceHash = "55555555555555555555555555555555"
	deps := a.NewSendDeps(sourceHash)
	if deps == nil {
		t.Fatal("NewSendDeps = nil")
	}
	if got := deps.SendDestination(); got != nil {
		t.Errorf("SendDestination = %v, want nil for an unknown identity", got)
	}
	if got := deps.DownloadsPath(); got != a.DownloadsPath {
		t.Errorf("DownloadsPath = %q, want %q", got, a.DownloadsPath)
	}
	if got := deps.TmpFilesPath(); got != a.TmpFilesPath {
		t.Errorf("TmpFilesPath = %q, want %q", got, a.TmpFilesPath)
	}
}
