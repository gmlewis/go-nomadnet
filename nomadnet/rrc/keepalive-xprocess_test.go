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

package rrc

// TODO item 22 forensics: the live rrcd hub's own RNS watchdog ("Link closed"
// logged from rrcd's __watchdog_job threads) tears down idle Go-client links
// after minutes, while the Python nomadnet client's links survive. This test
// isolates the link-layer interop: a Python RNS responder (the same role
// rrcd's hub plays — the link RESPONDER) over loopback TCP, a raw Go initiator
// link, then a completely silent idle window. With loopback RTT ≈ 0 both
// sides derive keepalive=5s and stale=10s (Link.KEEPALIVE_MIN × STALE_FACTOR),
// so 30s of silence is enough to prove or refute that the Go client's
// keepalives feed the Python watchdog.

import (
	"bufio"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// pythonKeepaliveResponderScript is a raw RNS link RESPONDER (the hub-side
// role rrcd plays): it announces an IN destination over the config-supplied
// TCPServerInterface, captures the inbound link, and dumps its watchdog state
// every 2 seconds — rx count, last-inbound age, derived keepalive and stale
// windows, and link status — so the test can see exactly when the Python side
// stops hearing the Go client.
const pythonKeepaliveResponderScript = `
import sys, time
import RNS

configdir = sys.argv[1]
RNS.Reticulum(configdir)

identity = RNS.Identity()
dest = RNS.Destination(identity, RNS.Destination.IN, RNS.Destination.SINGLE, "rrc", "chat")

link_ref = [None]

def on_link(link):
    link_ref[0] = link
    link.set_link_closed_callback(on_closed)
    print("ESTABLISHED rtt=%s keepalive=%s stale=%s" % (link.rtt, link.keepalive, link.stale_time), flush=True)

def on_closed(link):
    print("CLOSED reason=%s" % (link.teardown_reason,), flush=True)

dest.set_link_established_callback(on_link)
dest.announce()
print("HASH=" + dest.hash.hex(), flush=True)

while True:
    time.sleep(2)
    link = link_ref[0]
    if link is None:
        print("WAITING", flush=True)
        continue
    li = link.last_inbound if link.last_inbound is not None else 0
    print("STATE rx=%s age=%.1f keepalive=%.1f stale=%.1f status=%s" % (
        link.rx, time.time() - li, link.keepalive, link.stale_time, link.status), flush=True)
`

// TestIntegrationKeepalivePythonResponder proves the Go link keepalive feeds a
// Python RNS responder through 30+ seconds of complete silence (TODO item 22).
// On loopback both sides derive keepalive=5s / stale=10s, so a silent 30s
// window with the Python side still ACTIVE (age < 12s) demonstrates the
// cross-implementation keepalive works at the link layer.
func TestIntegrationKeepalivePythonResponder(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)
	pyPath := findRNSPython(t)

	pyPort := reserveTCPPortXProc(t)
	pyCfgDir := filepath.Join(testutils.TempDir(t, "nomadnet-rrc-keepalive-py"), "config")
	writePythonRNSConfigWithTCPServer(t, pyCfgDir, pyPort)

	scriptPath := filepath.Join(filepath.Dir(pyCfgDir), "keepalive_responder.py")
	if err := os.WriteFile(scriptPath, []byte(pythonKeepaliveResponderScript), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(pyPath, scriptPath, pyCfgDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	lb := &lineBuffer{}
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			lb.push(scanner.Text())
		}
	}()

	hashHex := lb.waitForLine(t, "HASH=", 15*time.Second)
	serverHash, err := hex.DecodeString(strings.TrimSpace(hashHex))
	if err != nil || len(serverHash) == 0 {
		t.Fatalf("invalid HASH from python: %q (err %v)", hashHex, err)
	}

	ts, tsCleanup := newStartedTSWithTCPClient(t, "127.0.0.1", pyPort)
	defer tsCleanup()

	identity := waitForIdentity(t, ts, serverHash, 20*time.Second)
	dest, err := rns.NewDestination(ts, identity, rns.DestinationOut, rns.DestinationSingle, "rrc", "chat")
	if err != nil {
		t.Fatalf("NewDestination: %v", err)
	}

	link, err := rns.NewLink(ts, dest)
	if err != nil {
		t.Fatalf("NewLink: %v", err)
	}
	established := make(chan struct{}, 1)
	link.SetLinkEstablishedCallback(func(_ *rns.Link) {
		select {
		case established <- struct{}{}:
		default:
		}
	})
	closed := make(chan string, 1)
	link.SetLinkClosedCallback(func(_ *rns.Link) {
		select {
		case closed <- "closed":
		default:
		}
	})
	if err := link.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	select {
	case <-established:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for Go→Python link establishment")
	}

	// The silent idle window: with loopback RTT both sides derive
	// keepalive=5s / stale=10s, so the responder must keep hearing the
	// client's 0xFF keepalives the whole time.
	deadline := time.Now().Add(30 * time.Second)
	maxAge := 0.0
	for time.Now().Before(deadline) {
		select {
		case <-closed:
			t.Fatalf("Go link closed during the idle window; python so far:\n%v", lbAll(lb))
		default:
		}
		if state, ok := lb.findLine("STATE "); ok && strings.Contains(state, "age=") {
			if age := parseStateAge(state); age > maxAge {
				maxAge = age
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if maxAge > 12.0 {
		t.Errorf("Python responder last-inbound age reached %.1fs (stale is 10s): the Go keepalives are not feeding the responder", maxAge)
	}
	if lb.hasLinePrefix("CLOSED") {
		t.Errorf("Python responder closed the link during the idle window:\n%v", lbAll(lb))
	}
}

func parseStateAge(state string) float64 {
	idx := strings.Index(state, "age=")
	if idx < 0 {
		return 0
	}
	rest := state[idx+len("age="):]
	end := strings.IndexAny(rest, " ")
	if end < 0 {
		end = len(rest)
	}
	v, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil {
		return 0
	}
	return v
}

func lbAll(lb *lineBuffer) string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return strings.Join(lb.lines, "\n")
}

// hasLinePrefix reports whether any collected line starts with prefix.
func (lb *lineBuffer) hasLinePrefix(prefix string) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	for _, l := range lb.lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

// waitForIdentity polls Recall until the announced identity arrives.
func waitForIdentity(t *testing.T, ts *rns.TransportSystem, hash []byte, timeout time.Duration) *rns.Identity {
	t.Helper()
	_ = ts.RequestPath(hash)
	var id *rns.Identity
	if !testutils.PollUntil(timeout, func() bool {
		id = ts.Recall(hash)
		return id != nil
	}) {
		t.Fatal("timeout waiting for the announced identity")
	}
	return id
}
