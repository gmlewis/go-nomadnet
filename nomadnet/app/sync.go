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

	"github.com/gmlewis/go-nomadnet/nomadnet/util"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
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
			a.psMu.Lock()
			a.PeerSettings.LastLXMFSync = int(timeNow().Unix())
			a.savePeerSettingsLocked()
			a.psMu.Unlock()
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

// LastSyncInfo returns the persisted last-LXMF-sync time and the default
// propagation node's display label for the Conversations "Last sync:" footer,
// mirroring Python's _sync_status_line (Conversations.py:517-545). The time is
// read live from PeerSettings.LastLXMFSync (peer_settings["last_lxmf_sync"]),
// so it survives across runs (a sync writes the field via RequestLXMFSync).
// The label is the propagation node's directory display name, falling back to
// "<" + the first 8 hex chars of its destination hash + "…" (Python lines
// 530-543). A zero time (no sync recorded) formats as "never" upstream.
func (a *App) LastSyncInfo() (time.Time, string) {
	var t time.Time
	a.psMu.Lock()
	if a.PeerSettings != nil && a.PeerSettings.LastLXMFSync > 0 {
		t = time.Unix(int64(a.PeerSettings.LastLXMFSync), 0)
	}
	a.psMu.Unlock()

	if t.IsZero() {
		return t, ""
	}

	pnHash := a.GetDefaultPropagationNode()
	if len(pnHash) == 0 {
		return t, ""
	}

	// Recall the node identity and look up its "nomadnetwork.node" directory
	// entry for the display name (Python: Identity.recall →
	// hash_from_name_and_identity("nomadnetwork.node", ident) → directory.find).
	if a.Transport != nil {
		if id := rns.RecallIdentity(a.Transport, pnHash); id != nil {
			nodeDest := rns.CalculateHash(id, "nomadnetwork", "node")
			if entry := a.Dir.Find(nodeDest); entry != nil && entry.DisplayName != "" {
				if name := util.StripModifiers(&entry.DisplayName); name != nil && *name != "" {
					return t, *name
				}
			}
		}
	}

	// Fallback: "<" + first 8 hex chars of the propagation node hash + "…".
	return t, "<" + fmt.Sprintf("%x", pnHash)[:8] + "…>"
}
