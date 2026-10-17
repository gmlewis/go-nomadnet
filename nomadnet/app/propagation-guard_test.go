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
	"github.com/gmlewis/go-reticulum/rns/msgpack"
)

// propagationAppData builds the propagation announce payload a declaration
// carries, mirroring Python's get_propagation_node_app_data (LXMRouter.py:324-336):
// a seven-element list whose third element is the node_state flag — true when the
// announcing router serves propagation, false when it declares an address it is
// not serving.
func propagationAppData(t *testing.T, serving bool) []byte {
	t.Helper()
	packed, err := msgpack.Pack([]any{
		false,
		1700000000,
		serving,
		128,
		256,
		[]any{16, 3, 16},
		map[any]any{1: []byte("Node")},
	})
	if err != nil {
		t.Fatalf("Pack propagation app data: %v", err)
	}
	return packed
}

// rememberPropagationNodeAnnounce seeds destHash as an address that has been
// announced, carrying the given serving declaration — exactly what
// TransportSystem.Remember stores for a received propagation announce.
func rememberPropagationNodeAnnounce(t *testing.T, ts *rns.TransportSystem, destHash []byte, serving bool) {
	t.Helper()
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	ts.Remember([]byte{0x02}, destHash, id.GetPublicKey(), propagationAppData(t, serving))
}

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
		serving       bool
		hasPath       bool
		want          string
	}{
		{"reachable", true, true, true, ""},
		{"no identity", false, false, true, "no identity"},
		{"no identity, no path", false, false, false, "no identity"},
		{"declared but stopped", true, false, true, "stopped"},
		{"declared but stopped, no path", true, false, false, "stopped"},
		{"serving but no path", true, true, false, "no path"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := propagationNodeMarker(tc.identityKnown, tc.serving, tc.hasPath); got != tc.want {
				t.Errorf("propagationNodeMarker(%v, %v, %v) = %q, want %q",
					tc.identityKnown, tc.serving, tc.hasPath, got, tc.want)
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

	// The node's identity *is* the propagation node's, and that address is
	// announced with node_state true: the legitimate case the Info page can
	// produce (Python's own arrangement, where a node that serves a propagation
	// node declares it from its own identity).
	pnHash := rns.CalculateHash(id, "lxmf", "propagation")
	rememberPropagationNodeAnnounce(t, ts, pnHash, true)

	if err := a.SetPropagationNodeFromNodeIdentity(nodeHash); err != nil {
		t.Fatalf("SetPropagationNodeFromNodeIdentity = %v, want nil", err)
	}
	if got := a.GetUserSelectedPropagationNode(); !bytes.Equal(got, pnHash) {
		t.Errorf("user-selected PN = %x, want %x", got, pnHash)
	}
}

// TestSetPropagationNodeFromNodeIdentityRefusesDeclaredButStopped pins the
// serving test against a declared address that is not serving. This is the shape
// the Go fleet has after the Python-parity declaration lands: every NomadNet node
// announces its own lxmf.propagation address on each node announce
// (Node.py:216-220) with node_state false while propagation is switched off. Such
// an address is real and announced but answers no requests, so adopting it would
// silently fail every propagated send — exactly the trap the guard exists for.
// It is also why the guard cannot test mere existence
// (PropagationNodeKnown): the declaration would otherwise reopen the hole.
func TestSetPropagationNodeFromNodeIdentityRefusesDeclaredButStopped(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	nodeHash := bytes.Repeat([]byte{0x5f}, 16)
	id := rememberDestination(t, ts, nodeHash)
	pnHash := rns.CalculateHash(id, "lxmf", "propagation")
	rememberPropagationNodeAnnounce(t, ts, pnHash, false)

	if !a.PropagationNodeKnown(pnHash) {
		t.Fatal("precondition: the address should be announced (declared)")
	}
	if a.PropagationNodeServing(pnHash) {
		t.Fatal("precondition: a node_state=false declaration must not count as serving")
	}

	err := a.SetPropagationNodeFromNodeIdentity(nodeHash)
	if !errors.Is(err, ErrPropagationNodeUnproven) {
		t.Fatalf("SetPropagationNodeFromNodeIdentity(declared but stopped) = %v, want ErrPropagationNodeUnproven", err)
	}
	if got := a.GetUserSelectedPropagationNode(); got != nil {
		t.Errorf("user-selected PN = %x, want nil (a stopped node must not be adopted)", got)
	}
}

// TestPropagationNodeServing covers the serving predicate directly: no announce,
// a declaration with node_state true, a declaration with node_state false, and a
// declaration whose payload carries no readable state (e.g. an implicit
// re-announce with no app data).
func TestPropagationNodeServing(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	if a.PropagationNodeServing(nil) {
		t.Error("PropagationNodeServing(nil) = true, want false")
	}

	unknown := bytes.Repeat([]byte{0x6f}, 16)
	if a.PropagationNodeServing(unknown) {
		t.Error("PropagationNodeServing(announced by nobody) = true, want false")
	}

	rememberPropagationNodeAnnounce(t, ts, unknown, true)
	if !a.PropagationNodeServing(unknown) {
		t.Error("PropagationNodeServing(node_state true) = false, want true")
	}

	stopped := bytes.Repeat([]byte{0x7f}, 16)
	rememberPropagationNodeAnnounce(t, ts, stopped, false)
	if a.PropagationNodeServing(stopped) {
		t.Error("PropagationNodeServing(node_state false) = true, want false")
	}

	// An announce with no app data carries no state element: not serving, and a
	// different outcome from the no-identity case.
	unreadable := bytes.Repeat([]byte{0x8f}, 16)
	rememberDestination(t, ts, unreadable)
	if a.PropagationNodeServing(unreadable) {
		t.Error("PropagationNodeServing(no readable state) = true, want false")
	}
	if !a.PropagationNodeKnown(unreadable) {
		t.Error("PropagationNodeKnown(no readable state) = false, want true (it *is* announced)")
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

// TestPropagationNodeMarkerFor covers the reachability facts the transport can
// supply, plus the no-selection and no-transport cases. The all-known cases are
// covered by the pure table test above, which needs no transport.
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

	// Announced as serving, but no path known yet.
	rememberPropagationNodeAnnounce(t, ts, unknown, true)
	if got := a.propagationNodeMarkerFor(unknown); got != "no path" {
		t.Errorf("propagationNodeMarkerFor(no path) = %q, want %q", got, "no path")
	}

	// Announced with propagation switched off.
	stopped := bytes.Repeat([]byte{0x9f}, 16)
	rememberPropagationNodeAnnounce(t, ts, stopped, false)
	if got := a.propagationNodeMarkerFor(stopped); got != "stopped" {
		t.Errorf("propagationNodeMarkerFor(stopped) = %q, want %q", got, "stopped")
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

	// Known identity that declares itself serving, but no path: a different
	// marker from both the unknown and the stopped case.
	rememberPropagationNodeAnnounce(t, ts, dead, true)
	_, label = a.lastSyncInfoFor(dead)
	if !strings.Contains(label, "no path") {
		t.Errorf("lastSyncInfoFor(no path) label = %q, want it to carry the no-path marker", label)
	}

	// Declared, but the node says it is not serving propagation.
	stopped := bytes.Repeat([]byte{0xbf}, 16)
	rememberPropagationNodeAnnounce(t, ts, stopped, false)
	_, label = a.lastSyncInfoFor(stopped)
	if !strings.Contains(label, "stopped") {
		t.Errorf("lastSyncInfoFor(stopped) label = %q, want it to carry the stopped marker", label)
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
	for _, marker := range []string{
		propagationNodeMarker(true, true, false),
		propagationNodeMarker(false, false, true),
		propagationNodeMarker(true, false, true),
	} {
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
