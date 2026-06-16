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
)

// Trust level constants matching Python's DirectoryEntry.
const (
	TrustUnknown   = "unknown"
	TrustUntrusted = "untrusted"
	TrustTrusted   = "trusted"
	TrustWarning   = "warning"
)

// NodeEntryFull holds full node information for the known-nodes store.
type NodeEntryFull struct {
	SourceHash        string
	DisplayName       string
	TrustLevel        string
	HostsNode         bool
	PreferredDelivery string
	SortRank          int
	Notes             string
}

// NodeStore manages the collection of known nodes/peers.
// Thread-safe for concurrent access.
type NodeStore struct {
	mu    sync.RWMutex
	nodes map[string]*NodeEntryFull
}

// NewNodeStore creates an empty node store.
func NewNodeStore() *NodeStore {
	return &NodeStore{nodes: make(map[string]*NodeEntryFull)}
}

// Add inserts or updates a node entry. Returns a pointer to the
// stored entry for subsequent mutation.
func (ns *NodeStore) Add(sourceHash, displayName, trustLevel string,
	hostsNode bool, delivery string) *NodeEntryFull {

	ns.mu.Lock()
	defer ns.mu.Unlock()

	e := &NodeEntryFull{
		SourceHash:        sourceHash,
		DisplayName:       displayName,
		TrustLevel:        trustLevel,
		HostsNode:         hostsNode,
		PreferredDelivery: delivery,
		SortRank:          -1,
	}
	ns.nodes[sourceHash] = e
	return e
}

// Get returns the entry for the given source hash, or nil if not found.
func (ns *NodeStore) Get(sourceHash string) *NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.nodes[sourceHash]
}

// Delete removes a node entry. No-op if not found.
func (ns *NodeStore) Delete(sourceHash string) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	delete(ns.nodes, sourceHash)
}

// Count returns the number of stored nodes.
func (ns *NodeStore) Count() int {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return len(ns.nodes)
}

// List returns all entries as a slice (unordered).
func (ns *NodeStore) List() []*NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	out := make([]*NodeEntryFull, 0, len(ns.nodes))
	for _, e := range ns.nodes {
		out = append(out, e)
	}
	return out
}

// KnownNodes returns entries sorted by (sort_rank, trust desc, name asc).
// Matches Python's Directory.known_nodes sorting.
func (ns *NodeStore) KnownNodes() []*NodeEntryFull {
	entries := ns.List()

	sort.SliceStable(entries, func(i, j int) bool {
		ri, rj := entries[i].SortRank, entries[j].SortRank
		// Unranked entries (sort_rank < 0) go to the end
		if ri < 0 && rj >= 0 {
			return false
		}
		if ri >= 0 && rj < 0 {
			return true
		}
		if ri >= 0 && rj >= 0 && ri != rj {
			return ri < rj
		}

		ti := trustOrder(entries[i].TrustLevel)
		tj := trustOrder(entries[j].TrustLevel)
		if ti != tj {
			return ti < tj
		}

		ni := strings.ToLower(entries[i].DisplayName)
		nj := strings.ToLower(entries[j].DisplayName)
		if ni != nj {
			return ni < nj
		}
		return entries[i].SourceHash < entries[j].SourceHash
	})

	return entries
}

// SearchByName returns entries whose display name contains the
// given substring (case-insensitive). An empty query matches all.
func (ns *NodeStore) SearchByName(query string) []*NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	q := strings.ToLower(query)
	var result []*NodeEntryFull
	for _, e := range ns.nodes {
		if q == "" || strings.Contains(strings.ToLower(e.DisplayName), q) {
			result = append(result, e)
		}
	}
	return result
}

// FilterByTrust returns entries matching the given trust level.
func (ns *NodeStore) FilterByTrust(level string) []*NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	var result []*NodeEntryFull
	for _, e := range ns.nodes {
		if e.TrustLevel == level {
			result = append(result, e)
		}
	}
	return result
}

// FilterByHashPrefix returns entries whose source hash starts with
// the given prefix (case-insensitive). An empty prefix matches all.
func (ns *NodeStore) FilterByHashPrefix(prefix string) []*NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	p := strings.ToLower(prefix)
	var result []*NodeEntryFull
	for _, e := range ns.nodes {
		if p == "" || strings.HasPrefix(strings.ToLower(e.SourceHash), p) {
			result = append(result, e)
		}
	}
	return result
}

// FilterByDelivery returns entries matching the given delivery method.
func (ns *NodeStore) FilterByDelivery(delivery string) []*NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	var result []*NodeEntryFull
	for _, e := range ns.nodes {
		if e.PreferredDelivery == delivery {
			result = append(result, e)
		}
	}
	return result
}

// HostsNodeOnly returns entries that are nodes (hosting pages).
func (ns *NodeStore) HostsNodeOnly() []*NodeEntryFull {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	var result []*NodeEntryFull
	for _, e := range ns.nodes {
		if e.HostsNode {
			result = append(result, e)
		}
	}
	return result
}

// trustOrder returns a numeric sort weight for trust levels.
// Lower values sort first: trusted > unknown > untrusted.
func trustOrder(level string) int {
	switch level {
	case TrustTrusted:
		return 0
	case TrustWarning:
		return 1
	case TrustUnknown:
		return 2
	case TrustUntrusted:
		return 3
	default:
		return 4
	}
}
