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
	const silentFor = 60 * time.Second
	deadline = time.Now().Add(silentFor)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		status, welcomedNow := hub.Status, hub.Welcomed
		hub.lock.Unlock()
		if !welcomedNow || status != StatusConnected {
			t.Fatalf("MULTI-HOP link DIED during %v silence: status=%d (the Go transport node loses keepalives; the Python hub watchdog staled the link)", silentFor, status)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Logf("multi-hop link survived %v of silence", silentFor)
}
