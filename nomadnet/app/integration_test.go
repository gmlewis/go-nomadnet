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

	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

func TestIntegrationSetupTwoNodeApps(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	if appA.Router == nil {
		t.Error("appA Router is nil")
	}
	if appB.Router == nil {
		t.Error("appB Router is nil")
	}
	if appA.LXMFDest == nil {
		t.Error("appA LXMFDest is nil")
	}
	if appB.LXMFDest == nil {
		t.Error("appB LXMFDest is nil")
	}
}

func TestIntegrationLXMFMessageSendReceive(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A error: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B error: %v", err)
	}

	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	destB := appB.LXMFDest

	msg, err := lxmf.NewMessage(destB, appA.LXMFDest, "hello from A", "test title", nil)
	if err != nil {
		t.Fatalf("NewMessage error: %v", err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	if err := appA.Router.HandleOutbound(msg); err != nil {
		t.Fatalf("HandleOutbound error: %v", err)
	}

	got := waitForLXMFMessage(t, receivedCh, 30*time.Second)

	if got.TitleString() != "test title" {
		t.Errorf("title = %q, want %q", got.TitleString(), "test title")
	}
	if got.ContentString() != "hello from A" {
		t.Errorf("content = %q, want %q", got.ContentString(), "hello from A")
	}
}

func TestIntegrationLXMFMessageCreatesConversation(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	receivedCh := make(chan *lxmf.Message, 1)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	destB := appB.LXMFDest

	msg, err := lxmf.NewMessage(destB, appA.LXMFDest, "msg1", "title1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	if err := appA.Router.HandleOutbound(msg); err != nil {
		t.Fatal(err)
	}

	waitForLXMFMessage(t, receivedCh, 10*time.Second)

	list := appB.ConversationList()
	if len(list) == 0 {
		t.Error("expected at least one conversation after message delivery")
	}
}

func TestIntegrationNodeAnnounceReceivedByPeer(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	nodeDest, err := rns.NewDestination(appA.Transport, appA.Identity, rns.DestinationIn, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("node dest error: %v", err)
	}

	if err := nodeDest.Announce([]byte("TestNode")); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	waitForAnnounce(t, appB, "node", 5*time.Second)
}

func TestIntegrationLXMFAnnouncePopulatesDirectory(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	waitForAnnounce(t, appB, "peer", 5*time.Second)

	peerAnnounces := appB.Dir.PeerAnnounces()
	if len(peerAnnounces) == 0 {
		t.Fatal("expected at least one peer announce in directory after LXMF announce")
	}

	found := false
	for _, a := range peerAnnounces {
		if a.AnnounceType == "peer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected peer announce type in directory peer announces")
	}
}

func TestIntegrationNodeAnnouncePopulatesDirectory(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	nodeDest, err := rns.NewDestination(appA.Transport, appA.Identity, rns.DestinationIn, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("node dest error: %v", err)
	}

	if err := nodeDest.Announce([]byte("MyNode")); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	waitForAnnounce(t, appB, "node", 5*time.Second)

	nodeAnnounces := appB.Dir.NodeAnnounces()
	if len(nodeAnnounces) == 0 {
		t.Fatal("expected at least one node announce in directory after node announce")
	}

	found := false
	for _, a := range nodeAnnounces {
		if a.AnnounceType == "node" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected node announce type in directory node announces")
	}
}

func TestIntegrationLXMFAnnouncePopulatesDirectoryName(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	waitForAnnounce(t, appB, "peer", 5*time.Second)

	peerAnnounces := appB.Dir.PeerAnnounces()
	if len(peerAnnounces) == 0 {
		t.Fatal("expected at least one peer announce")
	}

	appData := peerAnnounces[0].AppData
	if len(appData) == 0 {
		t.Error("expected app_data in peer announce, got empty")
	}
}

func TestIntegrationAnnounceStreamOrder(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("LXMF Announce error: %v", err)
	}
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	nodeDest, err := rns.NewDestination(appA.Transport, appA.Identity, rns.DestinationIn, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("node dest error: %v", err)
	}
	if err := nodeDest.Announce([]byte("NodeA")); err != nil {
		t.Fatalf("Node Announce error: %v", err)
	}
	waitForAnnounce(t, appB, "node", 5*time.Second)

	peerAnnounces := appB.Dir.PeerAnnounces()
	nodeAnnounces := appB.Dir.NodeAnnounces()

	if len(peerAnnounces) == 0 {
		t.Error("expected peer announces in directory")
	}
	if len(nodeAnnounces) == 0 {
		t.Error("expected node announces in directory")
	}

	for _, a := range peerAnnounces {
		if a.AnnounceType != "peer" {
			t.Errorf("peer announce has wrong type: %q", a.AnnounceType)
		}
	}
	for _, a := range nodeAnnounces {
		if a.AnnounceType != "node" {
			t.Errorf("node announce has wrong type: %q", a.AnnounceType)
		}
	}
}

func TestIntegrationNodeAnnounceCreatesKnownNodeForTrustedPeer(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	peerHash := rns.CalculateHash(appA.Identity, "lxmf", "delivery")
	entry := directory.NewEntry(peerHash)
	entry.DisplayName = "TrustedPeer"
	entry.TrustLevel = directory.TrustTrusted
	appB.Dir.Remember(entry)

	nodeDest, err := rns.NewDestination(appA.Transport, appA.Identity, rns.DestinationIn, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("node dest error: %v", err)
	}

	if err := nodeDest.Announce([]byte("TrustedNode")); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	waitForAnnounce(t, appB, "node", 5*time.Second)

	knownNodes := appB.Dir.KnownNodes()
	if len(knownNodes) == 0 {
		t.Fatal("expected at least one known node after trusted peer announces node")
	}

	nodeHash := rns.CalculateHash(appA.Identity, "nomadnetwork", "node")
	found := false
	for _, n := range knownNodes {
		if bytes.Equal(n.SourceHash, nodeHash) {
			found = true
			if !n.HostsNode {
				t.Error("known node entry should have HostsNode=true")
			}
			if n.DisplayName != "TrustedNode" {
				t.Errorf("display name = %q, want %q", n.DisplayName, "TrustedNode")
			}
			break
		}
	}
	if !found {
		t.Error("node hash not found in known nodes")
	}
}

func TestIntegrationPNAnnounceReceivedByPeer(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	_, err := appA.Router.RegisterPropagationDestination()
	if err != nil {
		t.Fatalf("RegisterPropagationDestination error: %v", err)
	}
	appA.Router.EnablePropagation()
	appA.Router.AnnouncePropagationNode()

	waitForAnnounce(t, appB, "pn", 5*time.Second)

	pnAnnounces := appB.Dir.PNAnnounces()
	if len(pnAnnounces) == 0 {
		t.Fatal("expected at least one PN announce in directory after propagation node announce")
	}

	found := false
	for _, a := range pnAnnounces {
		if a.AnnounceType == "pn" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pn announce type in directory PN announces")
	}
}
