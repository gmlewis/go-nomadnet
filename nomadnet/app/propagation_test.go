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
	"bytes"
	"os"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/rns"
)

func TestUserSelectedPropagationNode(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if err := os.MkdirAll(a.StoragePath, 0o755); err != nil {
		t.Fatal(err)
	}
	a.loadPeerSettings()
	if a.GetUserSelectedPropagationNode() != nil {
		t.Fatal("default should be nil")
	}
	node := []byte{1, 2, 3, 4}
	a.SetUserSelectedPropagationNode(node)
	if got := a.GetUserSelectedPropagationNode(); !bytes.Equal(got, node) {
		t.Fatalf("got %v, want %v", got, node)
	}
	// persisted
	a2 := NewApp(a.ConfigDir, "", false, false)
	a2.setupPaths()
	a2.loadPeerSettings()
	if got := a2.GetUserSelectedPropagationNode(); !bytes.Equal(got, node) {
		t.Fatalf("reloaded got %v, want %v", got, node)
	}
}

func TestGetDefaultPropagationNodeNoRouter(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if got := a.GetDefaultPropagationNode(); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestAutoSelectPropagationNodeUserSelected(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.loadPeerSettings()
	node := []byte{0xaa, 0xbb}
	a.SetUserSelectedPropagationNode(node)
	// Without a router, AutoSelect should still not panic; default stays nil.
	if got := a.GetDefaultPropagationNode(); got != nil {
		t.Fatalf("got %v, want nil without router", got)
	}
}

// TestAutoSelectPropagationNodeTrustedFewestHops pins the selection rule Python's
// autoselect_propagation_node implements (NomadNetworkApp.py:613-625): among the
// TRUSTED directory nodes only, the one with the fewest hops wins, and its strict
// `<` means the first of several equally-close nodes is kept. Untrusted nodes are
// skipped however close they are, and with no trusted node nothing is selected at
// all.
func TestAutoSelectPropagationNodeTrustedFewestHops(t *testing.T) {
	t.Parallel()

	trusted := func(hash byte) *directory.Entry {
		return &directory.Entry{SourceHash: []byte{hash}, TrustLevel: directory.TrustTrusted, HostsNode: true}
	}
	untrusted := func(hash byte) *directory.Entry {
		return &directory.Entry{SourceHash: []byte{hash}, TrustLevel: directory.TrustUntrusted, HostsNode: true}
	}

	tests := []struct {
		name  string
		nodes []*directory.Entry
		// hops maps a node's single-byte source hash to its hop count. A hash
		// absent from the map is unreachable (PathfinderM), like an unlearned path.
		hops map[byte]int
		// want is the selected hash's single byte, or 0 for "nothing selected".
		want byte
	}{
		{
			name:  "the trusted node with the fewest hops wins",
			nodes: []*directory.Entry{trusted(1), trusted(2), trusted(3)},
			hops:  map[byte]int{1: 4, 2: 2, 3: 7},
			want:  2,
		},
		{
			name:  "an untrusted node is skipped however close it is",
			nodes: []*directory.Entry{untrusted(1), trusted(2)},
			hops:  map[byte]int{1: 1, 2: 5},
			want:  2,
		},
		{
			name:  "the first of several equally-close nodes wins",
			nodes: []*directory.Entry{trusted(1), trusted(2), trusted(3)},
			hops:  map[byte]int{1: 3, 2: 3, 3: 3},
			want:  1,
		},
		{
			name:  "no trusted node selects nothing",
			nodes: []*directory.Entry{untrusted(1), untrusted(2)},
			hops:  map[byte]int{1: 1, 2: 2},
			want:  0,
		},
		{
			name:  "a directory with no nodes selects nothing",
			nodes: nil,
			hops:  map[byte]int{},
			want:  0,
		},
		{
			name:  "an unreachable trusted node is still selected when it is the only one",
			nodes: []*directory.Entry{trusted(1)},
			hops:  map[byte]int{},
			want:  1,
		},
		{
			name:  "a later nearer node overtakes an earlier one",
			nodes: []*directory.Entry{trusted(1), trusted(2)},
			hops:  map[byte]int{1: 6, 2: 2},
			want:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hops := func(hash []byte) int {
				if len(hash) == 1 {
					if h, ok := tc.hops[hash[0]]; ok {
						return h
					}
				}
				return rns.PathfinderM
			}

			got := autoSelectPropagationNode(tc.nodes, hops)
			if tc.want == 0 {
				if got != nil {
					t.Fatalf("selected %x, want nothing (no trusted node)", got)
				}
				return
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("selected %x, want [%x]", got, tc.want)
			}
		})
	}
}

// TestAutoSelectPropagationNodeAppPath pins the app-level half: a stored user choice
// is used as-is, and it is stored on the router (Python's
// autoselect_propagation_node sets it via set_outbound_propagation_node,
// NomadNetworkApp.py:632).
func TestAutoSelectPropagationNodeAppPath(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)

	// A propagation node hash is a truncated destination hash: 16 bytes, which is
	// what the router's setter requires.
	userChosen := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0x01, 0x02, 0x03, 0x04,
		0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}
	a.SetUserSelectedPropagationNode(userChosen)

	if got := a.Router.GetOutboundPropagationNode(); !bytes.Equal(got, userChosen) {
		t.Errorf("outbound propagation node = %x, want the user's %x", got, userChosen)
	}
	if got := a.GetDefaultPropagationNode(); !bytes.Equal(got, userChosen) {
		t.Errorf("GetDefaultPropagationNode = %x, want the user's %x", got, userChosen)
	}
}

// TestAutoSelectPropagationNodeWithoutATrustedNode pins the "nothing to select"
// branch: with a directory holding only untrusted nodes, the router's outbound
// propagation node stays unset.
func TestAutoSelectPropagationNodeWithoutATrustedNode(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)
	a.Dir.Remember(&directory.Entry{SourceHash: []byte{1}, TrustLevel: directory.TrustUntrusted, HostsNode: true})

	a.AutoSelectPropagationNode()

	if got := a.Router.GetOutboundPropagationNode(); got != nil {
		t.Errorf("outbound propagation node = %x, want nil with no trusted node", got)
	}
}
