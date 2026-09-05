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

package rrc

import (
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// The hub-liveness watchdog: the hub pings every ~30 s, so total inbound
// silence means the hub instance is gone even though this process's link
// object still looks active. The RNS link watchdog cannot be relied on for
// this — its stale window is 2*clamp(rtt*(KEEPALIVE_MAX/KEEPALIVE_MAX_RTT),
// 5s, 360s) with the rtt measured once at establishment, so a link
// established during a load spike (rtt ~15 s observed live on the
// raspberrypi) stays "active" for up to 12 minutes after the hub died: the
// TUI shows Connected while the user's joins and messages vanish.

func TestHubLivenessWatchdogTearsDownSilentHub(t *testing.T) {
	mgr, hub, _ := pingFixture(t)
	_ = mgr
	hub.link = &rns.Link{}
	hub.hubLiveness = 60 * time.Millisecond
	torn := make(chan struct{})
	hub.livenessTeardownFn = func() { close(torn) }
	hub.lastHubTraffic.Store(time.Now().UnixNano())
	hub.startHubLivenessLoop()

	select {
	case <-torn:
	case <-time.After(3 * time.Second):
		t.Fatal("the liveness watchdog did not tear down a hub link that went silent")
	}
}

func TestHubLivenessWatchdogResetByTraffic(t *testing.T) {
	mgr, hub, _ := pingFixture(t)
	_ = mgr
	hub.link = &rns.Link{}
	hub.hubLiveness = 120 * time.Millisecond
	torn := make(chan struct{})
	hub.livenessTeardownFn = func() { close(torn) }
	hub.lastHubTraffic.Store(time.Now().UnixNano())
	hub.startHubLivenessLoop()

	// Steady hub traffic (the ~30 s pings, compressed here) must keep the
	// watchdog quiet for the whole window.
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		hub.HandleData(pingEnvelope(t, []byte("pingbody"), "mid"))
		select {
		case <-torn:
			t.Fatal("the watchdog tore down a link that was receiving hub traffic")
		default:
		}
		time.Sleep(40 * time.Millisecond)
	}
}
