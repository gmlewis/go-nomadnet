// Copyright 2026 Glenn Lewis. All rights reserved.
//
// Use of this source code is governed by the Reticulum License
// that can be found in the LICENSE file.

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/msgpack"
)

// TestB2DisplayNameFallbackFromAnnounceAppData verifies B2: when the directory
// does not have a display name for a peer, ConversationList should fall back to
// the announce app data (LXMF display_name_from_app_data), matching nomadnet
// 1.2.8's _update_peer_info (Conversations.py:2092-2101) which tries:
// 1. directory.display_name → 2. recall_app_data + display_name_from_app_data → 3. hash
func TestB2DisplayNameFallbackFromAnnounceAppData(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	a.setupPaths()
	if err := os.MkdirAll(a.StoragePath, 0o755); err != nil {
		t.Fatal(err)
	}
	a.loadPeerSettings()
	a.Dir = directory.New()

	// Create a peer identity and register it in the transport with announce
	// app data containing a display name (matching LXMF's announce format:
	// [display_name_bytes, stamp_cost, [supported_functionality]]).
	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerDest, err := rns.NewDestination(ts, peerID, rns.DestinationOut, rns.DestinationSingle, lxmf.AppName, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	peerHash := peerDest.Hash

	// Build announce app data: [display_name_bytes, nil, [SFCompression]]
	peerData := []any{[]byte("AnnouncedName"), nil, []any{lxmf.SFCompression}}
	appData, err := msgpack.Pack(peerData)
	if err != nil {
		t.Fatal(err)
	}

	// Register the peer in the transport's known destinations with app data.
	ts.Remember(nil, peerHash, peerID.GetPublicKey(), appData)

	// Create a conversation directory for the peer (so it appears in the list).
	peerHashHex := peerDest.HexHash
	convDir := filepath.Join(a.ConversationPath, peerHashHex)
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Do NOT add a directory entry for the peer (so Dir.DisplayName returns "").

	list := a.ConversationList()
	if len(list) != 1 {
		t.Fatalf("ConversationList returned %v items, want 1", len(list))
	}

	// The display name should come from the announce app data, not the hash.
	if list[0].DisplayName != "AnnouncedName" {
		t.Errorf("B2: DisplayName = %q, want %q (from announce app data)", list[0].DisplayName, "AnnouncedName")
	}
}

// TestB2StoredEmptyNameFallsBackToAnnounce pins the Python display-name
// resolution for a NAMED-BEFORE peer whose stored directory name is empty
// (Python Conversation.conversation_list, Conversation.py:125 + 151-152):
// a directory entry EXISTING with an empty display name must still resolve to
// the announced app-data name for the conversation-list row, instead of
// rendering a bare symbol (or Python's post-load "Undefined"). Wire
// equivalent: the golden LXMF announce app-data
// msgpack [b"Go port of NomadNet on RaspPi", 32] resolves (via
// /opt/homebrew/bin/python3: LXMF.display_name_from_app_data) to
// "Go port of NomadNet on RaspPi".
func TestB2StoredEmptyNameFallsBackToAnnounce(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	a.setupPaths()
	if err := os.MkdirAll(a.StoragePath, 0o755); err != nil {
		t.Fatal(err)
	}
	a.loadPeerSettings()
	a.Dir = directory.New()

	peerID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerDest, err := rns.NewDestination(ts, peerID, rns.DestinationOut, rns.DestinationSingle, lxmf.AppName, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	peerHash := peerDest.Hash

	peerData := []any{[]byte("Go port of NomadNet on RaspPi"), nil, []any{lxmf.SFCompression}}
	appData, err := msgpack.Pack(peerData)
	if err != nil {
		t.Fatal(err)
	}
	ts.Remember(nil, peerHash, peerID.GetPublicKey(), appData)

	// A stored directory entry with an EMPTY display name (the wiped /
	// no-name state) must not defeat the announced-name fallback.
	a.Dir.Remember(directory.NewEntry(peerHash))

	convDir := filepath.Join(a.ConversationPath, peerDest.HexHash)
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	list := a.ConversationList()
	if len(list) != 1 {
		t.Fatalf("ConversationList returned %v items, want 1", len(list))
	}
	if list[0].DisplayName != "Go port of NomadNet on RaspPi" {
		t.Errorf("row with empty stored name = %q, want the announced name", list[0].DisplayName)
	}
}
