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
	"sort"
	"strings"
	"sync"
	"time"
)

// ConversationStore manages the collection of conversations.
// Thread-safe for concurrent access. Supports CRUD, search, and filtering.
type ConversationStore struct {
	mu    sync.RWMutex
	convs map[string]*ConversationInfo
	order []string // insertion order for stable listing

	lastSyncTime time.Time
	lastSyncNode string
}

// NewConversationStore creates an empty conversation store.
func NewConversationStore() *ConversationStore {
	return &ConversationStore{convs: make(map[string]*ConversationInfo)}
}

// Add inserts or updates a conversation entry.
func (cs *ConversationStore) Add(conv ConversationInfo) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	entry := conv
	cs.convs[conv.SourceHash] = &entry

	// Track insertion order if new
	found := false
	for _, h := range cs.order {
		if h == conv.SourceHash {
			found = true
			break
		}
	}
	if !found {
		cs.order = append(cs.order, conv.SourceHash)
	}
}

// Get returns the conversation for the given source hash, or nil.
func (cs *ConversationStore) Get(sourceHash string) *ConversationInfo {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.convs[sourceHash]
}

// Delete removes a conversation. No-op if not found.
func (cs *ConversationStore) Delete(sourceHash string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	delete(cs.convs, sourceHash)
	for i, h := range cs.order {
		if h == sourceHash {
			cs.order = append(cs.order[:i], cs.order[i+1:]...)
			break
		}
	}
}

// Count returns the number of stored conversations.
func (cs *ConversationStore) Count() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.convs)
}

// List returns all conversations in insertion order.
func (cs *ConversationStore) List() []ConversationInfo {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	out := make([]ConversationInfo, 0, len(cs.order))
	for _, h := range cs.order {
		if c, ok := cs.convs[h]; ok {
			out = append(out, *c)
		}
	}
	return out
}

// Search returns conversations matching a case-insensitive substring
// query on display name or source hash. Empty query returns all.
func (cs *ConversationStore) Search(query string) []ConversationInfo {
	all := cs.List()
	if query == "" {
		return all
	}
	q := strings.ToLower(query)
	var result []ConversationInfo
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.DisplayName), q) ||
			strings.Contains(strings.ToLower(c.SourceHash), q) {
			result = append(result, c)
		}
	}
	return result
}

// FilterByTrust returns conversations matching the given trust level.
// Empty trustLevel returns all.
func (cs *ConversationStore) FilterByTrust(trustLevel string) []ConversationInfo {
	all := cs.List()
	if trustLevel == "" {
		return all
	}
	var result []ConversationInfo
	for _, c := range all {
		if c.TrustLevel == trustLevel {
			result = append(result, c)
		}
	}
	return result
}

// ByTrustLevel returns a filtered copy matching the trust level.
// Empty trustLevel returns all conversations.
func (cs *ConversationStore) ByTrustLevel(trustLevel string) []ConversationInfo {
	return cs.FilterByTrust(trustLevel)
}

// SortedByName returns conversations sorted alphabetically by display name.
func (cs *ConversationStore) SortedByName() []ConversationInfo {
	convs := cs.List()
	sort.Slice(convs, func(i, j int) bool {
		return strings.ToLower(convs[i].DisplayName) < strings.ToLower(convs[j].DisplayName)
	})
	return convs
}

// SortedByTime returns conversations sorted by last activity (most recent first).
func (cs *ConversationStore) SortedByTime() []ConversationInfo {
	convs := cs.List()
	sort.Slice(convs, func(i, j int) bool {
		return convs[i].LastTime.After(convs[j].LastTime)
	})
	return convs
}

// MarkRead marks a conversation as read.
func (cs *ConversationStore) MarkRead(sourceHash string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if c, ok := cs.convs[sourceHash]; ok {
		c.Unread = false
	}
}

// MarkFailed marks a conversation as failed.
func (cs *ConversationStore) MarkFailed(sourceHash string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if c, ok := cs.convs[sourceHash]; ok {
		c.Failed = true
	}
}

// MarkAllRead marks all conversations as read.
func (cs *ConversationStore) MarkAllRead() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, c := range cs.convs {
		c.Unread = false
	}
}

// UnreadCount returns the number of conversations with unread messages.
func (cs *ConversationStore) UnreadCount() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	n := 0
	for _, c := range cs.convs {
		if c.Unread {
			n++
		}
	}
	return n
}

// SetLastSyncTime updates the last sync timestamp.
func (cs *ConversationStore) SetLastSyncTime(t time.Time) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastSyncTime = t
}

// SetSyncNodeLabel updates the propagation node label.
func (cs *ConversationStore) SetSyncNodeLabel(label string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastSyncNode = label
}

// LastSyncStatus returns the formatted sync status line.
// Matches Python's _sync_status_line() format exactly.
func (cs *ConversationStore) LastSyncStatus() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var when string
	if cs.lastSyncTime.IsZero() {
		when = "never"
	} else {
		when = RelativeTime(cs.lastSyncTime)
	}

	line := "Last sync: " + when
	if cs.lastSyncNode != "" {
		line += "  (" + cs.lastSyncNode + ")"
	}
	return line
}
