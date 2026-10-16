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
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/rns"
)

// unservedPropagationGuard pins the Go-only guard on the KnownNodeInfo
// "Use as default propagation node" checkbox. That checkbox derives the
// lxmf.propagation address of the identity that announced the NODE
// (Python Network.py:629), which is only a real mailbox when the propagation
// node itself announced that address. Kamrui's node is announced by its app
// identity, so the checkbox derives ef289bf8… — an address nothing serves.
// Python accepts it silently; the Go port must not.
//
// testAppWithTransport builds an App over a real but offline transport, enough
// to exercise the reachability guards without touching the network.
func testAppWithTransport(t *testing.T) (*App, *rns.TransportSystem) {
	t.Helper()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if err := os.MkdirAll(a.StoragePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%v): %v", a.StoragePath, err)
	}
	a.Logger = rns.NewLogger()
	a.Dir = directory.New()
	a.loadPeerSettings()
	ts := rns.NewTransportSystem(a.Logger)
	a.Transport = ts
	return a, ts
}

// rememberDestination seeds the transport's known-destination table for
// destHash with a freshly generated public key, exactly as a received announce
// would (TransportSystem.Remember). It returns the identity whose public key
// was stored, so a caller can derive destination hashes from it the way the
// announce's sender would have.
func rememberDestination(t *testing.T, ts *rns.TransportSystem, destHash []byte) *rns.Identity {
	t.Helper()
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	ts.Remember([]byte{0x01}, destHash, id.GetPublicKey(), nil)
	return id
}

func TestPropagationNodeReachabilityMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		identityKnown bool
		hasPath       bool
		want          string
	}{
		{"reachable", true, true, ""},
		{"no identity", false, true, "no identity"},
		{"no path", true, false, "no path"},
		{"neither", false, false, "no identity"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := propagationNodeMarker(tc.identityKnown, tc.hasPath); got != tc.want {
				t.Errorf("propagationNodeMarker(%v, %v) = %q, want %q",
					tc.identityKnown, tc.hasPath, got, tc.want)
			}
		})
	}
}

func TestPropagationNodeKnown(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	if a.PropagationNodeKnown(nil) {
		t.Error("PropagationNodeKnown(nil) = true, want false")
	}
	unknown := bytes.Repeat([]byte{0xab}, 16)
	if a.PropagationNodeKnown(unknown) {
		t.Error("PropagationNodeKnown(announced by nobody) = true, want false")
	}
	rememberDestination(t, ts, unknown)
	if !a.PropagationNodeKnown(unknown) {
		t.Error("PropagationNodeKnown(announced) = false, want true")
	}
}

func TestSetPropagationNodeFromNodeIdentityRefusesUnserved(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	// A node announced by its app identity: the derived lxmf.propagation address
	// is the app identity's, which no propagation node serves. This is the exact
	// shape of the kamrui trap.
	nodeHash := bytes.Repeat([]byte{0x1f}, 16)
	rememberDestination(t, ts, nodeHash)
	if a.NodePropagationHash(nodeHash) == nil {
		t.Fatal("precondition: node identity not recallable")
	}

	err := a.SetPropagationNodeFromNodeIdentity(nodeHash)
	if !errors.Is(err, ErrPropagationNodeUnproven) {
		t.Fatalf("SetPropagationNodeFromNodeIdentity = %v, want ErrPropagationNodeUnproven", err)
	}
	if got := a.GetUserSelectedPropagationNode(); got != nil {
		t.Errorf("user-selected PN = %x, want nil (an unserved address must not be accepted)", got)
	}
}

func TestSetPropagationNodeFromNodeIdentityAcceptsServed(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	nodeHash := bytes.Repeat([]byte{0x2f}, 16)
	id := rememberDestination(t, ts, nodeHash)

	// The node's identity *is* the propagation node's, so the derived address is
	// backed by an announce (the legitimate case the Info page can produce).
	pnHash := rns.CalculateHash(id, "lxmf", "propagation")
	rememberDestination(t, ts, pnHash)

	if err := a.SetPropagationNodeFromNodeIdentity(nodeHash); err != nil {
		t.Fatalf("SetPropagationNodeFromNodeIdentity = %v, want nil", err)
	}
	if got := a.GetUserSelectedPropagationNode(); !bytes.Equal(got, pnHash) {
		t.Errorf("user-selected PN = %x, want %x", got, pnHash)
	}
}

func TestSetPropagationNodeFromNodeIdentityUnknownNode(t *testing.T) {
	t.Parallel()

	a, _ := testAppWithTransport(t)

	err := a.SetPropagationNodeFromNodeIdentity(bytes.Repeat([]byte{0x3f}, 16))
	if !errors.Is(err, ErrPropagationNodeUnproven) {
		t.Fatalf("SetPropagationNodeFromNodeIdentity(unknown node) = %v, want ErrPropagationNodeUnproven", err)
	}
	if got := a.GetUserSelectedPropagationNode(); got != nil {
		t.Errorf("user-selected PN = %x, want nil", got)
	}
}

// TestPropagationNodeMarkerFor covers the two reachability facts the transport
// can supply, plus the no-selection and no-transport cases. The
// identity-known-and-path-known case is covered by the pure table test above,
// which needs no transport.
func TestPropagationNodeMarkerFor(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	if got := a.propagationNodeMarkerFor(nil); got != "" {
		t.Errorf("propagationNodeMarkerFor(nil) = %q, want \"\"", got)
	}

	unknown := bytes.Repeat([]byte{0x4f}, 16)
	if got := a.propagationNodeMarkerFor(unknown); got != "no identity" {
		t.Errorf("propagationNodeMarkerFor(unknown) = %q, want %q", got, "no identity")
	}

	// An identity that announced but for which no path is known yet.
	rememberDestination(t, ts, unknown)
	if got := a.propagationNodeMarkerFor(unknown); got != "no path" {
		t.Errorf("propagationNodeMarkerFor(no path) = %q, want %q", got, "no path")
	}
}

// TestLastSyncInfoMarksUnreachableNode pins the Conversations footer's
// reachability marker: Python's _sync_status_line (Conversations.py:517-545)
// renders the node label with no reachability information, so a client
// configured with a dead address renders exactly like a healthy one.
func TestLastSyncInfoMarksUnreachableNode(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	// No node selected: no label, no marker.
	if _, label := a.lastSyncInfoFor(nil); label != "" {
		t.Errorf("lastSyncInfoFor(nil) label = %q, want \"\"", label)
	}

	dead := bytes.Repeat([]byte{0xef}, 16)
	_, label := a.lastSyncInfoFor(dead)
	if !strings.Contains(label, "no identity") {
		t.Errorf("lastSyncInfoFor(unknown identity) label = %q, want it to carry the no-identity marker", label)
	}
	if !strings.HasPrefix(label, "<"+hexPrefix(dead)+"…>") {
		t.Errorf("lastSyncInfoFor(unknown identity) label = %q, want the <hex…> fallback prefix", label)
	}

	// Known identity but no path: a different marker.
	rememberDestination(t, ts, dead)
	_, label = a.lastSyncInfoFor(dead)
	if !strings.Contains(label, "no path") {
		t.Errorf("lastSyncInfoFor(no path) label = %q, want it to carry the no-path marker", label)
	}
}

func TestPropagationNodeUnprovenNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pnHash   []byte
		contains []string
	}{
		{
			name:     "names the refused address and where to go instead",
			pnHash:   bytes.Repeat([]byte{0xef}, 16),
			contains: []string{"<efefefef…>", "Propagation Nodes", "Ctrl-L"},
		},
		{
			name:     "copes with an underivable address",
			pnHash:   nil,
			contains: []string{"this address", "Propagation Nodes", "Ctrl-L"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PropagationNodeUnprovenNotice(tc.pnHash)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("PropagationNodeUnprovenNotice() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// TestPropagationNodeMarkerFitsTheFooter guards the marker's length. The footer
// is one line in the Conversations left pane, and the marker is appended after
// the node label, so a marker that is too long is the *reason* that gets cut off
// at the right edge — leaving " Last sync: …  (<7acc095f…>, " and no explanation,
// which is worse than showing nothing. The first wording shipped that way.
//
// The fixed parts are the worst realistic ones: a three-minute-old sync, and the
// "<8 hex chars…>" node label Python falls back to for a propagation node (they
// announce with an empty name). tui pads the line to its listWidth minus the
// 2-column border, and pads by BYTE length, so the "…" counts as three.
func TestPropagationNodeMarkerFitsTheFooter(t *testing.T) {
	t.Parallel()

	const paneInnerWidth = 50 // tui's default listWidth (52) minus its border
	for _, marker := range []string{propagationNodeMarker(true, false), propagationNodeMarker(false, true)} {
		line := " Last sync: 27m ago  (<7acc095f…>, " + marker + ")"
		if len(line) > paneInnerWidth {
			t.Errorf("footer line %q is %v bytes; the pane holds %v, so the reason would be clipped",
				line, len(line), paneInnerWidth)
		}
	}
}

// hexPrefix is the first 8 hex characters of a destination hash, matching the
// footer's "<hex…>" fallback (Python Network.py's hexrep(...)[:8]).
func hexPrefix(h []byte) string {
	const digits = "0123456789abcdef"
	var b strings.Builder
	for _, v := range h {
		b.WriteByte(digits[v>>4])
		b.WriteByte(digits[v&0x0f])
		if b.Len() >= 8 {
			break
		}
	}
	return b.String()
}
