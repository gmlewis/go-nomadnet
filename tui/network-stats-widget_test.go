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

package tui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestNetworkStatsWidgetContent asserts the NetworkStats panel renders the two
// UpdatingText lines from Python's NetworkStats (Network.py:1570-1603):
// "Heard Peers: <n> (30m)" and "Known Nodes: <n>", inside a bordered box titled
// "Network Stats". The value providers are injected so the widget need not
// import the app.
func TestNetworkStatsWidgetContent(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	peers := 5
	nodes := 12
	ns := NewNetworkStats(app,
		func() int { return peers },
		func() int { return nodes },
		time.Second*5,
	)

	text := ns.view.GetText(true)
	for _, want := range []string{"Heard Peers: 5 (30m)", "Known Nodes: 12"} {
		if !strings.Contains(text, want) {
			t.Errorf("NetworkStats text missing %q; got: %q", want, text)
		}
	}
	if got := ns.view.GetTitle(); got != "Network Stats" {
		t.Errorf("NetworkStats title = %q, want %q", got, "Network Stats")
	}
	// tview exposes no border getter (Box.border is unexported); border
	// presence is verified via capture, not unit-tested here.
}

// TestNetworkStatsWidgetRefresh asserts refresh() re-reads the providers and
// updates the displayed text, matching UpdatingText.update
// (Network.py:1546-1548).
func TestNetworkStatsWidgetRefresh(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	peers := 1
	nodes := 1
	ns := NewNetworkStats(app,
		func() int { return peers },
		func() int { return nodes },
		time.Second*5,
	)

	peers, nodes = 42, 7
	ns.refresh()

	text := ns.view.GetText(true)
	if !strings.Contains(text, "Heard Peers: 42 (30m)") {
		t.Errorf("after refresh, text = %q; want Heard Peers: 42 (30m)", text)
	}
	if !strings.Contains(text, "Known Nodes: 7") {
		t.Errorf("after refresh, text = %q; want Known Nodes: 7", text)
	}
}

// TestNetworkStatsWidgetStartStop asserts Start launches a refresh goroutine
// that picks up provider changes, and Stop halts it (no further updates),
// matching NetworkStats.start (Network.py:1605-1607) + UpdatingText.start/stop.
func TestNetworkStatsWidgetStartStop(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	var peers atomic.Int64
	ns := NewNetworkStats(app,
		func() int { return int(peers.Load()) },
		func() int { return 0 },
		20*time.Millisecond,
	)

	// Synchronous path (marshal=false): tests have no running event loop, so
	// QueueUpdateDraw would block. Production Start() marshals via
	// QueueUpdateDraw.
	ns.start(false)
	defer ns.Stop()

	peers.Store(99)
	// Wait long enough for at least one refresh tick.
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(ns.ViewText(), "Heard Peers: 99 (30m)") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("start did not refresh within timeout; text=%q", ns.ViewText())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	ns.Stop()
	peers.Store(1000)
	time.Sleep(80 * time.Millisecond)
	if strings.Contains(ns.ViewText(), "Heard Peers: 1000 (30m)") {
		t.Error("Stop did not halt the refresh goroutine")
	}
}
