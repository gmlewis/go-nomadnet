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
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
)

// mkMessageFiles seeds count 64-char-hex-named message files in a conversation
// dir (the on-disk layout ScanStorage discovers).
func mkMessageFiles(t *testing.T, convDir string, count int) {
	t.Helper()
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range count {
		hash := make([]byte, 32)
		hash[0] = byte(i + 1)
		name := hex.EncodeToString(hash)
		if err := os.WriteFile(filepath.Join(convDir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAppPurgeFailedMessages pins Python ConversationWidget.keypress
// "ctrl u" → Conversation.purge_failed (Conversations.py:2227-2228 +
// Conversation.py:274-283) at the app layer: only FAILED messages are purged
// from disk and the in-memory list, non-failed messages survive, and a
// conversation that has never been opened is a no-op.
func TestAppPurgeFailedMessages(t *testing.T) {
	t.Parallel()

	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	convDir := filepath.Join(a.ConversationPath, "cafe")
	mkMessageFiles(t, convDir, 3)

	conv := a.conversationFor("cafe")
	if conv == nil {
		t.Fatal("conversationFor returned nil for an existing conversation dir")
	}
	failedState := conversation.StateFailed
	conv.Messages[0].CachedState = &failedState
	conv.Messages[0].Loaded = true

	a.PurgeFailedMessages("cafe")

	if got := len(conv.Messages); got != 2 {
		t.Errorf("after PurgeFailedMessages, messages = %v, want 2", got)
	}
	entries, _ := os.ReadDir(convDir)
	msgFiles := 0
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) == 64 {
			msgFiles++
		}
	}
	if msgFiles != 2 {
		t.Errorf("after PurgeFailedMessages, on-disk message files = %v, want 2", msgFiles)
	}

	// A conversation with no directory on disk is a no-op (no panic, no dir
	// created).
	a.PurgeFailedMessages("beef")
	if _, err := os.Stat(filepath.Join(a.ConversationPath, "beef")); !os.IsNotExist(err) {
		t.Error("purging a nonexistent conversation should not create its directory")
	}
}

// TestAppClearConversationHistory pins Python clear_history_dialog's
// confirmed() → Conversation.clear_history (Conversations.py:2129 +
// Conversation.py:284-292) at the app layer: every message file is purged and
// the in-memory message list empties.
func TestAppClearConversationHistory(t *testing.T) {
	t.Parallel()

	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	convDir := filepath.Join(a.ConversationPath, "cafe")
	mkMessageFiles(t, convDir, 3)

	a.ClearConversationHistory("cafe")

	conv := a.conversationFor("cafe")
	if conv == nil {
		t.Fatal("conversationFor returned nil after ClearConversationHistory")
	}
	if got := len(conv.Messages); got != 0 {
		t.Errorf("after ClearConversationHistory, messages = %v, want 0", got)
	}
	entries, _ := os.ReadDir(convDir)
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) == 64 {
			t.Errorf("message file %v survived ClearConversationHistory", e.Name())
		}
	}
}
