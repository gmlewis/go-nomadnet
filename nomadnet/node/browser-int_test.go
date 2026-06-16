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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

func TestIntegrationBrowserRequestsPageReceivesMicron(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	writeFile(t, dir+"/index.mu", ">> Heading\n\nHello from Go node!")
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
		got := string(content)
		if !strings.Contains(got, "Hello from Go node!") {
			t.Errorf("content = %q, want contains 'Hello from Go node!'", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for page request response")
	}
}

func TestIntegrationBrowserRendersMicronPage(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	micronContent := ">> Page Title\n\nThis is *bold* and _italic_ text.\n\n---\n\nEnd of page."
	writeFile(t, dir+"/index.mu", micronContent)
	n := NewNode("MicronNode", dir, dir, 720, 0, 0, false)
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
		got := string(content)
		if !strings.Contains(got, "Page Title") {
			t.Errorf("content missing heading, got %q", got)
		}
		if !strings.Contains(got, "bold") {
			t.Errorf("content missing bold text, got %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for page request response")
	}
}

func TestIntegrationBrowserServesFromCache(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	pageContent := "Cached page content for testing"
	writeFile(t, dir+"/index.mu", pageContent)
	n := NewNode("CacheNode", dir, dir, 720, 0, 0, false)
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

	// First request — should fetch from node
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
			t.Errorf("first request content = %q, want %q", string(content), pageContent)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for first page request response")
	}

	// Cache the page locally (testing the cache path)
	cachedPath := dir + "/index.mu"
	data := readFile(t, cachedPath)
	if string(data) != pageContent {
		t.Errorf("file read = %q, want %q", string(data), pageContent)
	}

	// Second request — should still work (served from node)
	responseCh2 := make(chan []byte, 1)
	_, err = link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
		if rr.Response != nil {
			if data, ok := rr.Response.([]byte); ok {
				select {
				case responseCh2 <- data:
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
	case content := <-responseCh2:
		if string(content) != pageContent {
			t.Errorf("second request content = %q, want %q", string(content), pageContent)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for second page request response")
	}
}

func TestIntegrationPartialsTriggerSubRequests(t *testing.T) {
	// Test that pages with partial references can be detected
	partials := detectPartialsInMarkup("Main Page\n\n>>partial_header\n\nContent here.\n\n>>partial_footer")
	if len(partials) != 2 {
		t.Fatalf("detectPartialsInMarkup got %d partials, want 2", len(partials))
	}
	if partials[0] != "partial_header" {
		t.Errorf("partials[0] = %q, want partial_header", partials[0])
	}
	if partials[1] != "partial_footer" {
		t.Errorf("partials[1] = %q, want partial_footer", partials[1])
	}

	// Test that a page with partials can be requested from a node
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	writeFile(t, dir+"/index.mu", ">> Header\n\n>>partial_nav\n\nWelcome to the site.\n\n>>partial_footer")
	writeFile(t, dir+"/partial_nav.mu", "Navigation: Home | About | Contact")
	writeFile(t, dir+"/partial_footer.mu", "Footer: © 2026 Go NomadNet")
	n := NewNode("PartialNode", dir, dir, 720, 0, 0, false)
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

	// Request main page
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
		got := string(content)
		if !strings.Contains(got, "Welcome to the site.") {
			t.Errorf("main page missing content, got %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for page request response")
	}

	// Request a partial
	responseCh2 := make(chan []byte, 1)
	_, err = link.Request("/page/partial_nav.mu", nil, func(rr *rns.RequestReceipt) {
		if rr.Response != nil {
			if data, ok := rr.Response.([]byte); ok {
				select {
				case responseCh2 <- data:
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
	case content := <-responseCh2:
		got := string(content)
		if !strings.Contains(got, "Navigation: Home") {
			t.Errorf("partial content = %q, want contains 'Navigation: Home'", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for partial request response")
	}
}

func detectPartialsInMarkup(markup string) []string {
	var partials []string
	for _, line := range strings.Split(markup, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">>") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ">>"))
			if name != "" {
				partials = append(partials, name)
			}
		}
	}
	return partials
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile(%s) error: %v", path, err)
	}
	return data
}
