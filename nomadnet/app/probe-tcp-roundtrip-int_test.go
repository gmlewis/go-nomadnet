// Copyright 2026 Glenn Lewis. All rights reserved.

//go:build integration

// probe-tcp-roundtrip-int_test.go is a diagnostic for the cmd/test-conversations
// harness: it reproduces the harness's EXACT production RNS path — two App
// instances, each booted via Init() -> async initRNS() -> NewReticulumWithLogger
// loading a private rnsconfig (A = TCP server, B = TCP client), announce-at-start
// firing on its own — but WITHOUT the TUI/tmux layer, so a failure here isolates
// the bug to the RNS/app layer (announce propagation, path, MethodDirect delivery)
// rather than the TUI send path (C-d not firing, editor text false-positive).
//
// Run: GOCACHE=/tmp/go-cache go test -tags=integration -run TestIntegrationTwoAppTCPRoundTrip -v ./nomadnet/app/
package app

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// writeProbeRNSServerConfig mirrors cmd/test-conversations/config.go
// writeRNSConfigServer: a standalone RNS config hosting a TCP server interface.
func writeProbeRNSServerConfig(t *testing.T, dir string, port int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`[reticulum]
  share_instance = No

[logging]
  loglevel = 4

[interfaces]

  [[TCP Server Interface]]
    type = TCPServerInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = %v
`, port)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeProbeRNSClientConfig mirrors cmd/test-conversations/config.go
// writeRNSConfigClient: a standalone RNS config connecting a TCP client.
func writeProbeRNSClientConfig(t *testing.T, dir string, port int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`[reticulum]
  share_instance = No

[logging]
  loglevel = 4

[interfaces]

  [[TCP Client Interface]]
    type = TCPClientInterface
    enabled = yes
    target_host = 127.0.0.1
    target_port = %v
`, port)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationTwoAppTCPRoundTrip verifies the production initRNS path delivers
// an LXMF message from A to B over a localhost TCP RNS link configured entirely
// via rnsconfig (no manual interface registration), exactly like the
// cmd/test-conversations harness. A failure here means the harness's
// cross-instance delivery gap is in the RNS/app layer, not the TUI.
func TestIntegrationTwoAppTCPRoundTrip(t *testing.T) {
	testutils.SkipShortIntegration(t)

	port := reservePort(t)

	cfgA := testutils.TempDir(t, "probe-cfg-a")
	rnsA := testutils.TempDir(t, "probe-rns-a")
	cfgB := testutils.TempDir(t, "probe-cfg-b")
	rnsB := testutils.TempDir(t, "probe-rns-b")
	writeTestNomadNetConfig(t, cfgA)
	writeProbeRNSServerConfig(t, rnsA, port)
	writeTestNomadNetConfig(t, cfgB)
	writeProbeRNSClientConfig(t, rnsB, port)

	// Boot both apps via the production Init() -> async initRNS() path. NewApp
	// sets PeerAnnounceAtStart=true (the harness default), so each announces on
	// its own after START_ANNOUNCE_DELAY (3s).
	appA := NewApp(cfgA, rnsA, false, false)
	if err := appA.Init(); err != nil {
		t.Fatalf("Init A: %v", err)
	}
	// Wait for A's async initRNS to finish BEFORE starting B. A is the TCP
	// server: NewTCPServerInterface binds/listens synchronously inside initRNS,
	// so once initWG completes A is listening on `port`. If B's TCP client
	// starts before A is listening, its first connect is refused and it waits
	// reconnectDelay (5s) to retry — but it has already registered with B's
	// transport (and announced B's destinations) while disconnected, so that
	// announce is lost, and reconnect does not re-announce. Under load that
	// means A never learns B and the test times out. Waiting here makes B's
	// first connect succeed, so B registers while connected and the announce
	// propagates both ways (the same path the isolation run takes in ~200ms).
	appA.initWG.Wait()

	appB := NewApp(cfgB, rnsB, false, false)
	if err := appB.Init(); err != nil {
		t.Fatalf("Init B: %v", err)
	}
	t.Cleanup(func() {
		appA.Shutdown()
		appB.Shutdown()
	})

	// Wait for B's initRNS goroutine to finish (transport + LXMF router + the
	// TCP interface loaded from rnsconfig + announce-at-start goroutine spawned).
	appB.initWG.Wait()

	if appA.LXMFDest == nil || appB.LXMFDest == nil {
		t.Fatalf("LXMF destinations not ready: A=%v B=%v", appA.LXMFDest, appB.LXMFDest)
	}
	hashA := hex.EncodeToString(appA.LXMFDest.Hash)
	hashB := hex.EncodeToString(appB.LXMFDest.Hash)
	t.Logf("A LXMF hash=%s", hashA)
	t.Logf("B LXMF hash=%s", hashB)

	// Capture B's delivered messages on a channel.
	receivedCh := make(chan *lxmf.Message, 4)
	appB.DeliveryCallback = func(msg any) {
		if m, ok := msg.(*lxmf.Message); ok {
			select {
			case receivedCh <- m:
			default:
			}
		}
	}

	// Wait for announce-at-start to propagate so each side recalls the other's
	// identity AND has a path. This is the precondition the harness's
	// announceWait covers; SendConversation aborts (returns false) if the peer
	// identity is not yet recalled.
	deadline := time.Now().Add(40 * time.Second)
	aKnowsB, bKnowsA := false, false
	for time.Now().Before(deadline) {
		if !aKnowsB && rns.RecallIdentity(appA.Transport, appB.LXMFDest.Hash) != nil && appA.Transport.HasPath(appB.LXMFDest.Hash) {
			aKnowsB = true
			t.Logf("A knows B (recalled + has_path) after %v", time.Since(deadline.Add(-40*time.Second)))
		}
		if !bKnowsA && rns.RecallIdentity(appB.Transport, appA.LXMFDest.Hash) != nil && appB.Transport.HasPath(appA.LXMFDest.Hash) {
			bKnowsA = true
			t.Logf("B knows A (recalled + has_path) after %v", time.Since(deadline.Add(-40*time.Second)))
		}
		if aKnowsB && bKnowsA {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !aKnowsB {
		t.Fatalf("A never recalled B / got a path: announce-at-start did not propagate A<-B")
	}
	if !bKnowsA {
		t.Fatalf("B never recalled A / got a path: announce-at-start did not propagate B<-A")
	}

	// A sends to B via the exact TUI C-d path (SendConversation -> Conversation.Send
	// -> appSendDeps -> Router.HandleOutbound). Default method for a brand-new
	// peer with no ratchet is MethodDirect.
	ok := appA.SendConversation(hashB, "hello-probe", "")
	if !ok {
		t.Fatalf("SendConversation returned false (peer identity not recalled? directory entry missing?)")
	}
	t.Logf("SendConversation returned true; waiting for B DeliveryCallback")

	// Wait for B to receive + deliver via callback (the router delivers the
	// inbound MethodDirect packet to the LXMF destination, firing the callback).
	select {
	case got := <-receivedCh:
		if got.ContentString() != "hello-probe" {
			t.Fatalf("delivered content = %q, want %q", got.ContentString(), "hello-probe")
		}
		t.Logf("B received %q via DeliveryCallback — TCP round-trip works at the app/RNS layer", got.ContentString())
	case <-time.After(30 * time.Second):
		t.Fatalf("B did not deliver A's MethodDirect message via DeliveryCallback within 30s")
	}
}
