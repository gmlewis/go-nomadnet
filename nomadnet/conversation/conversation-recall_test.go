// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by the Reticulum License
// that can be found in the LICENSE file.

package conversation

import (
	"encoding/hex"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// newRecallTestPeer builds a transport plus a remembered peer lxmf.delivery
// destination, mirroring announce receipt: the peer entry lands in the table
// with use-timestamp 0 (never used).
func newRecallTestPeer(t *testing.T) (*rns.TransportSystem, *rns.Destination) {
	t.Helper()
	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("NewIdentity(peer): %v", err)
	}
	ts := rns.NewTransportSystem(nil)
	dest, err := rns.NewDestination(ts, peerID, rns.DestinationOut, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatalf("NewDestination(peer): %v", err)
	}
	ts.Remember(nil, dest.Hash, peerID.GetPublicKey(), nil)
	return ts, dest
}

// TestRecallPeerMarksPeerUsed verifies that RecallPeer uses the default
// use-marking recall, mirroring Python Conversation.__init__
// (Conversation.py:204-208): a peer whose entry was never used must have its
// use-timestamp bumped above zero, which keeps the entry out of the
// transport's pathless/never-used cleanup (the protection whose absence let
// CleanKnownDestinations drop conversation peers and left "Unknown Origin"
// stamps after a node restart).
func TestRecallPeerMarksPeerUsed(t *testing.T) {
	ts, peer := newRecallTestPeer(t)

	use, ok := ts.KnownDestinationUseTimestamp(peer.Hash)
	if !ok {
		t.Fatal("peer entry missing before recall")
	}
	if use != 0 {
		t.Fatalf("peer use-timestamp = %v before recall, want 0 (never used)", use)
	}

	conv := NewConversation(hex.EncodeToString(peer.Hash), "")
	conv.SetTransport(ts)
	conv.RecallPeer()

	use, ok = ts.KnownDestinationUseTimestamp(peer.Hash)
	if !ok {
		t.Fatal("peer entry missing after recall")
	}
	if use <= 0 {
		t.Fatalf("peer use-timestamp = %v after recall, want > 0 (marked used)", use)
	}
}

// TestRecallPeerNoOpGuards verifies RecallPeer's nil-transport and malformed
// source-hash guards do not panic (Python's constructor recall only runs when
// the app has a transport; conversations without one must stay inert).
func TestRecallPeerNoOpGuards(t *testing.T) {
	conv := NewConversation("zzzz-not-hex", "")
	conv.RecallPeer() // no transport stamped: no-op

	ts, _ := newRecallTestPeer(t)

	bad := NewConversation("not-hex-at-all", "")
	bad.SetTransport(ts)
	bad.RecallPeer() // malformed hash: no-op, nothing recalled

	// Unknown peer: recall misses and the transport path request runs; the
	// call must neither panic nor error.
	unknownID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("NewIdentity(unknown): %v", err)
	}
	unknownDest, err := rns.NewDestination(ts, unknownID, rns.DestinationOut, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatalf("NewDestination(unknown): %v", err)
	}
	unknown := NewConversation(hex.EncodeToString(unknownDest.Hash), "")
	unknown.SetTransport(ts)
	unknown.RecallPeer()

	if id := ts.RecallNoUse(unknownDest.Hash); id != nil {
		t.Fatal("unknown peer recalled before any path/announce arrived")
	}
}
