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

func TestTrustDisplayIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level string
		want  string
	}{
		{TrustTrusted, "●"},
		{TrustUntrusted, "×"},
		{TrustWarning, "⚠"},
		{TrustUnknown, "○"},
		{"unknown", "○"},
		{"", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()
			got := TrustDisplayIcon(tt.level)
			if got != tt.want {
				t.Errorf("TrustDisplayIcon(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestFormatTrustLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level string
		want  string
	}{
		{TrustTrusted, "Trusted"},
		{TrustUntrusted, "Untrusted"},
		{TrustWarning, "Warning"},
		{TrustUnknown, "Unknown"},
		{"", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()
			got := FormatTrustLabel(tt.level)
			if got != tt.want {
				t.Errorf("FormatTrustLabel(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestFormatNodeSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry *NodeEntryFull
		want  string
	}{
		{
			name:  "trusted with name",
			entry: &NodeEntryFull{DisplayName: "Alice", TrustLevel: TrustTrusted, SourceHash: "aabbccddeeff"},
			want:  "● Alice",
		},
		{
			name:  "untrusted with name",
			entry: &NodeEntryFull{DisplayName: "Eve", TrustLevel: TrustUntrusted, SourceHash: "112233445566"},
			want:  "× Eve",
		},
		{
			name:  "unknown without name",
			entry: &NodeEntryFull{TrustLevel: TrustUnknown, SourceHash: "aabbccddeeff00112233"},
			want:  "○ <aabbccddeeff…>",
		},
		{
			name:  "warning with short hash",
			entry: &NodeEntryFull{TrustLevel: TrustWarning, SourceHash: "abc"},
			want:  "⚠ <abc>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatNodeSummary(tt.entry)
			if got != tt.want {
				t.Errorf("FormatNodeSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatNodeDetail(t *testing.T) {
	t.Parallel()

	entry := &NodeEntryFull{
		DisplayName:       "MyNode",
		TrustLevel:        TrustTrusted,
		SourceHash:        "aabbccddeeff00112233",
		HostsNode:         true,
		PreferredDelivery: "lxmf",
		SortRank:          5,
		Notes:             "Test notes",
	}

	detail := FormatNodeDetail(entry, false)
	if detail == "" {
		t.Error("FormatNodeDetail returned empty")
	}
	if !strings.Contains(detail, "MyNode") {
		t.Error("Detail missing name")
	}
	if !strings.Contains(detail, "Trusted") {
		t.Error("Detail missing trust label")
	}
	if !strings.Contains(detail, "aabbccddeeff00112233") {
		t.Error("Detail missing hash")
	}
	if !strings.Contains(detail, "lxmf") {
		t.Error("Detail missing delivery")
	}
	if !strings.Contains(detail, "Sort Rank: 5") {
		t.Error("Detail missing sort rank")
	}
	if !strings.Contains(detail, "Notes: Test notes") {
		t.Error("Detail missing notes")
	}
}

func TestFormatNodeDetailEditable(t *testing.T) {
	t.Parallel()

	entry := &NodeEntryFull{DisplayName: "Test", TrustLevel: TrustUnknown, SourceHash: "aabb"}
	detail := FormatNodeDetail(entry, true)
	if !strings.Contains(detail, "(editable)") {
		t.Error("Editable flag not shown")
	}
}

func TestFormatNodeDetailNoNode(t *testing.T) {
	t.Parallel()

	entry := &NodeEntryFull{TrustLevel: TrustUnknown, SourceHash: "aabb"}
	detail := FormatNodeDetail(entry, false)
	if strings.Contains(detail, "Node: yes") {
		t.Error("Non-node should not show Node: yes")
	}
}

func TestSearchConversations(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "aabb1122", DisplayName: "Alice Chat"},
		{SourceHash: "ccdd3344", DisplayName: "Bob Room"},
		{SourceHash: "eeff5566", DisplayName: "Charlie"},
	}

	tests := []struct {
		name  string
		query string
		count int
	}{
		{"empty query", "", 3},
		{"name match", "alice", 1},
		{"hash prefix", "aabb", 1},
		{"case insensitive", "CHARLIE", 1},
		{"partial match", "oo", 1},
		{"no match", "zzz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SearchConversations(convs, tt.query)
			if len(got) != tt.count {
				t.Errorf("SearchConversations(%q) returned %v, want %v", tt.query, len(got), tt.count)
			}
		})
	}
}

func TestSearchConversationsEmptyInput(t *testing.T) {
	t.Parallel()

	got := SearchConversations(nil, "test")
	if len(got) != 0 {
		t.Errorf("SearchConversations(nil, %q) returned %v, want 0", "test", len(got))
	}
}

func TestFormatConversationSummary(t *testing.T) {
	t.Parallel()

	conv := ConversationInfo{
		DisplayName:  "Alice Chat",
		TrustLevel:   "trusted",
		MessageCount: 42,
		LastTime:     time.Now().Add(-5 * time.Minute),
	}

	summary := FormatConversationSummary(conv)
	if summary == "" {
		t.Error("FormatConversationSummary returned empty")
	}
	if !strings.Contains(summary, "Alice Chat") {
		t.Error("Summary missing name")
	}
	if !strings.Contains(summary, "Trusted") {
		t.Error("Summary missing trust")
	}
	if !strings.Contains(summary, "42") {
		t.Error("Summary missing message count")
	}
}
