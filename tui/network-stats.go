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
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/rivo/tview"
)

// AnnounceTimeLabel builds the "Announced : <when>" status line for the local
// peer, matching Python's AnnounceTime.update_time (Network.py:1036). A nil
// lastAnnounce yields "Never"; otherwise the timestamp is rendered with
// prettyDateAt against the supplied now (Python's pretty_date uses datetime.now).
func AnnounceTimeLabel(lastAnnounce *int64, now time.Time) string {
	s := "Never"
	if lastAnnounce != nil {
		s = prettyDateAt(time.Unix(*lastAnnounce, 0), now)
	}
	return "Announced : " + s
}

// NodeAnnounceTimeLabel builds the "Last Announce  : <when>" status line for
// the local node, matching Python's NodeAnnounceTime.update_time
// (Network.py:1068).
func NodeAnnounceTimeLabel(nodeLastAnnounce *int64, now time.Time) string {
	s := "Never"
	if nodeLastAnnounce != nil {
		s = prettyDateAt(time.Unix(*nodeLastAnnounce, 0), now)
	}
	return "Last Announce  : " + s
}

// NodeActiveConnectionsLabel builds the "Connected Now  : <n>" status line,
// matching Python's NodeActiveConnections.update_stat (Network.py:1099). With
// no node the value is "None".
func NodeActiveConnectionsLabel(linkCount int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(linkCount)
	}
	return "Connected Now  : " + s
}

// NodeStorageStatsLabel builds the "LXMF Storage   : <usage>" status line,
// matching Python's NodeStorageStats.update_stat (Network.py:1130). When the
// node is absent or propagation is disabled the value is "None". With a known
// limit it reports "pct%, used of limit"; with a nil limit it reports just the
// used size. pct uses Python's round((used/limit)*100, 1) which matches Go's
// %.1f banker's rounding.
func NodeStorageStatsLabel(used, limit *int64, hasNode, propagationEnabled bool) string {
	s := "None"
	if hasNode && propagationEnabled {
		pctStr := ""
		limitStr := ""
		if limit != nil && used != nil {
			pct := (float64(*used) / float64(*limit)) * 100
			pctStr = fmt.Sprintf("%.1f%%, ", pct)
			limitStr = " of " + Prettysize(float64(*limit))
		}
		usedStr := ""
		if used != nil {
			usedStr = Prettysize(float64(*used))
		}
		s = pctStr + usedStr + limitStr
	}
	return "LXMF Storage   : " + s
}

// NodeTotalConnectionsLabel builds the "Total Connects : <n>" status line,
// matching Python's NodeTotalConnections.update_stat (Network.py:1173).
func NodeTotalConnectionsLabel(connects int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(connects)
	}
	return "Total Connects : " + s
}

// NodeTotalPagesLabel builds the "Served Pages   : <n>" status line, matching
// Python's NodeTotalPages.update_stat (Network.py:1205).
func NodeTotalPagesLabel(pages int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(pages)
	}
	return "Served Pages   : " + s
}

// NodeTotalFilesLabel builds the "Served Files   : <n>" status line, matching
// Python's NodeTotalFiles.update_stat (Network.py:1237).
func NodeTotalFilesLabel(files int, hasNode bool) string {
	s := "None"
	if hasNode {
		s = strconv.Itoa(files)
	}
	return "Served Files   : " + s
}

// NetworkStats is the bordered "Network Stats" panel showing two refreshing
// lines — "Heard Peers: <n> (30m)" and "Known Nodes: <n>" — matching Python's
// NetworkStats (Network.py:1570-1603). The counts come from two injected
// providers so the widget need not import the app/directory. The refresh
// interval matches UpdatingText.timeout = animation_interval*5 (Network.py:1543),
// i.e. 5 s at the default 1 s animation_interval.
type NetworkStats struct {
	app      *App
	view     *tview.TextView
	numPeers func() int
	numNodes func() int
	interval time.Duration

	mu      sync.Mutex
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
}

// NewNetworkStats creates a NetworkStats panel that reads counts from the given
// providers. refresh is called once immediately so the panel is populated
// before the first tick. interval is the refresh period (use 5*
// animation_interval to match Python).
func NewNetworkStats(app *App, numPeers, numNodes func() int, interval time.Duration) *NetworkStats {
	ns := &NetworkStats{
		app:      app,
		numPeers: numPeers,
		numNodes: numNodes,
		interval: interval,
	}
	ns.view = tview.NewTextView().
		SetDynamicColors(false)
	ns.view.SetBorder(true)
	ns.view.SetTitle("Network Stats")
	ns.refresh()
	return ns
}

// Widget returns the bordered tview primitive.
func (ns *NetworkStats) Widget() tview.Primitive { return ns.view }

// refresh re-reads the count providers and updates the displayed text,
// matching UpdatingText.update (Network.py:1546-1548): title + str(value) +
// append_text for each line. The view mutation is guarded by ns.mu so the
// refresh goroutine does not race with concurrent readers (e.g. tests reading
// ViewText while the ticker fires).
func (ns *NetworkStats) refresh() {
	peers := 0
	nodes := 0
	if ns.numPeers != nil {
		peers = ns.numPeers()
	}
	if ns.numNodes != nil {
		nodes = ns.numNodes()
	}
	ns.mu.Lock()
	ns.view.SetText(fmt.Sprintf("Heard Peers: %v (30m)\nKnown Nodes: %v", peers, nodes))
	ns.mu.Unlock()
}

// ViewText returns the panel's current text under ns.mu, safe to call
// concurrently with the refresh goroutine.
func (ns *NetworkStats) ViewText() string {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	return ns.view.GetText(true)
}

// start launches the refresh goroutine. When marshal is true each refresh is
// queued onto the application event loop via QueueUpdateDraw (production); when
// false it runs synchronously (tests, where no event loop is running and
// QueueUpdateDraw would block forever on an undrained channel). Idempotent.
func (ns *NetworkStats) start(marshal bool) {
	ns.mu.Lock()
	if ns.started {
		ns.mu.Unlock()
		return
	}
	ns.stopCh = make(chan struct{})
	stop := ns.stopCh
	ns.started = true
	ns.mu.Unlock()

	ns.wg.Add(1)
	go func() {
		defer ns.wg.Done()
		ticker := time.NewTicker(ns.interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if marshal && ns.app != nil {
					ns.app.QueueUpdateDraw(ns.refresh)
				} else {
					ns.refresh()
				}
			}
		}
	}()
}

// Start launches the refresh goroutine, marshaling updates onto the event loop
// (production), matching NetworkStats.start (Network.py:1605-1607).
func (ns *NetworkStats) Start() { ns.start(true) }

// Stop halts the refresh goroutine. Idempotent.
func (ns *NetworkStats) Stop() {
	ns.mu.Lock()
	ns.started = false
	ch := ns.stopCh
	ns.stopCh = nil
	ns.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	ns.wg.Wait()
}
