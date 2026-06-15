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

package tui

import (
	"strings"
	"testing"
	"time"
)

func TestConversationStoreCreate(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	if store.Count() != 0 {
		t.Errorf("empty store count = %d, want 0", store.Count())
	}
}

func TestConversationStoreAdd(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	conv := ConversationInfo{
		SourceHash:  "aabbccdd",
		DisplayName: "Alice",
		TrustLevel:  "trusted",
		LastMessage: "Hello!",
		LastTime:    time.Now(),
		Unread:      true,
	}
	store.Add(conv)

	if store.Count() != 1 {
		t.Errorf("count = %d, want 1", store.Count())
	}

	got := store.Get("aabbccdd")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Alice")
	}
}

func TestConversationStoreGetMissing(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	if store.Get("nonexistent") != nil {
		t.Error("Get on missing should return nil")
	}
}

func TestConversationStoreDelete(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A"})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B"})

	store.Delete("aabb")
	if store.Count() != 1 {
		t.Errorf("count after delete = %d, want 1", store.Count())
	}
	if store.Get("aabb") != nil {
		t.Error("deleted entry still found")
	}
	if store.Get("ccdd") == nil {
		t.Error("non-deleted entry missing")
	}
}

func TestConversationStoreDeleteMissing(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Delete("nonexistent") // should not panic
}

func TestConversationStoreSearch(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "Alice Chat", TrustLevel: "trusted"})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "Bob Room", TrustLevel: "untrusted"})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "Charlie", TrustLevel: "trusted"})

	tests := []struct {
		query string
		count int
	}{
		{"", 3},
		{"alice", 1},
		{"ALICE", 1},
		{"chat", 1},
		{"bob", 1},
		{"zzz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()
			got := store.Search(tt.query)
			if len(got) != tt.count {
				t.Errorf("Search(%q) returned %d, want %d", tt.query, len(got), tt.count)
			}
		})
	}
}

func TestConversationStoreFilterByTrust(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A", TrustLevel: "trusted"})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B", TrustLevel: "untrusted"})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "C", TrustLevel: "trusted"})

	trusted := store.FilterByTrust("trusted")
	if len(trusted) != 2 {
		t.Errorf("trusted count = %d, want 2", len(trusted))
	}

	untrusted := store.FilterByTrust("untrusted")
	if len(untrusted) != 1 {
		t.Errorf("untrusted count = %d, want 1", len(untrusted))
	}
}

func TestConversationStoreSortByName(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "Charlie", LastTime: time.Now()})
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "Alice", LastTime: time.Now().Add(-time.Hour)})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "Bob", LastTime: time.Now().Add(-2 * time.Hour)})

	sorted := store.SortedByName()
	if len(sorted) != 3 {
		t.Fatalf("sorted count = %d, want 3", len(sorted))
	}
	if sorted[0].DisplayName != "Alice" {
		t.Errorf("first = %q, want Alice", sorted[0].DisplayName)
	}
	if sorted[1].DisplayName != "Bob" {
		t.Errorf("second = %q, want Bob", sorted[1].DisplayName)
	}
	if sorted[2].DisplayName != "Charlie" {
		t.Errorf("third = %q, want Charlie", sorted[2].DisplayName)
	}
}

func TestConversationStoreSortByTime(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	now := time.Now()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A", LastTime: now.Add(-2 * time.Hour)})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B", LastTime: now})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "C", LastTime: now.Add(-time.Hour)})

	sorted := store.SortedByTime()
	if len(sorted) != 3 {
		t.Fatalf("sorted count = %d, want 3", len(sorted))
	}
	if sorted[0].DisplayName != "B" {
		t.Errorf("first = %q, want B (most recent)", sorted[0].DisplayName)
	}
	if sorted[2].DisplayName != "A" {
		t.Errorf("third = %q, want A (oldest)", sorted[2].DisplayName)
	}
}

func TestConversationStoreUpdate(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "Alice", Unread: false})

	entry := store.Get("aabb")
	entry.Unread = true
	entry.LastMessage = "New message"

	got := store.Get("aabb")
	if !got.Unread {
		t.Error("Unread should be true after update")
	}
	if got.LastMessage != "New message" {
		t.Errorf("LastMessage = %q, want %q", got.LastMessage, "New message")
	}
}

func TestConversationStoreMarkRead(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A", Unread: true})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B", Unread: false})

	store.MarkRead("aabb")
	if store.Get("aabb").Unread {
		t.Error("should be marked as read")
	}
	if store.Get("ccdd").Unread {
		t.Error("other conv should not change")
	}
}

func TestConversationStoreMarkFailed(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A"})

	store.MarkFailed("aabb")
	if !store.Get("aabb").Failed {
		t.Error("should be marked as failed")
	}
}

func TestConversationStoreMarkAllRead(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A", Unread: true})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B", Unread: true})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "C", Unread: false})

	store.MarkAllRead()

	for _, conv := range store.List() {
		if conv.Unread {
			t.Errorf("%s should be marked as read", conv.DisplayName)
		}
	}
}

func TestConversationStoreUnreadCount(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A", Unread: true})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B", Unread: true})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "C", Unread: false})

	if got := store.UnreadCount(); got != 2 {
		t.Errorf("UnreadCount() = %d, want 2", got)
	}
}

func TestConversationStoreByTrustLevel(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	store.Add(ConversationInfo{SourceHash: "aabb", DisplayName: "A", TrustLevel: "trusted"})
	store.Add(ConversationInfo{SourceHash: "ccdd", DisplayName: "B", TrustLevel: "untrusted"})
	store.Add(ConversationInfo{SourceHash: "eeff", DisplayName: "C", TrustLevel: "trusted"})
	store.Add(ConversationInfo{SourceHash: "1122", DisplayName: "D", TrustLevel: "warning"})

	trusted := store.ByTrustLevel("trusted")
	if len(trusted) != 2 {
		t.Errorf("trusted count = %d, want 2", len(trusted))
	}

	all := store.ByTrustLevel("")
	if len(all) != 4 {
		t.Errorf("empty filter count = %d, want 4", len(all))
	}
}

func TestConversationStoreLastSyncStatus(t *testing.T) {
	t.Parallel()

	store := NewConversationStore()
	if got := store.LastSyncStatus(); got != "Last sync: never" {
		t.Errorf("no sync = %q, want %q", got, "Last sync: never")
	}

	store.SetLastSyncTime(time.Now().Add(-5 * time.Minute))
	got := store.LastSyncStatus()
	if !strings.Contains(got, "5m ago") {
		t.Errorf("after sync = %q, want to contain '5m ago'", got)
	}
	if !strings.HasPrefix(got, "Last sync:") {
		t.Errorf("after sync = %q, want prefix 'Last sync:'", got)
	}

	store.SetSyncNodeLabel("TestNode")
	got = store.LastSyncStatus()
	if !strings.Contains(got, "TestNode") {
		t.Errorf("with node = %q, want to contain 'TestNode'", got)
	}
}
