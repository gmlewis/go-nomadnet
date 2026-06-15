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
	"testing"
	"time"
)

func TestConversationsSortMode(t *testing.T) {
	t.Parallel()

	now := time.Now()
	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Charlie", LastTime: now.Add(-1 * time.Hour)},
		{SourceHash: "bbb", DisplayName: "Alice", LastTime: now.Add(-3 * time.Hour)},
		{SourceHash: "ccc", DisplayName: "Bob", LastTime: now},
	}

	SortConversations(convs, SortRecent)
	got0 := convs[0].DisplayName
	if got0 != "Bob" {
		t.Errorf("SortRecent: first item = %q, want %q", got0, "Bob")
	}

	SortConversations(convs, SortName)
	got0 = convs[0].DisplayName
	if got0 != "Alice" {
		t.Errorf("SortName: first item = %q, want %q", got0, "Alice")
	}
}

func TestConversationsSortPinnedToTop(t *testing.T) {
	t.Parallel()

	now := time.Now()
	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Charlie", LastTime: now.Add(-1 * time.Hour), Pinned: false},
		{SourceHash: "bbb", DisplayName: "Alice", LastTime: now.Add(-3 * time.Hour), Pinned: true},
		{SourceHash: "ccc", DisplayName: "Bob", LastTime: now, Pinned: false},
	}

	SortConversations(convs, SortRecent)

	// Alice is pinned so she should be first
	if convs[0].DisplayName != "Alice" {
		t.Errorf("pinned first: got %q, want %q", convs[0].DisplayName, "Alice")
	}
	// Bob is most recent non-pinned, should be second
	if convs[1].DisplayName != "Bob" {
		t.Errorf("second: got %q, want %q", convs[1].DisplayName, "Bob")
	}
	// Charlie is oldest non-pinned, should be last
	if convs[2].DisplayName != "Charlie" {
		t.Errorf("third: got %q, want %q", convs[2].DisplayName, "Charlie")
	}
}

func TestConversationsSortPinnedToTopByName(t *testing.T) {
	t.Parallel()

	now := time.Now()
	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Charlie", LastTime: now, Pinned: true},
		{SourceHash: "bbb", DisplayName: "Alice", LastTime: now, Pinned: false},
		{SourceHash: "ccc", DisplayName: "Bob", LastTime: now, Pinned: false},
	}

	SortConversations(convs, SortName)

	// Charlie is pinned, should be first
	if convs[0].DisplayName != "Charlie" {
		t.Errorf("pinned first: got %q, want %q", convs[0].DisplayName, "Charlie")
	}
	// Among non-pinned: Alice < Bob
	if convs[1].DisplayName != "Alice" {
		t.Errorf("second: got %q, want %q", convs[1].DisplayName, "Alice")
	}
	if convs[2].DisplayName != "Bob" {
		t.Errorf("third: got %q, want %q", convs[2].DisplayName, "Bob")
	}
}

func TestConversationsSortEmpty(t *testing.T) {
	t.Parallel()

	var convs []ConversationInfo
	SortConversations(convs, SortRecent)
	SortConversations(convs, SortName)
	// Should not panic
}

func TestConversationsSortStable(t *testing.T) {
	t.Parallel()

	now := time.Now()
	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Alice", LastTime: now},
		{SourceHash: "bbb", DisplayName: "Alice", LastTime: now.Add(-time.Hour)},
	}

	SortConversations(convs, SortRecent)

	// Both named Alice, sort by recent. First should have more recent time.
	if convs[0].SourceHash != "aaa" {
		t.Errorf("stable sort: got %q, want %q", convs[0].SourceHash, "aaa")
	}
}

func TestSortModeToggle(t *testing.T) {
	t.Parallel()

	mode := SortRecent
	if mode != SortRecent {
		t.Error("initial mode should be SortRecent")
	}

	mode = ToggleSortMode(mode)
	if mode != SortName {
		t.Errorf("after toggle: got %d, want SortName(%d)", mode, SortName)
	}

	mode = ToggleSortMode(mode)
	if mode != SortRecent {
		t.Errorf("after second toggle: got %d, want SortRecent(%d)", mode, SortRecent)
	}
}

func TestConversationsSortMultiplePinned(t *testing.T) {
	t.Parallel()

	now := time.Now()
	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Charlie", LastTime: now.Add(-1 * time.Hour), Pinned: true},
		{SourceHash: "bbb", DisplayName: "Alice", LastTime: now.Add(-3 * time.Hour), Pinned: true},
		{SourceHash: "ccc", DisplayName: "Bob", LastTime: now, Pinned: false},
	}

	SortConversations(convs, SortName)

	// Both pinned should come first, sorted by name among themselves
	if convs[0].DisplayName != "Alice" {
		t.Errorf("first pinned: got %q, want %q", convs[0].DisplayName, "Alice")
	}
	if convs[1].DisplayName != "Charlie" {
		t.Errorf("second pinned: got %q, want %q", convs[1].DisplayName, "Charlie")
	}
	if convs[2].DisplayName != "Bob" {
		t.Errorf("non-pinned: got %q, want %q", convs[2].DisplayName, "Bob")
	}
}

func TestFilterConversationsTrusted(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Alice", TrustLevel: "trusted"},
		{SourceHash: "bbb", DisplayName: "Bob", TrustLevel: "untrusted"},
		{SourceHash: "ccc", DisplayName: "Charlie", TrustLevel: "trusted"},
	}

	filtered := FilterConversations(convs, "trusted")
	if len(filtered) != 2 {
		t.Fatalf("trusted filter: got %d, want 2", len(filtered))
	}
	if filtered[0].DisplayName != "Alice" {
		t.Errorf("first: got %q, want %q", filtered[0].DisplayName, "Alice")
	}
	if filtered[1].DisplayName != "Charlie" {
		t.Errorf("second: got %q, want %q", filtered[1].DisplayName, "Charlie")
	}
}

func TestFilterConversationsUntrusted(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Alice", TrustLevel: "trusted"},
		{SourceHash: "bbb", DisplayName: "Bob", TrustLevel: "untrusted"},
	}

	filtered := FilterConversations(convs, "untrusted")
	if len(filtered) != 1 {
		t.Fatalf("untrusted filter: got %d, want 1", len(filtered))
	}
	if filtered[0].DisplayName != "Bob" {
		t.Errorf("first: got %q, want %q", filtered[0].DisplayName, "Bob")
	}
}

func TestFilterConversationsBlocked(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "aaa", DisplayName: "Alice", TrustLevel: "trusted"},
		{SourceHash: "bbb", DisplayName: "Bob", TrustLevel: "untrusted"},
		{SourceHash: "ccc", DisplayName: "Eve", TrustLevel: "blocked"},
	}

	// Untrusted tab without show blocked
	filtered := FilterConversations(convs, "untrusted")
	if len(filtered) != 1 {
		t.Fatalf("untrusted no-block: got %d, want 1", len(filtered))
	}

	// Untrusted tab with show blocked
	filtered = FilterConversationsWithBlocked(convs, "untrusted", true)
	if len(filtered) != 2 {
		t.Fatalf("untrusted with blocked: got %d, want 2", len(filtered))
	}

	// Trusted tab should never show blocked
	filtered = FilterConversationsWithBlocked(convs, "trusted", true)
	if len(filtered) != 1 {
		t.Fatalf("trusted with blocked flag: got %d, want 1", len(filtered))
	}
}

