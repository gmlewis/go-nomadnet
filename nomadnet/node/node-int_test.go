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

package node

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

func TestIntegrationNodeAnnounceReceivedByPeer(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	n := NewNode("IntegrationNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	announceReceived := make(chan []byte, 1)
	tsB.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter: "nomadnetwork.node",
		ReceivedAnnounceWithContext: func(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
			select {
			case announceReceived <- appData:
			default:
			}
		},
	})

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	select {
	case appData := <-announceReceived:
		if string(appData) != "IntegrationNode" {
			t.Errorf("appData = %q, want %q", string(appData), "IntegrationNode")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for node announce on peer")
	}
}

func newStartedTS(t *testing.T) (*rns.TransportSystem, func()) {
	t.Helper()
	dir, cleanup := testutils.TempDir(t, "nomadnet-node-int-ts")
	cfgDir := filepath.Join(dir, "config")
	writeRNSConfig(t, cfgDir)
	ts := rns.NewTransportSystem(nil)
	_, err := rns.NewReticulum(ts, cfgDir)
	if err != nil {
		cleanup()
		t.Fatalf("NewReticulum error: %v", err)
	}
	return ts, cleanup
}

func writeRNSConfig(t *testing.T, configDir string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `[reticulum]
share_instance = No

[logging]
loglevel = 4
`
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestPipes(t *testing.T, tsA, tsB *rns.TransportSystem) (*interfaces.PipeInterface, *interfaces.PipeInterface, func()) {
	t.Helper()
	pipeA := interfaces.NewPipeInterface("a", func(data []byte, iface interfaces.Interface) {
		tsA.Inbound(data, iface)
	})
	pipeB := interfaces.NewPipeInterface("b", func(data []byte, iface interfaces.Interface) {
		tsB.Inbound(data, iface)
	})
	pipeA.SetOther(pipeB)
	pipeB.SetOther(pipeA)
	cleanup := func() {
		_ = pipeA.Detach()
		_ = pipeB.Detach()
	}
	return pipeA, pipeB, cleanup
}

func tempDirInt(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nomadnet-node-int-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestIntegrationNodeServesDefaultIndexPage(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	n := NewNode("TestNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tsB.HasPath(n.Destination().Hash) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !tsB.HasPath(n.Destination().Hash) {
		t.Fatal("timeout waiting for path to node")
	}

	linkEstablished := make(chan *rns.Link, 1)
	n.Destination().SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case linkEstablished <- l:
		default:
		}
	})

	outDest, err := rns.NewDestination(tsB, n.identity, rns.DestinationOut, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("NewDestination error: %v", err)
	}

	link, err := rns.NewLink(tsB, outDest)
	if err != nil {
		t.Fatalf("NewLink error: %v", err)
	}
	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case linkEstablished <- l:
		default:
		}
	})
	if err := link.Establish(); err != nil {
		t.Fatalf("Establish error: %v", err)
	}

	var receiverLink *rns.Link
	select {
	case receiverLink = <-linkEstablished:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for link establishment")
	}

	responseCh := make(chan []byte, 1)
	_, err = link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
		if rr.Response != nil {
			if data, ok := rr.Response.([]byte); ok {
				select {
				case responseCh <- data:
				default:
				}
			}
		}
	}, func(rr *rns.RequestReceipt) {
		t.Logf("request failed")
	}, nil, 15*time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	select {
	case content := <-responseCh:
		if len(content) == 0 {
			t.Error("expected non-empty default index page content")
		}
		if string(content) != DefaultIndex {
			t.Errorf("content = %q, want default index", string(content)[:min(50, len(content))])
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for page request response")
	}

	_ = receiverLink
}

func TestIntegrationNodeServesCustomPage(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pageContent := ">Hello World\n\nThis is a test page.\n"
	if err := os.WriteFile(filepath.Join(pagesDir, "index.mu"), []byte(pageContent), 0o644); err != nil {
		t.Fatal(err)
	}

	n := NewNode("TestNode", pagesDir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tsB.HasPath(n.Destination().Hash) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !tsB.HasPath(n.Destination().Hash) {
		t.Fatal("timeout waiting for path to node")
	}

	linkEstablished := make(chan *rns.Link, 1)
	n.Destination().SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case linkEstablished <- l:
		default:
		}
	})

	outDest, err := rns.NewDestination(tsB, n.identity, rns.DestinationOut, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("NewDestination error: %v", err)
	}

	link, err := rns.NewLink(tsB, outDest)
	if err != nil {
		t.Fatalf("NewLink error: %v", err)
	}
	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case linkEstablished <- l:
		default:
		}
	})
	if err := link.Establish(); err != nil {
		t.Fatalf("Establish error: %v", err)
	}

	select {
	case <-linkEstablished:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for link establishment")
	}

	responseCh := make(chan []byte, 1)
	_, err = link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
		if rr.Response != nil {
			if data, ok := rr.Response.([]byte); ok {
				select {
				case responseCh <- data:
				default:
				}
			}
		}
	}, func(rr *rns.RequestReceipt) {
		t.Logf("request failed")
	}, nil, 15*time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	select {
	case content := <-responseCh:
		if string(content) != pageContent {
			t.Errorf("content = %q, want %q", string(content), pageContent)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for page request response")
	}
}

func TestIntegrationNodeAllowedFileRestrictsAccess(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	pagesDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pageContent := ">Secret Page\n\nThis page is restricted.\n"
	pagePath := filepath.Join(pagesDir, "secret.mu")
	if err := os.WriteFile(pagePath, []byte(pageContent), 0o644); err != nil {
		t.Fatal(err)
	}

	otherID, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatalf("NewIdentity error: %v", err)
	}
	allowedHash := fmt.Sprintf("%x", otherID.Hash)
	if err := os.WriteFile(pagePath+".allowed", []byte(allowedHash+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	n := NewNode("TestNode", pagesDir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tsB.HasPath(n.Destination().Hash) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !tsB.HasPath(n.Destination().Hash) {
		t.Fatal("timeout waiting for path to node")
	}

	linkEstablished := make(chan *rns.Link, 1)
	n.Destination().SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case linkEstablished <- l:
		default:
		}
	})

	outDest, err := rns.NewDestination(tsB, n.identity, rns.DestinationOut, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("NewDestination error: %v", err)
	}

	link, err := rns.NewLink(tsB, outDest)
	if err != nil {
		t.Fatalf("NewLink error: %v", err)
	}
	link.SetLinkEstablishedCallback(func(l *rns.Link) {
		select {
		case linkEstablished <- l:
		default:
		}
	})
	if err := link.Establish(); err != nil {
		t.Fatalf("Establish error: %v", err)
	}

	select {
	case <-linkEstablished:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for link establishment")
	}

	responseCh := make(chan []byte, 1)
	_, err = link.Request("/page/secret.mu", nil, func(rr *rns.RequestReceipt) {
		if rr.Response != nil {
			if data, ok := rr.Response.([]byte); ok {
				select {
				case responseCh <- data:
				default:
				}
			}
		}
	}, func(rr *rns.RequestReceipt) {
		select {
		case responseCh <- nil:
		default:
		}
	}, nil, 15*time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	select {
	case content := <-responseCh:
		if string(content) == DefaultNotAllowed {
			t.Log("access correctly denied for unlisted identity")
		} else if string(content) == pageContent {
			t.Error("access should be denied but page was served")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for page request response")
	}
}
