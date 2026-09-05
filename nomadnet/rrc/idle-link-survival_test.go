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

// Idle-link survival against a REAL Python RNS link watchdog: the fleet
// symptom is an RRC hub link that dies every ~45s whenever the client goes
// silent (the RaspPi's connect/disconnect storm). Python's Link watchdog
// shrinks keepalive to max(min(rtt*(KEEPALIVE_MAX/KEEPALIVE_MAX_RTT),
// KEEPALIVE_MAX), KEEPALIVE_MIN) at establishment (Link.py:795-797), so on a
// fast link the hub stales an idle link after ~2*5s=10s unless the client
// sends RNS keepalives. This test holds an established link SILENT (no
// who-refresh, no messages) and asserts the link survives: the Go client's
// watchdog must send keepalives often enough to keep the Python hub's
// last_inbound fresh.

import (
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

func TestIntegrationIdleLinkSurvivesPythonWatchdog(t *testing.T) {
	t.Parallel()
	// The wait IS the assertion: the test holds the link silent for a real
	// window to prove the Python watchdog does not tear it down, so it cannot
	// meet the -short budget.
	testutils.SkipShortIntegration(t)
	port := freePortRRC(t)
	hubHash, _, hubCleanup := startPythonMiniHub(t, port)
	defer hubCleanup()

	// tempDirRRC (not t.TempDir): on macOS t.TempDir paths are too long for
	// the Unix domain sockets RNS may create under the config dir.
	dir := tempDirRRC(t)
	cfgDir := filepath.Join(dir, "config")
	writeRNSConfigRRC(t, cfgDir)
	appendTCPClientInterface(t, filepath.Join(cfgDir, "config"), port)

	ts := rns.NewTransportSystem(nil)
	if _, err := rns.NewReticulum(ts, cfgDir); err != nil {
		t.Fatalf("NewReticulum: %v", err)
	}

	mgr := NewManager(filepath.Join(dir, "storage"), func() []byte { return ts.Identity().Hash })
	mgr.SetNickname("GoClient")
	mgr.SetTransport(ts)
	hubHashBytes, err := hex.DecodeString(hubHash)
	if err != nil {
		t.Fatalf("hub hash: %v", err)
	}
	hub := mgr.AddHub(hubHashBytes, "rrc.hub", "RNS Community")

	// Warm the path BEFORE dialing so the measured establishment RTT reflects
	// pure wire time, not path-discovery latency: on a cold path the rtt
	// includes announce propagation (seconds), which clamps the keepalive to
	// its 360s ceiling and masks the watchdog behavior under test.
	if !ts.HasPath(hubHashBytes) {
		_ = ts.RequestPath(hubHashBytes)
		pathDeadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(pathDeadline) && !ts.HasPath(hubHashBytes) {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !ts.HasPath(hubHashBytes) {
		t.Fatal("no path to the python hub")
	}
	hub.ConnectAsync()

	// Wait for the welcome, then go completely silent.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		welcomed := hub.Welcomed
		hub.lock.Unlock()
		if welcomed {
			break
		}
		time.Sleep(100 * time.Millisecond)
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
		t.Fatal("hub never welcomed")
	}
	t.Logf("established with rtt=%.3fs (Python keepalive would be %.1fs, stale %.1fs)",
		rtt, max(1, min(rtt*(360/1.75), 360)), 2*max(1, min(rtt*(360/1.75), 360)))

	// Freeze the who-refresh so the link sees ZERO RRC traffic; only the RNS
	// watchdog's own keepalives may flow.
	hub.stopWhoRefresh()

	// Hold silence past the hub-side stale window. On loopback the Python
	// hub's rtt is tiny, so its keepalive clamps to KEEPALIVE_MIN=5s and its
	// stale window is 2*5s=10s. 25s of silence is 3 stale windows.
	//
	// Loopback knife edge (same exposure as TestIntegrationMultiHopIdleLinkSurvives):
	// the stale window is exactly 2x the keepalive cadence, and the Python
	// responder echoes a keepalive only when its own last outbound is
	// >=keepalive old, so the echo is occasionally skipped right at the 5s
	// boundary and the next echo races a 10s stale clock. On a loaded CI
	// runner a single delayed keepalive or skipped echo can therefore kill a
	// perfectly healthy link whose keepalives ARE flowing — one death proves
	// nothing about the client's keepalives, but repeated deaths do: a
	// client that genuinely fails to send them kills every link attempt
	// within seconds of silence. So each death triggers a reconnect and
	// restarts the full silent window, and only a third death (or a failed
	// re-welcome) fails the test.
	const (
		silentFor     = 25 * time.Second
		maxLinkDeaths = 2
		reconnectFor  = 25 * time.Second
	)
	deaths := 0
	deadline = time.Now().Add(silentFor)
	for {
		hub.lock.Lock()
		status, welcomed := hub.Status, hub.Welcomed
		link := hub.link
		hub.lock.Unlock()
		if welcomed && status == StatusConnected && link != nil {
			if !time.Now().Before(deadline) {
				break
			}
			time.Sleep(250 * time.Millisecond)
			continue
		}

		deaths++
		if deaths > maxLinkDeaths {
			t.Fatalf("link DIED %d times during %v silence windows: status=%d text=%q (client keepalives are not arriving at the Python hub)", deaths, silentFor, status, hub.StatusText)
		}
		t.Logf("link died during silence (death %d/%d, status=%d text=%q); reconnecting", deaths, maxLinkDeaths, status, hub.StatusText)
		hub.ConnectAsync()
		reconnectDeadline := time.Now().Add(reconnectFor)
		for time.Now().Before(reconnectDeadline) {
			hub.lock.Lock()
			status, welcomed = hub.Status, hub.Welcomed
			hub.lock.Unlock()
			if welcomed && status == StatusConnected {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		hub.lock.Lock()
		status, welcomed = hub.Status, hub.Welcomed
		hub.lock.Unlock()
		if !welcomed || status != StatusConnected {
			t.Fatalf("link died during silence and the reconnect never re-welcomed within %v (status=%d text=%q)", reconnectFor, status, hub.StatusText)
		}
		// The re-welcome re-armed the who-refresh timer; silence again.
		hub.stopWhoRefresh()
		deadline = time.Now().Add(silentFor)
	}

	// The link is connected (the loop only exits on a healthy check). The
	// Python hub's stale window on this loopback link is ~2*5s=10s (keepalive
	// clamps to KEEPALIVE_MIN at wire RTT), so surviving the silent window
	// proves the client's RNS keepalives are arriving: Python consumes them
	// inside Link.receive (they never reach the app-level packet callback, so
	// the hub's event log cannot count them — the survival itself is the
	// receipt evidence).
	t.Logf("link survived %v of total silence (%d link deaths tolerated)", silentFor, deaths)
}
