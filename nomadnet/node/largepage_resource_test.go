//go:build integration

package node

import (
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestIntegrationLargePageResourceTransfer mirrors
// TestIntegrationBrowserRequestsPageReceivesMicron but with a page larger than
// the link MDU, forcing the response to be transferred as an RNS resource
// (advertise + part fetch) rather than a single ContextResponse packet. It
// guards the resource-transfer path over a direct (pipe) interface, complement
// of TestLoopbackBrowseViaSharedInstance which guards it over the
// shared-instance local interface.
func TestIntegrationLargePageResourceTransfer(t *testing.T) {
	tsA, cleanupA := newStartedTS(t)
	defer cleanupA()
	tsB, cleanupB := newStartedTS(t)
	defer cleanupB()

	pipeA, pipeB, pipeCleanup := newTestPipes(t, tsA, tsB)
	defer pipeCleanup()
	tsA.RegisterInterface(pipeA)
	tsB.RegisterInterface(pipeB)

	dir := tempDirInt(t)
	// ~100KB page, well over any single-packet MDU.
	writeFile(t, dir+"/index.mu", ">> Big\n\n"+strings.Repeat("x", 100000)+"\nEND\n")
	n := NewNode("BigNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsA, tsA.Identity()); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
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
	failCh := make(chan struct{}, 1)
	_, err = link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
		if data, ok := rr.Response.([]byte); ok {
			select {
			case responseCh <- data:
			default:
			}
		} else {
			select {
			case failCh <- struct{}{}:
			default:
			}
		}
	}, func(rr *rns.RequestReceipt) {
		t.Logf("request failed callback, status=%v", rr.Status)
		select {
		case failCh <- struct{}{}:
		default:
		}
	}, nil, 30*time.Second)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	select {
	case content := <-responseCh:
		t.Logf("got %d bytes", len(content))
	case <-failCh:
		t.Fatal("request failed callback fired")
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for large page response (resource transfer)")
	}
}
