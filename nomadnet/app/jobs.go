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
		if a.PeriodicLXMFSync && now.After(a.LastLXMFSync.Add(time.Duration(a.LXMFSyncInterval)*time.Second)) {
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
