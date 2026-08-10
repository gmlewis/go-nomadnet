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

// TestLoopbackBrowseViaSharedInstance guards the go-reticulum fix for
// resource transfers (large page responses) over a shared-instance local
// interface. A client C connected to a shared Reticulum instance S links to
// S's OWN nomadnetwork.node destination (loopback) and requests /page/index.mu.
//
// Before the fix, the link handshake completed but the page request timed out:
// the resource advertise packet is built with NewPacket(link, ...) and sent via
// Packet.Send() (Transport.Outbound), not Link.send, so it reached Outbound
// with AttachedInterface unset. Outbound's broadcast branch then handed the
// packet to the LocalServerInterface listener, whose Send is a no-op (the real
// socket lives on a spawned LocalClientInterface child that is NOT in
// ts.interfaces) — silently dropping the advertise. The fix resolves the
// attached interface from the local link at the top of Outbound so link packets
// route directly via it, mirroring Python where Link.send stamps
// packet.attached_interface.
func TestLoopbackBrowseViaSharedInstance(t *testing.T) {
	testutils.SkipShortIntegration(t)

	logger := rns.NewLogger()
	logger.SetLogLevel(rns.LogError)
	if os.Getenv("RNS_TEST_VERBOSE") != "" {
		logger.SetLogLevel(rns.LogDebug)
	}

	// Shared instance S (standalone config so it gets an identity; we add the
	// local server interface manually on a reserved port, as the production
	// shared instance does).
	cfgDirS := testutils.TempDir(t, "loopback-S")
	writeRNSConfigRaw(t, cfgDirS, "No", "4")
	tsS := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulum(tsS, cfgDirS); err != nil {
		t.Fatalf("NewReticulum S: %v", err)
	}
	t.Cleanup(func() { tsS.Stop() })

	// Client C.
	cfgDirC := testutils.TempDir(t, "loopback-C")
	writeRNSConfigRaw(t, cfgDirC, "No", "4")
	tsC := rns.NewTransportSystem(logger)
	if _, err := rns.NewReticulum(tsC, cfgDirC); err != nil {
		t.Fatalf("NewReticulum C: %v", err)
	}
	t.Cleanup(func() { tsC.Stop() })
	tsC.SetConnectedToSharedInstance(true)

	// IPC link S<->C over a real TCP local interface (production path).
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

	// Node on S serving a LARGE page (>MDU), forcing a resource transfer over
	// the shared-instance local interface — the production failure condition.
	dir := tempDirInt(t)
	writeFile(t, dir+"/index.mu", ">> Loopback\n\nHello loopback! "+strings.Repeat("x", 100000)+"\nEND\n")
	n := NewNode("LoopbackNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsS, tsS.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	t.Cleanup(n.Stop)
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash
	t.Logf("node nomadnetwork.node hash=%x identity=%x", nodeHash, n.identity.Hash)

	// C learns a path to S's own node destination. S does not forward its own
	// announce to local clients, so (as in production with --identity) C must
	// issue a path request that S answers because it owns the destination.
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

	// C links to S's own node destination (loopback) and requests the page.
	outDest, err := rns.NewDestination(tsC, n.identity, rns.DestinationOut, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("NewDestination: %v", err)
	}
	link, err := rns.NewLink(tsC, outDest)
	if err != nil {
		t.Fatalf("NewLink: %v", err)
	}
	established := make(chan struct{}, 1)
	link.SetLinkEstablishedCallback(func(*rns.Link) { established <- struct{}{} })
	if err := link.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	select {
	case <-established:
		t.Logf("link established (loopback)")
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for loopback link establishment")
	}

	respCh := make(chan []byte, 1)
	failCh := make(chan string, 1)
	_, err = link.Request("/page/index.mu", nil, func(rr *rns.RequestReceipt) {
		if data, ok := rr.Response.([]byte); ok {
			select {
			case respCh <- data:
			default:
			}
		} else {
			select {
			case failCh <- fmt.Sprintf("response callback, Response=%v status=%v", rr.Response, rr.Status):
			default:
			}
		}
	}, func(rr *rns.RequestReceipt) {
		select {
		case failCh <- fmt.Sprintf("failed callback status=%v", rr.Status):
		default:
		}
	}, nil, 30*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	select {
	case data := <-respCh:
		t.Logf("got %d bytes", len(data))
	case msg := <-failCh:
		t.Fatalf("request failed: %s", msg)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for loopback page response")
	}
}

func writeRNSConfigRaw(t *testing.T, configDir, share, loglevel string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[reticulum]\nshare_instance = " + share + "\n\n[logging]\nloglevel = " + loglevel + "\n"
	if err := os.WriteFile(configDir+"/config", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
