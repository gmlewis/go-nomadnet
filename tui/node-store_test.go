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

func TestNodeStoreAddAndGet(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	e := ns.Add("aabb", "Alice", "trusted", true, "tcp")
	if e.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want %q", e.DisplayName, "Alice")
	}
	if e.TrustLevel != "trusted" {
		t.Errorf("TrustLevel = %q, want %q", e.TrustLevel, "trusted")
	}
	if !e.HostsNode {
		t.Error("HostsNode should be true")
	}
	if e.PreferredDelivery != "tcp" {
		t.Errorf("PreferredDelivery = %q, want %q", e.PreferredDelivery, "tcp")
	}

	got := ns.Get("aabb")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.DisplayName != "Alice" {
		t.Errorf("Get().DisplayName = %q, want %q", got.DisplayName, "Alice")
	}
}

func TestNodeStoreGetMissing(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	if ns.Get("nonexistent") != nil {
		t.Error("Get on missing key should return nil")
	}
}

func TestNodeStoreDelete(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")
	ns.Delete("aabb")
	if ns.Get("aabb") != nil {
		t.Error("Get after delete should return nil")
	}
}

func TestNodeStoreUpdate(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")

	e := ns.Get("aabb")
	e.DisplayName = "Alice Updated"
	e.SortRank = 5
	e.Notes = "Test notes"

	got := ns.Get("aabb")
	if got.DisplayName != "Alice Updated" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Alice Updated")
	}
	if got.SortRank != 5 {
		t.Errorf("SortRank = %d, want 5", got.SortRank)
	}
	if got.Notes != "Test notes" {
		t.Errorf("Notes = %q, want %q", got.Notes, "Test notes")
	}
}

func TestNodeStoreList(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")
	ns.Add("ccdd", "Bob", "untrusted", false, "lxmf")

	nodes := ns.List()
	if len(nodes) != 2 {
		t.Errorf("List() returned %d nodes, want 2", len(nodes))
	}
}

func TestNodeStoreKnownNodesSort(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("0001", "Charlie", "untrusted", false, "lxmf")
	ns.Add("0002", "Alice", "trusted", true, "tcp")
	ns.Add("0003", "Bob", "trusted", false, "lxmf")

	nodes := ns.KnownNodes()
	// Trusted first, then alphabetical
	if len(nodes) != 3 {
		t.Fatalf("KnownNodes() returned %d nodes, want 3", len(nodes))
	}
	if nodes[0].DisplayName != "Alice" {
		t.Errorf("first = %q, want %q (trusted first)", nodes[0].DisplayName, "Alice")
	}
	if nodes[1].DisplayName != "Bob" {
		t.Errorf("second = %q, want %q", nodes[1].DisplayName, "Bob")
	}
}

func TestNodeStoreKnownNodesSortByRank(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("0001", "Alice", "trusted", true, "tcp")
	ns.Add("0002", "Bob", "trusted", false, "lxmf")

	a := ns.Get("0001")
	a.SortRank = 2
	b := ns.Get("0002")
	b.SortRank = 1

	nodes := ns.KnownNodes()
	// Lower sort rank first
	if nodes[0].DisplayName != "Bob" {
		t.Errorf("first = %q, want %q (sort_rank 1)", nodes[0].DisplayName, "Bob")
	}
}

func TestNodeStoreCount(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	if ns.Count() != 0 {
		t.Errorf("Count() = %d, want 0", ns.Count())
	}
	ns.Add("aabb", "Alice", "trusted", true, "tcp")
	if ns.Count() != 1 {
		t.Errorf("Count() = %d, want 1", ns.Count())
	}
}

func TestNodeStoreUpdateTrustLevel(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "unknown", false, "lxmf")

	e := ns.Get("aabb")
	e.TrustLevel = "trusted"
	e.HostsNode = true

	got := ns.Get("aabb")
	if got.TrustLevel != "trusted" {
		t.Errorf("TrustLevel = %q, want %q", got.TrustLevel, "trusted")
	}
	if !got.HostsNode {
		t.Error("HostsNode should be true")
	}
}

func TestNodeStoreDeleteNonexistent(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Delete("nonexistent")
	// Should not panic
}

func TestNodeStoreSearchByName(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice Node", "trusted", true, "tcp")
	ns.Add("ccdd", "Bob Hub", "untrusted", false, "lxmf")
	ns.Add("eeff", "Alicia Peer", "trusted", false, "lxmf")

	results := ns.SearchByName("ali")
	if len(results) != 2 {
		t.Fatalf("SearchByName('ali') returned %d, want 2", len(results))
	}
	// Results should be case-insensitive
	for _, r := range results {
		if r.DisplayName == "Bob Hub" {
			t.Errorf("SearchByName returned unexpected node %q", r.DisplayName)
		}
	}
}

func TestNodeStoreSearchByNameEmpty(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")

	results := ns.SearchByName("")
	if len(results) != 1 {
		t.Errorf("SearchByName('') returned %d, want 1 (empty matches all)", len(results))
	}
}

func TestNodeStoreFilterByTrust(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")
	ns.Add("ccdd", "Bob", "untrusted", false, "lxmf")
	ns.Add("eeff", "Charlie", "warning", false, "lxmf")

	trusted := ns.FilterByTrust("trusted")
	if len(trusted) != 1 {
		t.Fatalf("FilterByTrust('trusted') returned %d, want 1", len(trusted))
	}
	if trusted[0].DisplayName != "Alice" {
		t.Errorf("FilterByTrust('trusted')[0] = %q, want %q", trusted[0].DisplayName, "Alice")
	}

	untrusted := ns.FilterByTrust("untrusted")
	if len(untrusted) != 1 {
		t.Fatalf("FilterByTrust('untrusted') returned %d, want 1", len(untrusted))
	}
	if untrusted[0].DisplayName != "Bob" {
		t.Errorf("FilterByTrust('untrusted')[0] = %q, want %q", untrusted[0].DisplayName, "Bob")
	}
}

func TestNodeStoreFilterByHashPrefix(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb1122", "Alice", "trusted", true, "tcp")
	ns.Add("aabb3344", "Bob", "untrusted", false, "lxmf")
	ns.Add("ccdd5566", "Charlie", "trusted", false, "lxmf")

	results := ns.FilterByHashPrefix("aabb")
	if len(results) != 2 {
		t.Fatalf("FilterByHashPrefix('aabb') returned %d, want 2", len(results))
	}
}

func TestNodeStoreFilterByDelivery(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")
	ns.Add("ccdd", "Bob", "trusted", false, "lxmf")

	results := ns.FilterByDelivery("tcp")
	if len(results) != 1 {
		t.Fatalf("FilterByDelivery('tcp') returned %d, want 1", len(results))
	}
	if results[0].DisplayName != "Alice" {
		t.Errorf("FilterByDelivery('tcp')[0] = %q, want %q", results[0].DisplayName, "Alice")
	}
}

func TestNodeStoreHostsNode(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")
	ns.Add("ccdd", "Bob", "trusted", false, "lxmf")

	nodes := ns.HostsNodeOnly()
	if len(nodes) != 1 {
		t.Fatalf("HostsNodeOnly() returned %d, want 1", len(nodes))
	}
	if nodes[0].DisplayName != "Alice" {
		t.Errorf("HostsNodeOnly()[0] = %q, want %q", nodes[0].DisplayName, "Alice")
	}
}

func TestNodeStoreSearchByHashPrefixEmpty(t *testing.T) {
	t.Parallel()

	ns := NewNodeStore()
	ns.Add("aabb", "Alice", "trusted", true, "tcp")

	results := ns.FilterByHashPrefix("")
	if len(results) != 1 {
		t.Errorf("FilterByHashPrefix('') returned %d, want 1 (empty matches all)", len(results))
	}
}
