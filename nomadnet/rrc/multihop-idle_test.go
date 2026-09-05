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

// MULTI-HOP idle-link survival: the fleet's RRC links span multiple RNS
// transport hops (client -> local TCPServer hub -> public relays -> rrcd).
// The single-hop loopback rig cannot see a transport-forwarding failure, so
// this test inserts a GO transport node between the Go client and the Python
// hub: client (C) -> Go transport (B, enable_transport=Yes) -> Python hub (A).
// The client then holds SILENCE and the link must survive on RNS keepalives
// alone; if the Go transport node fails to forward them, the Python hub's
// watchdog stales the link and this test fails.

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

func writeRNSConfigDir(t *testing.T, dir string, enableTransport bool, extra string) string {
	t.Helper()
	cfgDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transport := "No"
	if enableTransport {
		transport = "Yes"
	}
	content := fmt.Sprintf(`[reticulum]
share_instance = No
enable_transport = %s

[logging]
loglevel = 4

[interfaces]
%s`, transport, extra)
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgDir
}

func TestIntegrationMultiHopIdleLinkSurvives(t *testing.T) {
	t.Parallel()
	// The wait IS the assertion: the test holds a 2-hop link chain silent for
	// a real window to prove both hops survive, so it cannot meet the -short
	// budget.
	testutils.SkipShortIntegration(t)
	// A: Python mini-hub on port P1.
	p1 := freePortRRC(t)
	hubHash, _, hubCleanup := startPythonMiniHub(t, p1)
	defer hubCleanup()

	// B: Go transport node. TCP client to the hub (P1) + TCP server (P2).
	// tempDirRRC (not t.TempDir): on macOS t.TempDir paths are too long for
	// the Unix domain sockets RNS may create under the config dir.
	p2 := freePortRRC(t)
	bCfg := writeRNSConfigDir(t, tempDirRRC(t), true, fmt.Sprintf(`
  [[B-To-Hub]]
    type = TCPClientInterface
    enabled = yes
    target_host = 127.0.0.1
    target_port = %d

  [[B-Listener]]
    type = TCPServerInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = %d
`, p1, p2))

	bTS := rns.NewTransportSystem(nil)
	if _, err := rns.NewReticulum(bTS, bCfg); err != nil {
		t.Fatalf("node B NewReticulum: %v", err)
	}

	// C: Go client. TCP client -> B's listener (P2).
	cDir := tempDirRRC(t)
	cCfg := writeRNSConfigDir(t, cDir, false, fmt.Sprintf(`
  [[C-To-B]]
    type = TCPClientInterface
    enabled = yes
    target_host = 127.0.0.1
    target_port = %d
`, p2))

	cTS := rns.NewTransportSystem(nil)
	if _, err := rns.NewReticulum(cTS, cCfg); err != nil {
		t.Fatalf("node C NewReticulum: %v", err)
	}

	mgr := NewManager(filepath.Join(cDir, "storage"), func() []byte { return cTS.Identity().Hash })
	mgr.SetNickname("GoClient")
	mgr.SetTransport(cTS)

	hubHashBytes, err := hex.DecodeString(hubHash)
	if err != nil {
		t.Fatalf("hub hash: %v", err)
	}
	hub := mgr.AddHub(hubHashBytes, "rrc.hub", "RNS Community")

	// Warm C's path to the hub THROUGH B: B must receive the hub's announce
	// and re-announce it (transport mode), then C learns the multi-hop path.
	if !cTS.HasPath(hubHashBytes) {
		_ = cTS.RequestPath(hubHashBytes)
	}
	warm := time.Now().Add(25 * time.Second)
	for time.Now().Before(warm) && !cTS.HasPath(hubHashBytes) {
		time.Sleep(200 * time.Millisecond)
	}
	if !cTS.HasPath(hubHashBytes) {
		t.Fatal("client never learned a multi-hop path to the python hub through the go transport node")
	}
	hub.ConnectAsync()

	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		welcomed := hub.Welcomed
		hub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	hub.lock.Lock()
	welcomed := hub.Welcomed
	link := hub.link
	var rtt float64
	if link != nil {
		rtt = link.RTT()
	}
	hub.lock.Unlock()
	if !welcomed {
		t.Fatal("multi-hop hub never welcomed")
	}
	t.Logf("multi-hop link established, rtt=%.4fs", rtt)

	// Silence: freeze the who-refresh; only RNS keepalives may flow.
	hub.stopWhoRefresh()

	// 60s is far beyond the Python hub's stale window at wire RTT
	// (~2*5s=10s): a single dropped keepalive in that window would tear the
	// link down, so survival proves end-to-end keepalive traversal through
	// the Go transport node.
	//
	// Loopback knife edge: at rtt≈0 both sides clamp keepalive to the 5s
	// floor (stale=10s). The Python responder echoes a keepalive only when
	// its own last outbound is ≥keepalive old, and the initiator's
	// keepalives arrive exactly on that 5s boundary, so the echo is
	// occasionally skipped; the next echo then races the initiator's 10s
	// stale clock. 2026-09-04 forensics (CI run 33931694854 and a local
	// repro): the Go transport forwarded every keepalive in both directions
	// — the deaths were Python-hub echo-skip races against the stale
	// boundary, not transport loss. One death therefore proves nothing
	// about the transport, but repeated deaths do: a transport that actually
	// loses keepalives kills every link attempt within seconds of silence.
	// So each death triggers a reconnect and restarts the full silent
	// window, and only a third death (or a failed re-welcome) fails the
	// test.
	const (
		silentFor     = 60 * time.Second
		maxLinkDeaths = 2
		reconnectFor  = 25 * time.Second
	)
	deaths := 0
	deadline = time.Now().Add(silentFor)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		status, welcomedNow := hub.Status, hub.Welcomed
		hub.lock.Unlock()
		if welcomedNow && status == StatusConnected {
			time.Sleep(250 * time.Millisecond)
			continue
		}

		deaths++
		if deaths > maxLinkDeaths {
			t.Fatalf("MULTI-HOP link died %d times during %v silence windows (status=%d): keepalives are not traversing the Go transport node", deaths, silentFor, status)
		}
		t.Logf("link died during silence (death %d/%d, status=%d); reconnecting", deaths, maxLinkDeaths, status)
		hub.ConnectAsync()
		reconnectDeadline := time.Now().Add(reconnectFor)
		for time.Now().Before(reconnectDeadline) {
			hub.lock.Lock()
			status, welcomedNow = hub.Status, hub.Welcomed
			hub.lock.Unlock()
			if welcomedNow && status == StatusConnected {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		hub.lock.Lock()
		status, welcomedNow = hub.Status, hub.Welcomed
		hub.lock.Unlock()
		if !welcomedNow || status != StatusConnected {
			t.Fatalf("multi-hop link died during silence and the reconnect never re-welcomed within %v (status=%d)", reconnectFor, status)
		}
		// The re-welcome re-armed the who-refresh timer; silence again.
		hub.stopWhoRefresh()
		deadline = time.Now().Add(silentFor)
	}
	t.Logf("multi-hop link survived %v of silence (%d link deaths tolerated)", silentFor, deaths)
}
