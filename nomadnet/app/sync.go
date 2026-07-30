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

import (
	"fmt"
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
)

// GetSyncStatus returns a human-readable description of the current LXMF
// propagation sync state, mirroring the Python NomadNet get_sync_status.
func (a *App) GetSyncStatus() string {
	if a.Router == nil {
		return "Idle"
	}
	state := a.Router.PropagationTransferState()
	switch state {
	case lxmf.PRIdle:
		return "Idle"
	case lxmf.PRPathRequested:
		return "Path requested"
	case lxmf.PRLinkEstablishing:
		return "Establishing link"
	case lxmf.PRLinkEstablished:
		return "Link established"
	case lxmf.PRRequestSent:
		return "Sync request sent"
	case lxmf.PRReceiving:
		return "Receiving messages"
	case lxmf.PRResponseReceived:
		return "Messages received"
	case lxmf.PRNoPath:
		return "No path to node"
	case lxmf.PRLinkFailed:
		return "Link establisment failed"
	case lxmf.PRTransferFailed:
		return "Sync request failed"
	case lxmf.PRNoIdentityRcvd:
		return "Remote got no identity"
	case lxmf.PRNoAccess:
		return "Node rejected request"
	case lxmf.PRFailed:
		return "Sync failed"
	case lxmf.PRComplete:
		newMsgs, ok := a.Router.PropagationTransferLastResult()
		if !ok || newMsgs == 0 {
			return "Done, no new messages"
		}
		return fmt.Sprintf("Downloaded %v new messages", newMsgs)
	}
	return "Unknown"
}

// SyncStatusShowPercent reports whether the sync status display should show a
// progress percentage (true only while actively receiving). This mirrors the
// Python NomadNet sync_status_show_percent.
func (a *App) SyncStatusShowPercent() bool {
	if a.Router == nil {
		return false
	}
	switch a.Router.PropagationTransferState() {
	case lxmf.PRReceiving, lxmf.PRResponseReceived:
		return true
	}
	return false
}

// GetSyncProgress returns the propagation sync progress as a fraction between
// 0 and 1, mirroring the Python NomadNet get_sync_progress.
func (a *App) GetSyncProgress() float64 {
	if a.Router == nil {
		return 0
	}
	return a.Router.PropagationTransferProgress()
}

// RequestLXMFSync initiates a propagation-node sync when the router is idle or
// a previous sync has completed, recording the sync time and persisting peer
// settings. A non-positive limit means no cap. This mirrors the Python NomadNet
// request_lxmf_sync.
func (a *App) RequestLXMFSync(limit int) {
	if a.Router == nil {
		a.mu.Lock()
		a.LastLXMFSync = timeNow()
		a.mu.Unlock()
		return
	}
	state := a.Router.PropagationTransferState()
	if state == lxmf.PRIdle || state >= lxmf.PRComplete {
		if a.PeerSettings != nil {
			a.PeerSettings.LastLXMFSync = int(timeNow().Unix())
			a.SavePeerSettings()
		}
		var lim *int
		if limit > 0 {
			lim = &limit
		}
		a.Router.RequestMessagesFromPropagationNode(lim)
		a.mu.Lock()
		a.LastLXMFSync = timeNow()
		a.mu.Unlock()
	}
}

// CancelLXMFSync cancels an in-progress propagation sync, mirroring the Python
// NomadNet cancel_lxmf_sync.
func (a *App) CancelLXMFSync() {
	if a.Router == nil {
		return
	}
	if a.Router.PropagationTransferState() != lxmf.PRIdle {
		a.Router.CancelPropagationNodeRequests()
	}
}

func timeNow() time.Time { return time.Now() }
