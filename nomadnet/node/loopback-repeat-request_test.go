//go:build integration

package node

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestLoopbackRepeatRequestOnSameLink reproduces the stress-test-nomadnet
// "link answers a few requests then dies" symptom on a loopback shared-instance
// interface (the production path for a gonomadnet client talking to its own
// shared instance's node). It establishes ONE link and fires N sequential
// page requests over it, reporting which succeed/fail and how long each took.
//
// The nomadnet server never tears a link down after serving (verified: no
// Teardown in the page/file handlers or the resource path), and this test does
// not call Teardown between requests — so every request should succeed. A
// failure or timeout after the first request reproduces the go-reticulum
// link-dies-after-N-requests bug.
func TestLoopbackRepeatRequestOnSameLink(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	logger := rns.NewLogger()
	logger.SetLogLevel(rns.LogError)
	if os.Getenv("RNS_TEST_VERBOSE") != "" {
		logger.SetLogLevel(rns.LogDebug)
	}

	// Shared instance S.
	cfgDirS := testutils.TempDir(t, "repeat-S")
	writeRNSConfigRaw(t, cfgDirS, "No", "4")
	tsS := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulum(tsS, cfgDirS); err != nil {
		t.Fatalf("NewReticulum S: %v", err)
	}
	t.Cleanup(func() { tsS.Stop() })

	// Client C.
	cfgDirC := testutils.TempDir(t, "repeat-C")
	writeRNSConfigRaw(t, cfgDirC, "No", "4")
	tsC := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulum(tsC, cfgDirC); err != nil {
		t.Fatalf("NewReticulum C: %v", err)
	}
	t.Cleanup(func() { tsC.Stop() })
	tsC.SetConnectedToSharedInstance(true)

	port := testutils.ReserveTCPPort(t)
	server, err := interfaces.NewLocalServerInterface("Local shared instance", "", port, func(data []byte, iface interfaces.Interface) {
		tsS.Inbound(data, iface)
	})
	if err != nil {
		t.Fatalf("NewLocalServerInterface: %v", err)
	}
	tsS.RegisterInterface(server)
	t.Cleanup(func() { _ = server.Detach() })

	client, err := interfaces.NewLocalClientInterface("Local shared instance", "", port, func(data []byte, iface interfaces.Interface) {
		tsC.Inbound(data, iface)
	})
	if err != nil {
		t.Fatalf("NewLocalClientInterface: %v", err)
	}
	tsC.RegisterInterface(client)
	t.Cleanup(func() { _ = client.Detach() })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !client.Status() {
		time.Sleep(20 * time.Millisecond)
	}
	if !client.Status() {
		t.Fatalf("local client never connected to shared instance")
	}

	// Node on S. Page size is configurable so we can compare inline vs
	// resource-transfer responses.
	pageSize := 200
	if s := os.Getenv("RNS_TEST_PAGE_BYTES"); s != "" {
		fmt.Sscanf(s, "%d", &pageSize)
	}
	dir := tempDirInt(t)
	writeFile(t, dir+"/index.mu", ">> Repeat\n\n"+strings.Repeat("x", pageSize)+"\nEND\n")
	n := NewNode("RepeatNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsS, tsS.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	t.Cleanup(n.Stop)
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash

	if err := tsC.RequestPath(nodeHash); err != nil {
		t.Fatalf("RequestPath: %v", err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if tsC.HasPath(nodeHash) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !tsC.HasPath(nodeHash) {
		t.Fatalf("client never learned path to S's own node destination")
	}

	outDest, err := rns.NewDestination(tsC, n.identity, rns.DestinationOut, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("NewDestination: %v", err)
	}
	link, err := rns.NewLink(tsC, outDest)
	if err != nil {
		t.Fatalf("NewLink: %v", err)
	}
	closed := make(chan struct{}, 1)
	established := make(chan struct{}, 1)
	link.SetLinkEstablishedCallback(func(*rns.Link) {
		select {
		case established <- struct{}{}:
		default:
		}
	})
	link.SetLinkClosedCallback(func(*rns.Link) {
		select {
		case closed <- struct{}{}:
		default:
		}
	})
	if err := link.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	select {
	case <-established:
		t.Logf("link established")
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for link establishment")
	}

	const nreq = 8
	reqTimeout := 30 * time.Second
	for i := 1; i <= nreq; i++ {
		got := make(chan struct {
			bytes int
			fail  string
		}, 2)
		start := time.Now()
		_, err := link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
			if data, ok := rr.Response.([]byte); ok {
				select {
				case got <- struct {
					bytes int
					fail  string
				}{bytes: len(data)}:
				default:
				}
			} else {
				select {
				case got <- struct {
					bytes int
					fail  string
				}{fail: fmt.Sprintf("status=%v", rr.Status)}:
				default:
				}
			}
		}, func(rr *rns.RequestReceipt) {
			select {
			case got <- struct {
				bytes int
				fail  string
			}{fail: fmt.Sprintf("failed status=%v", rr.Status)}:
			default:
			}
		}, nil, reqTimeout, 0)
		if err != nil {
			t.Fatalf("req %d: Request error: %v", i, err)
		}
		select {
		case r := <-got:
			if r.fail != "" {
				t.Fatalf("req %d: %s (after %v)", i, r.fail, time.Since(start))
			}
			t.Logf("req %d: OK %d bytes in %v", i, r.bytes, time.Since(start))
		case <-closed:
			t.Fatalf("req %d: link CLOSED after %v", i, time.Since(start))
		case <-time.After(reqTimeout + 2*time.Second):
			t.Fatalf("req %d: TIMEOUT after %v", i, time.Since(start))
		}
	}
	t.Logf("all %d requests succeeded on one link", nreq)
}
