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

import "testing"

func TestAnnounceFilterNew(t *testing.T) {
	t.Parallel()

	af := NewAnnounceFilter(nil)
	if af.Count() != 0 {
		t.Errorf("Count() = %v, want 0", af.Count())
	}
}

func TestAnnounceFilterWithType(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aa", DisplayName: "Node1", Type: "node"},
		{SourceHash: "bb", DisplayName: "Peer1", Type: "peer"},
		{SourceHash: "cc", DisplayName: "PN1", Type: "pn"},
	}

	af := NewAnnounceFilter(entries)
	af.SetTypeFilter("node")

	results := af.Filtered()
	if len(results) != 1 {
		t.Errorf("Filtered() returned %v, want 1", len(results))
	}
	if results[0].DisplayName != "Node1" {
		t.Errorf("first result = %q, want %q", results[0].DisplayName, "Node1")
	}
}

func TestAnnounceFilterWithSearch(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aa", DisplayName: "Alice Node", Type: "node"},
		{SourceHash: "bb", DisplayName: "Bob Peer", Type: "peer"},
		{SourceHash: "cc", DisplayName: "Charlie Node", Type: "node"},
	}

	af := NewAnnounceFilter(entries)
	af.SetSearch("alice")

	results := af.Filtered()
	if len(results) != 1 {
		t.Errorf("Filtered() returned %v, want 1", len(results))
	}
	if results[0].DisplayName != "Alice Node" {
		t.Errorf("first result = %q, want %q", results[0].DisplayName, "Alice Node")
	}
}

func TestAnnounceFilterWithTrust(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aa", DisplayName: "Trusted", TrustLevel: "trusted"},
		{SourceHash: "bb", DisplayName: "Unknown", TrustLevel: "unknown"},
		{SourceHash: "cc", DisplayName: "Untrusted", TrustLevel: "untrusted"},
	}

	af := NewAnnounceFilter(entries)
	af.SetTrustFilter("trusted")

	results := af.Filtered()
	if len(results) != 1 {
		t.Errorf("Filtered() returned %v, want 1", len(results))
	}
	if results[0].DisplayName != "Trusted" {
		t.Errorf("first result = %q, want %q", results[0].DisplayName, "Trusted")
	}
}

func TestAnnounceFilterNoFilter(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aa", DisplayName: "A", Type: "node"},
		{SourceHash: "bb", DisplayName: "B", Type: "peer"},
	}

	af := NewAnnounceFilter(entries)
	results := af.Filtered()
	if len(results) != 2 {
		t.Errorf("Filtered() returned %v, want 2", len(results))
	}
}

func TestAnnounceFilterCombined(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aa", DisplayName: "Alice Node", Type: "node", TrustLevel: "trusted"},
		{SourceHash: "bb", DisplayName: "Bob Peer", Type: "peer", TrustLevel: "trusted"},
		{SourceHash: "cc", DisplayName: "Alice Peer", Type: "peer", TrustLevel: "untrusted"},
	}

	af := NewAnnounceFilter(entries)
	af.SetTypeFilter("peer")
	af.SetSearch("alice")

	results := af.Filtered()
	if len(results) != 1 {
		t.Errorf("Filtered() returned %v, want 1", len(results))
	}
	if results[0].DisplayName != "Alice Peer" {
		t.Errorf("first result = %q, want %q", results[0].DisplayName, "Alice Peer")
	}
}

func TestAnnounceFilterClearFilters(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aa", DisplayName: "A", Type: "node"},
		{SourceHash: "bb", DisplayName: "B", Type: "peer"},
	}

	af := NewAnnounceFilter(entries)
	af.SetTypeFilter("node")
	af.ClearFilters()

	results := af.Filtered()
	if len(results) != 2 {
		t.Errorf("Filtered() after ClearFilters returned %v, want 2", len(results))
	}
}

func TestAnnounceFilterSearchByHash(t *testing.T) {
	t.Parallel()

	entries := []AnnounceEntry{
		{SourceHash: "aabbccdd", DisplayName: "Test", Type: "node"},
		{SourceHash: "11223344", DisplayName: "Other", Type: "peer"},
	}

	af := NewAnnounceFilter(entries)
	af.SetSearch("aabb")

	results := af.Filtered()
	if len(results) != 1 {
		t.Errorf("Filtered() returned %v, want 1", len(results))
	}
	if results[0].SourceHash != "aabbccdd" {
		t.Errorf("first result hash = %q, want %q", results[0].SourceHash, "aabbccdd")
	}
}

func TestAnnounceFilterUpdateEntries(t *testing.T) {
	t.Parallel()

	af := NewAnnounceFilter([]AnnounceEntry{
		{SourceHash: "aa", DisplayName: "A", Type: "node"},
	})

	if af.Count() != 1 {
		t.Errorf("Count() = %v, want 1", af.Count())
	}

	af.UpdateEntries([]AnnounceEntry{
		{SourceHash: "aa", DisplayName: "A", Type: "node"},
		{SourceHash: "bb", DisplayName: "B", Type: "peer"},
		{SourceHash: "cc", DisplayName: "C", Type: "pn"},
	})

	if af.Count() != 3 {
		t.Errorf("Count() after update = %v, want 3", af.Count())
	}
}
