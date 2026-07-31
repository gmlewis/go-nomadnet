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
)

func TestNewDirectoryDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	entries := []DirectoryEntry{
		{SourceHash: "abc123", DisplayName: "Alice", TrustLevel: "trusted"},
		{SourceHash: "def456", DisplayName: "Bob", TrustLevel: "untrusted"},
	}

	dd := NewDirectoryDisplay(app, entries)
	if dd == nil {
		t.Fatal("NewDirectoryDisplay returned nil")
	}
	if dd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestDirectoryDisplayEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	dd := NewDirectoryDisplay(app, nil)

	if dd == nil {
		t.Fatal("NewDirectoryDisplay with nil returned nil")
	}
}

func TestSortByTrust(t *testing.T) {
	t.Parallel()

	entries := []DirectoryEntry{
		{DisplayName: "Alice", TrustLevel: "untrusted"},
		{DisplayName: "Bob", TrustLevel: "trusted"},
		{DisplayName: "Charlie", TrustLevel: "unknown"},
	}

	SortByTrust(entries)

	if entries[0].DisplayName != "Bob" {
		t.Errorf("First entry = %q, want %q", entries[0].DisplayName, "Bob")
	}
	if entries[1].DisplayName != "Charlie" {
		t.Errorf("Second entry = %q, want %q", entries[1].DisplayName, "Charlie")
	}
	if entries[2].DisplayName != "Alice" {
		t.Errorf("Third entry = %q, want %q", entries[2].DisplayName, "Alice")
	}
}

func TestSortByName(t *testing.T) {
	t.Parallel()

	entries := []DirectoryEntry{
		{DisplayName: "Charlie"},
		{DisplayName: "Alice"},
		{DisplayName: "Bob"},
	}

	SortByName(entries)

	if entries[0].DisplayName != "Alice" {
		t.Errorf("First entry = %q, want %q", entries[0].DisplayName, "Alice")
	}
	if entries[1].DisplayName != "Bob" {
		t.Errorf("Second entry = %q, want %q", entries[1].DisplayName, "Bob")
	}
	if entries[2].DisplayName != "Charlie" {
		t.Errorf("Third entry = %q, want %q", entries[2].DisplayName, "Charlie")
	}
}

func TestDirectoryEntryLastMessage(t *testing.T) {
	t.Parallel()

	entry := DirectoryEntry{LastSeen: "5m ago"}
	if entry.LastMessage() != "5m ago" {
		t.Errorf("LastMessage() = %q, want %q", entry.LastMessage(), "5m ago")
	}

	entry2 := DirectoryEntry{}
	if entry2.LastMessage() != "unknown" {
		t.Errorf("LastMessage() empty = %q, want %q", entry2.LastMessage(), "unknown")
	}
}
