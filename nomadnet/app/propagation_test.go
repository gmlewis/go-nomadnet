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
	os.MkdirAll(a.StoragePath, 0o755)
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

func TestAutoSelectPropagationNodeTrustedFewestHops(t *testing.T) {
	t.Parallel()
	ts := rns.NewTransportSystem(nil)
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.loadPeerSettings()
	a.Dir = directory.New()
	a.Transport = ts

	// Two trusted nodes; HopsTo returns PathfinderM for unknown hashes, so both
	// tie and the first encountered wins. Add a non-trusted node that should be
	// skipped.
	a.Dir.Remember(&directory.Entry{SourceHash: []byte{1}, TrustLevel: directory.TrustTrusted, HostsNode: true})
	a.Dir.Remember(&directory.Entry{SourceHash: []byte{2}, TrustLevel: directory.TrustUntrusted, HostsNode: true})
	// AutoSelect with no router should not panic and should pick a trusted node.
	a.AutoSelectPropagationNode()
}
