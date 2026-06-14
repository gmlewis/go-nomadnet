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

// jobs runs the background job loop for periodic tasks.
func (a *App) jobs() {
	// Defer initial jobs
	time.Sleep(time.Duration(a.DeferJobs) * time.Second)
	a.Logger.Info("Starting background job scheduler")

	for {
		if !a.ShouldRunJobs {
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

		time.Sleep(time.Duration(a.JobInterval) * time.Second)
	}
}
