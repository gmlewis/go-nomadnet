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

package node

import (
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/testutils"
)

// TestResetStatsZeroesTheCounters pins the node's own bookkeeping reset: the App
// persists the peer-settings copy, and this clears the in-memory counters the live
// "Connected Now" / served counts are read from. A reset that left one of them set would
// show the user a stats pane that disagrees with the file it just wrote.
func TestResetStatsZeroesTheCounters(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	n := NewNode("stats-node", dir, dir, 720, 0, 0, false)
	n.ServedPageRequests = 7
	n.ServedFileRequests = 9
	n.NodeConnects = 3

	n.ResetStats()

	if n.ServedPageRequests != 0 || n.ServedFileRequests != 0 || n.NodeConnects != 0 {
		t.Errorf("counters after ResetStats = pages %v, files %v, connects %v; want all zero",
			n.ServedPageRequests, n.ServedFileRequests, n.NodeConnects)
	}
}

// TestStartJobsRunsAndStopStops pins the node job loop's lifecycle: StartJobs launches
// the loop tracked on jobsWG, the loop does its work (a page-refresh pass), and Stop
// waits for it to exit and leaves it exited. The refresh interval is one minute and the
// test runs in a synctest bubble, so the clock advances virtually rather than the test
// sleeping for real.
func TestStartJobsRunsAndStopStops(t *testing.T) {
	// The temp directory is created BEFORE the bubble: the first testutils.TempDir call
	// installs the package's signal cleanup handler (signal.Notify), and the runtime
	// goroutine that starts for it cannot coexist with a synctest bubble — it selects on
	// a channel the bubble owns and the runtime reports "select on synctest channel from
	// outside bubble".
	dir := tempDir(t)

	// Not parallel: synctest bubbles own the clock for their goroutines.
	testutils.RunInBubble(t, func(t *testing.T) {
		// pageRefresh = 1 minute, so one pass past a minute rescans the pages directory
		// and stamps LastPageRefresh.
		n := NewNode("jobs-node", dir, dir, 720, 1, 0, false)

		n.mu.Lock()
		before := n.LastPageRefresh
		n.mu.Unlock()

		n.StartJobs()
		time.Sleep(2 * time.Minute)

		n.mu.Lock()
		refreshed := n.LastPageRefresh
		n.mu.Unlock()
		if !refreshed.After(before) {
			t.Fatalf("the job loop did not run a page-refresh pass: LastPageRefresh still %v", refreshed)
		}

		n.Stop()
		if n.ShouldRunJobs {
			t.Error("Stop left ShouldRunJobs true")
		}

		// Once stopped, no further pass may run however far the clock advances. If Stop
		// did not wait for the loop (StartJobs did not track it on jobsWG), or the loop
		// ignored the stop channel, another refresh would land here.
		time.Sleep(5 * time.Minute)
		n.mu.Lock()
		afterStop := n.LastPageRefresh
		n.mu.Unlock()
		if !afterStop.Equal(refreshed) {
			t.Errorf("the job loop kept running after Stop: LastPageRefresh moved from %v to %v", refreshed, afterStop)
		}

		// Stop is idempotent (jobsStopOnce), so a second call must not panic on a
		// double close of the stop channel — the shutdown path can race with a quit.
		n.Stop()
	})
}
