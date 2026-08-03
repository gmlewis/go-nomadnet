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
	"time"
)

// SyncStatus holds the state for displaying conversation sync status.
// Matches Python's ConversationsDisplay._sync_status_line() logic.
type SyncStatus struct {
	LastSyncTime time.Time
	NodeLabel    string
	HasSynced    bool
	SyncRunning  bool
	SyncProgress int // 0-100
}

// FormatStatusLine returns the formatted sync status text.
// Matches Python's "Last sync: {time} ({node_label})" format.
func (ss *SyncStatus) FormatStatusLine() string {
	if !ss.HasSynced {
		return "Last sync: never"
	}

	var when string
	if ss.SyncRunning {
		when = "syncing..."
	} else {
		when = RelativeTime(ss.LastSyncTime)
	}

	if ss.NodeLabel != "" {
		return fmt.Sprintf("Last sync: %v (%v)", when, ss.NodeLabel)
	}
	return fmt.Sprintf("Last sync: %v", when)
}

// FormatSyncProgress returns a progress bar string for active syncs.
func (ss *SyncStatus) FormatSyncProgress() string {
	if !ss.SyncRunning {
		return ""
	}

	const barWidth = 20
	filled := int(float64(barWidth) * float64(ss.SyncProgress) / 100.0)
	empty := barWidth - filled

	bar := "["
	for i := 0; i < filled; i++ {
		bar += "="
	}
	for i := 0; i < empty; i++ {
		bar += " "
	}
	bar += "]"

	return fmt.Sprintf("%v %v%%", bar, ss.SyncProgress)
}
