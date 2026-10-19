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

package app

import "time"

// lxmfSyncDue reports whether the periodic LXMF sync is due at now. It mirrors
// Python's job-loop test verbatim (NomadNetworkApp.py:464, "now >
// self.peer_settings['last_lxmf_sync'] + self.lxmf_sync_interval"), so the clock
// it reads is the persisted last-sync time, seeded from peer settings at startup
// (see loadPeerSettings) rather than a per-run in-memory zero. Without that seed
// the zero time made every launch look like it had never synced, and the client
// re-synced its whole mailbox on each start.
//
// LastLXMFSync and its interval are read under mu because RequestLXMFSync (which
// writes LastLXMFSync) is reachable from the TUI goroutine while this runs on
// the job goroutine.
func (a *App) lxmfSyncDue(now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Deliberate Go improvement: Python's __jobs never consults
	// periodic_lxmf_sync (NomadNetworkApp.py:465-467), so the key is inert there.
	// Honoring it makes a documented setting actually work; see §6 of
	// parity-gaps.md.
	if !a.PeriodicLXMFSync {
		return false
	}
	return now.After(a.LastLXMFSync.Add(time.Duration(a.LXMFSyncInterval) * time.Second))
}

// jobs runs the background job loop for periodic tasks. The initial defer and
// the inter-iteration waits select on jobsStop so Shutdown (which closes it)
// interrupts the loop immediately rather than blocking up to DeferJobs seconds.
func (a *App) jobs() {
	// Defer initial jobs, but bail out immediately if Shutdown closes jobsStop
	// during the defer.
	select {
	case <-a.jobsStop:
		return
	case <-time.After(time.Duration(a.DeferJobs) * time.Second):
	}
	a.Logger.Info("Starting background job scheduler")

	for {
		a.mu.Lock()
		run := a.ShouldRunJobs
		a.mu.Unlock()
		if !run {
			return
		}

		now := time.Now()

		// Periodic LXMF sync
		if a.lxmfSyncDue(now) {
			a.Logger.Verbose("Initiating automatic LXMF sync")
			a.RequestLXMFSync(a.LXMFSyncLimit)
		}

		// Periodic announce
		if now.After(a.LastAnnounce.Add(time.Duration(a.AnnounceInterval) * time.Second)) {
			a.Logger.Verbose("Sending periodic announce")
			a.AnnounceNow()
		}

		select {
		case <-a.jobsStop:
			return
		case <-time.After(time.Duration(a.JobInterval) * time.Second):
		}
	}
}
