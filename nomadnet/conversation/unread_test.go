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

package conversation

import (
	"os"
	"path/filepath"
	"testing"
)

func mkConv(t *testing.T, base, hash string, unread, failed bool) string {
	t.Helper()
	conv := filepath.Join(base, hash)
	if err := os.MkdirAll(conv, 0o755); err != nil {
		t.Fatal(err)
	}
	if unread {
		if err := os.WriteFile(filepath.Join(conv, "unread"), []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if failed {
		if err := os.WriteFile(filepath.Join(conv, "failed"), []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return conv
}

func TestHasUnreadConversations(t *testing.T) {
	t.Parallel()
	base := tempDir(t)
	if HasUnreadConversations(base) {
		t.Fatal("empty dir should have no unread")
	}
	mkConv(t, base, "aaaa", false, false)
	if HasUnreadConversations(base) {
		t.Fatal("clean conv should not be unread")
	}
	mkConv(t, base, "bbbb", true, false)
	if !HasUnreadConversations(base) {
		t.Fatal("unread conv should be detected")
	}
	mkConv(t, base, "cccc", false, true)
	if !HasUnreadConversations(base) {
		t.Fatal("failed conv should be detected")
	}
}

func TestConversationIsUnread(t *testing.T) {
	t.Parallel()
	base := tempDir(t)
	mkConv(t, base, "deadbeef", false, false)
	if ConversationIsUnread("deadbeef", base) {
		t.Fatal("clean conv should not be unread")
	}
	mkConv(t, base, "cafe", true, false)
	if !ConversationIsUnread("cafe", base) {
		t.Fatal("unread not detected")
	}
	mkConv(t, base, "face", false, true)
	if !ConversationIsUnread("face", base) {
		t.Fatal("failed not detected")
	}
	if ConversationIsUnread("nonexistent", base) {
		t.Fatal("nonexistent conv should not be unread")
	}
}

func TestMarkConversationRead(t *testing.T) {
	t.Parallel()
	base := tempDir(t)
	conv := mkConv(t, base, "abcd", true, true)
	MarkConversationRead("abcd", base)
	if fileExists(filepath.Join(conv, "unread")) {
		t.Fatal("unread file should be removed")
	}
	if fileExists(filepath.Join(conv, "failed")) {
		t.Fatal("failed file should be removed")
	}
	// idempotent: marking a clean conversation read is a no-op
	MarkConversationRead("abcd", base)
	// in-memory maps should be cleared
	if _, ok := unreadConversations["abcd"]; ok {
		t.Fatal("unreadConversations should not contain abcd")
	}
	if _, ok := failedConversations["abcd"]; ok {
		t.Fatal("failedConversations should not contain abcd")
	}
}
