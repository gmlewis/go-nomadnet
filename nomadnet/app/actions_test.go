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
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
)

func mkUnreadConv(t *testing.T, base, hash string) {
	t.Helper()
	conv := filepath.Join(base, hash)
	if err := os.MkdirAll(conv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conv, "unread"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAppHasUnreadConversations(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if a.HasUnreadConversations() {
		t.Fatal("empty should not be unread")
	}
	mkUnreadConv(t, a.ConversationPath, "abcd")
	if !a.HasUnreadConversations() {
		t.Fatal("should detect unread")
	}
}

func TestAppConversationIsUnreadAndMarkRead(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	mkUnreadConv(t, a.ConversationPath, "beef")
	if !a.ConversationIsUnread("beef") {
		t.Fatal("beef should be unread")
	}
	a.MarkConversationRead("beef")
	if a.ConversationIsUnread("beef") {
		t.Fatal("beef should be read after mark")
	}
	if a.HasUnreadConversations() {
		t.Fatal("no unread after mark")
	}
}

func TestAppClearTmpDir(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if err := os.MkdirAll(a.TmpFilesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.TmpFilesPath, "a.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.TmpFilesPath, "b.bin"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.ClearTmpDir()
	entries, _ := os.ReadDir(a.TmpFilesPath)
	if len(entries) != 0 {
		t.Fatalf("tmp dir not empty: %d entries", len(entries))
	}
}

func TestAppShouldPrint(t *testing.T) {
	t.Parallel()
	mk := func() *App {
		a := NewApp(tempDir(t), "", false, false)
		a.setupPaths()
		a.Dir = directory.New()
		return a
	}
	trusted := []byte{1, 2, 3}
	untrusted := []byte{4, 5, 6}

	t.Run("disabled", func(t *testing.T) {
		a := mk()
		a.PrintMessages = false
		a.PrintAllMessages = true
		if a.ShouldPrint(&lxmf.Message{SourceHash: trusted}) {
			t.Fatal("disabled should not print")
		}
	})
	t.Run("all", func(t *testing.T) {
		a := mk()
		a.PrintMessages = true
		a.PrintAllMessages = true
		if !a.ShouldPrint(&lxmf.Message{SourceHash: untrusted}) {
			t.Fatal("print-all should print everything")
		}
	})
	t.Run("trusted_only", func(t *testing.T) {
		a := mk()
		a.PrintMessages = true
		a.PrintTrustedMessages = true
		a.Dir.Remember(&directory.Entry{SourceHash: trusted, TrustLevel: directory.TrustTrusted})
		if !a.ShouldPrint(&lxmf.Message{SourceHash: trusted}) {
			t.Fatal("trusted should print")
		}
		if a.ShouldPrint(&lxmf.Message{SourceHash: untrusted}) {
			t.Fatal("untrusted should not print")
		}
	})
	t.Run("allowed_destinations", func(t *testing.T) {
		a := mk()
		a.PrintMessages = true
		a.AllowedMessagePrintDestinations = []string{"040506"}
		if !a.ShouldPrint(&lxmf.Message{SourceHash: untrusted}) {
			t.Fatal("allowed destination should print")
		}
		if a.ShouldPrint(&lxmf.Message{SourceHash: trusted}) {
			t.Fatal("non-allowed should not print")
		}
	})
}

func TestAppDeleteConversation(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	conv := filepath.Join(a.ConversationPath, "cafe")
	if err := os.MkdirAll(conv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conv, "msg.lxm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.DeleteConversation("cafe")
	if _, err := os.Stat(conv); !os.IsNotExist(err) {
		t.Fatalf("conversation dir should be removed, got err=%v", err)
	}
}

func TestAppCreateDirectoryEntry(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	hash := []byte{0xaa, 0xbb}
	entry := a.CreateDirectoryEntry(hash, "Alice")
	if entry == nil {
		t.Fatal("entry should not be nil")
	}
	found := a.Dir.Find(hash)
	if found == nil {
		t.Fatal("entry not stored in directory")
	}
	if found.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want Alice", found.DisplayName)
	}
	if found.TrustLevel != directory.TrustUnknown {
		t.Errorf("TrustLevel = %v, want unknown", found.TrustLevel)
	}
}

func TestAppDisplayName(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	os.MkdirAll(a.StoragePath, 0o755)
	a.loadPeerSettings()
	if a.GetDisplayName() != "Anonymous Peer" {
		t.Fatalf("default name = %q, want Anonymous Peer", a.GetDisplayName())
	}
	a.SetDisplayName("Glenn")
	if a.GetDisplayName() != "Glenn" {
		t.Fatalf("name = %q, want Glenn", a.GetDisplayName())
	}
	wantBytes := []byte("Glenn")
	if got := a.GetDisplayNameBytes(); string(got) != string(wantBytes) {
		t.Fatalf("bytes = %q, want %q", got, wantBytes)
	}
	// persisted to disk
	a2 := NewApp(a.ConfigDir, "", false, false)
	a2.setupPaths()
	a2.loadPeerSettings()
	if a2.GetDisplayName() != "Glenn" {
		t.Fatalf("reloaded name = %q, want Glenn", a2.GetDisplayName())
	}
}
