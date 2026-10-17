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

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// These tests pin the Python-parity propagation declaration: a NomadNet instance
// tells the network about its own lxmf.propagation address, so the "LXMF
// Propagation Node Address is <hex>" line a peer's node Info page shows
// (Network.py:629-634) names a destination that exists. Python does it in two
// places — Node.announce declares on every node announce (Node.py:216-220), and
// startup applies [node] disable_propagation to the router
// (NomadNetworkApp.py:378-396) — and the port must do both.

// newTestAppWithRouter builds an initialized app with a live (offline) LXMF
// router, the minimum needed to exercise the propagation wiring.
func newTestAppWithRouter(t *testing.T) *App {
	t.Helper()
	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	t.Cleanup(a.Shutdown)
	if a.Router == nil {
		t.Fatal("precondition: no LXMF router")
	}
	return a
}

// propagationNodeState reads the node_state of the payload this app's router
// declares for its own propagation address — the payload an announce carries
// (the destination's default app data, which implicit re-announces use).
func propagationNodeState(t *testing.T, a *App) (serving, ok bool) {
	t.Helper()
	dest := a.Router.PropagationDestination()
	if dest == nil {
		t.Fatal("router has no propagation destination")
	}
	return lxmf.PropagationNodeStateFromAppData(dest.DefaultAppData())
}

// TestPropagationDestinationExistsFromStartup pins Python's LXMRouter.__init__
// (LXMRouter.py:188-191): the propagation destination exists as soon as the
// router does, so any instance — client or propagation node — can declare the
// address. The port created it only when serving, which is why a NomadNet node
// could derive a propagation address that nothing had ever announced.
func TestPropagationDestinationExistsFromStartup(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)

	if a.Router.PropagationDestination() == nil {
		t.Fatal("PropagationDestination() = nil, want the destination to exist from construction")
	}
	want := rns.CalculateHash(a.Identity, "lxmf", "propagation")
	if got := a.Router.PropagationDestinationHash(); string(got) != string(want) {
		t.Errorf("PropagationDestinationHash() = %x, want the identity's lxmf.propagation address %x", got, want)
	}
	if _, ok := propagationNodeState(t, a); !ok {
		t.Error("the propagation destination carries no readable declaration payload")
	}
}

// TestApplyPropagationConfigDisabledWithoutMarkerIsSilent pins the branch
// NomadNetworkApp.py:379-385 takes for an instance that never announced
// propagation state: with disable_propagation set and no marker file, nothing is
// announced (there is nothing to withdraw) and no marker is written.
func TestApplyPropagationConfigDisabledWithoutMarkerIsSilent(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)
	a.DisablePropagation = true

	if _, err := os.Stat(a.Storage.PNAnnounced); !os.IsNotExist(err) {
		t.Fatalf("precondition: marker %v should not exist yet (%v)", a.Storage.PNAnnounced, err)
	}

	a.applyPropagationConfig()

	if _, err := os.Stat(a.Storage.PNAnnounced); !os.IsNotExist(err) {
		t.Errorf("marker created by a disabled startup with no prior declaration (%v)", err)
	}
	if a.Router.PropagationEnabled() {
		t.Error("PropagationEnabled() = true after a disabled startup, want false")
	}
}

// TestApplyPropagationConfigDisabledWithMarkerWithdraws pins
// NomadNetworkApp.py:379-385: when the marker file is present the instance
// declares the withdrawal (LXMRouter.disable_propagation announces node_state
// false) and removes the marker.
func TestApplyPropagationConfigDisabledWithMarkerWithdraws(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)
	a.DisablePropagation = true

	// The marker records that this instance has announced propagation state, so
	// its peers hold a declaration that now needs withdrawing.
	if err := os.WriteFile(a.Storage.PNAnnounced, []byte{0xcb, 0, 0, 0, 0, 0, 0, 0, 0}, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	a.applyPropagationConfig()

	if _, err := os.Stat(a.Storage.PNAnnounced); !os.IsNotExist(err) {
		t.Errorf("marker still present after withdrawing the declaration (%v)", err)
	}
	if a.Router.PropagationEnabled() {
		t.Error("PropagationEnabled() = true after the withdrawal, want false")
	}
	serving, ok := propagationNodeState(t, a)
	if !ok {
		t.Error("the withdrawal declares no readable node-state element")
	}
	if serving {
		t.Error("node_state = true after the withdrawal, want false")
	}
}

// TestApplyPropagationConfigEnabledServesAndMarks pins the else branch of
// NomadNetworkApp.py:387-396: an instance with propagation enabled starts
// serving, declares node_state true, and records the marker so a later switch to
// disabled announces the withdrawal.
func TestApplyPropagationConfigEnabledServesAndMarks(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)
	a.DisablePropagation = false

	a.applyPropagationConfig()

	if !a.Router.PropagationEnabled() {
		t.Error("PropagationEnabled() = false after an enabled startup, want true")
	}
	serving, ok := propagationNodeState(t, a)
	if !ok {
		t.Error("the enabled router declares no readable node-state element")
	}
	if !serving {
		t.Error("node_state = false for an enabled router, want true")
	}
	if _, err := os.Stat(a.Storage.PNAnnounced); err != nil {
		t.Errorf("marker %v not written by an enabled startup: %v", a.Storage.PNAnnounced, err)
	}
}

// TestStartNodeWiresPropagationDeclaration pins the wiring that makes every node
// announce declare the instance's propagation address (Python Node.py:217 calls
// the app's message router). Without it the node announce carries only the node
// name and the address a peer derives stays undeclared.
func TestStartNodeWiresPropagationDeclaration(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)
	a.EnableNode = true
	a.NodeName = "Declaring Node"

	if err := a.startNode(); err != nil {
		t.Fatalf("startNode: %v", err)
	}
	if a.Node == nil {
		t.Fatal("startNode did not create a node")
	}
	if a.Node.OnAnnouncePropagationNode == nil {
		t.Fatal("node has no propagation declaration hook, want a.declarePropagationNodeAddress")
	}

	// The declaration also runs on the node's own announce path, and must be safe
	// with no router (daemon/CLI paths that never create one).
	if err := a.Node.Announce(); err != nil {
		t.Fatalf("node announce with the declaration hook: %v", err)
	}
	a.Router = nil
	a.Node.OnAnnouncePropagationNode()
}

// TestNodePropagationNotServing pins the predicate behind the Info page's
// display rule: a node whose own identity declared its propagation address as
// not serving must be reported so the form can drop the address line and the
// default-PN checkbox, rather than offering an address that cannot be used as a
// propagation node. Only a positive declaration counts — an address nobody
// declared is "unknown", not "not serving", and keeps the SoT display.
func TestNodePropagationNotServing(t *testing.T) {
	t.Parallel()

	a, ts := testAppWithTransport(t)

	// Unknown node: no identity, nothing declared.
	unknown := bytes.Repeat([]byte{0xc1}, 16)
	if a.NodePropagationNotServing(unknown) {
		t.Error("NodePropagationNotServing(unknown node) = true, want false")
	}

	// Node whose identity announced a *serving* propagation address: shown.
	servingNode := bytes.Repeat([]byte{0xc2}, 16)
	servingID := rememberDestination(t, ts, servingNode)
	rememberPropagationNodeAnnounce(t, ts, rns.CalculateHash(servingID, "lxmf", "propagation"), true)
	if a.NodePropagationNotServing(servingNode) {
		t.Error("NodePropagationNotServing(serving node) = true, want false")
	}

	// Node whose identity declared the address as not serving (the fleet's shape:
	// the node announces its own propagation address with node_state false).
	stoppedNode := bytes.Repeat([]byte{0xc3}, 16)
	stoppedID := rememberDestination(t, ts, stoppedNode)
	rememberPropagationNodeAnnounce(t, ts, rns.CalculateHash(stoppedID, "lxmf", "propagation"), false)
	if !a.NodePropagationNotServing(stoppedNode) {
		t.Error("NodePropagationNotServing(declared not serving) = false, want true")
	}

	// Node whose identity never declared anything: unknown, so the derived address
	// is still shown (with the adoption guard refusing it if it is not served).
	silentNode := bytes.Repeat([]byte{0xc4}, 16)
	rememberDestination(t, ts, silentNode)
	if a.NodePropagationNotServing(silentNode) {
		t.Error("NodePropagationNotServing(no declaration) = true, want false (unknown is not 'not serving')")
	}
}
