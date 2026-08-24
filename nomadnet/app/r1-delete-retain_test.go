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
)

// TestR1DeleteConversationRetainsDirectoryEntry verifies R1: deleting a
// conversation (C-x) removes the conversation/messages but RETAINS the
// directory entry (name + trust), matching nomadnet 1.2.8. A subsequent
// incoming message from that peer re-creates the conversation under the
// retained entry, reusing the name and trust.
func TestR1DeleteConversationRetainsDirectoryEntry(t *testing.T) {
	t.Parallel()

	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Dir = directory.New()

	// Create a directory entry for a peer with a display name and trusted level.
	hash := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	a.CreateDirectoryEntry(hash, "TestPeer")
	a.SetPeerTrustLevel(hash, directory.TrustTrusted)

	// Verify the directory entry exists before delete.
	entry := a.Dir.Find(hash)
	if entry == nil {
		t.Fatal("directory entry should exist before delete")
	}
	if entry.DisplayName != "TestPeer" {
		t.Fatalf("DisplayName = %q, want TestPeer", entry.DisplayName)
	}
	if entry.TrustLevel != directory.TrustTrusted {
		t.Fatalf("TrustLevel = %v, want trusted", entry.TrustLevel)
	}

	// Create a conversation directory for the peer.
	hashHex := "aabbccdd"
	convDir := filepath.Join(a.ConversationPath, hashHex)
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convDir, "msg.lxm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Delete the conversation.
	a.DeleteConversation(hashHex)

	// Verify the conversation directory is removed.
	if _, err := os.Stat(convDir); !os.IsNotExist(err) {
		t.Fatalf("conversation dir should be removed, got err=%v", err)
	}

	// R1: verify the directory entry is RETAINED (name + trust preserved).
	entry = a.Dir.Find(hash)
	if entry == nil {
		t.Fatal("R1: directory entry should be retained after delete")
	}
	if entry.DisplayName != "TestPeer" {
		t.Errorf("R1: DisplayName = %q, want TestPeer (retained)", entry.DisplayName)
	}
	if entry.TrustLevel != directory.TrustTrusted {
		t.Errorf("R1: TrustLevel = %v, want trusted (retained)", entry.TrustLevel)
	}
}
