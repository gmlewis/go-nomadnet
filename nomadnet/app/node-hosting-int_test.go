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

//go:build integration

package app

import (
	"bytes"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestAppNodeHostingStartsAndAnnounces verifies the node-hosting wiring:
// App.startNode instantiates and starts a nomadnet/node.Node on the app's
// transport, the node's destination is the "nomadnetwork.node" hash of the
// app identity (mirroring Python NomadNetworkApp.py:399 self.node =
// nomadnet.Node(self) → Node.py:18), and a node announce propagates to a peer
// app where handleNodeAnnounce records it with the node name as app data.
// Shutdown stops the node job loop (ShouldRunJobs false), mirroring the
// Python exit_handler.
func TestAppNodeHostingStartsAndAnnounces(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	// Node hosting is off in the default test config; turn it on for appA and
	// start the node explicitly (announce-at-start disabled so the test stays
	// deterministic — we announce manually below).
	appA.EnableNode = true
	appA.NodeName = "AppHostedNode"
	appA.NodeAnnounceAtStart = false
	if err := appA.startNode(); err != nil {
		t.Fatalf("startNode: %v", err)
	}

	if appA.Node == nil {
		t.Fatal("appA.Node is nil after startNode")
	}
	if appA.Node.Destination() == nil {
		t.Fatal("node destination is nil")
	}
	// The node destination is the "nomadnetwork.node" aspect of the identity.
	wantHash := rns.CalculateHash(appA.Identity, "nomadnetwork", "node")
	if !bytes.Equal(appA.Node.Destination().Hash, wantHash) {
		t.Errorf("node hash = %x, want %x", appA.Node.Destination().Hash, wantHash)
	}
	if appA.Node.Name != "AppHostedNode" {
		t.Errorf("node name = %q, want %q", appA.Node.Name, "AppHostedNode")
	}

	// The node's job loop should be running.
	if !appA.Node.ShouldRunJobs {
		t.Error("node ShouldRunJobs = false, want true (job loop should run)")
	}

	// Announce the node; appB's handleNodeAnnounce should record it.
	if err := appA.Node.Announce(); err != nil {
		t.Fatalf("Node.Announce: %v", err)
	}
	waitForAnnounce(t, appB, "node", 5*time.Second)

	// The recorded node announce carries the node name as app data.
	var found bool
	for _, ev := range appB.DirAnnounceEvents() {
		if ev.AnnounceType == "node" {
			if string(ev.AppData) != "AppHostedNode" {
				t.Errorf("node announce appdata = %q, want %q", string(ev.AppData), "AppHostedNode")
			}
			found = true
		}
	}
	if !found {
		t.Error("no node announce recorded on appB")
	}

	// Shutdown stops the node job loop.
	appA.Shutdown()
	if appA.Node.ShouldRunJobs {
		t.Error("node ShouldRunJobs = true after Shutdown, want false")
	}
}

// TestAppNodeHostingDisabled verifies startNode is a no-op when EnableNode is
// false (the default), leaving App.Node nil — mirroring Python's
// NomadNetworkApp.py:401 self.node = None branch.
func TestAppNodeHostingDisabled(t *testing.T) {
	appA, _, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	appA.EnableNode = false
	if err := appA.startNode(); err != nil {
		t.Fatalf("startNode: %v", err)
	}
	if appA.Node != nil {
		t.Error("appA.Node is non-nil when EnableNode is false")
	}
}

// TestAppNodeHostingNameFallback verifies the Python Node.py:35-36 fallback:
// when node_name is unset, the node name becomes
// "<peer display name>'s Node".
func TestAppNodeHostingNameFallback(t *testing.T) {
	appA, _, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	appA.EnableNode = true
	appA.NodeName = ""
	appA.NodeAnnounceAtStart = false
	if appA.PeerSettings == nil {
		t.Fatal("appA.PeerSettings is nil")
	}
	appA.PeerSettings.DisplayName = "Anonymous Peer"
	if err := appA.startNode(); err != nil {
		t.Fatalf("startNode: %v", err)
	}
	if want := "Anonymous Peer's Node"; appA.Node.Name != want {
		t.Errorf("node name = %q, want %q", appA.Node.Name, want)
	}
	appA.Shutdown()
}
