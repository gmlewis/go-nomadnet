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
	"testing"
	"time"
)

func TestSyncStatusNeverSynced(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{}
	got := ss.FormatStatusLine()
	if got != "Last sync: never" {
		t.Errorf("FormatStatusLine() = %q, want %q", got, "Last sync: never")
	}
}

func TestSyncStatusWithTime(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{
		HasSynced:    true,
		LastSyncTime: time.Now().Add(-5 * time.Minute),
	}

	got := ss.FormatStatusLine()
	if !strings.HasPrefix(got, "Last sync: 5m ago") {
		t.Errorf("FormatStatusLine() = %q, want prefix %q", got, "Last sync: 5m ago")
	}
}

func TestSyncStatusWithNodeLabel(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{
		HasSynced:    true,
		LastSyncTime: time.Now().Add(-10 * time.Minute),
		NodeLabel:    "MyNode",
	}

	got := ss.FormatStatusLine()
	if !strings.Contains(got, "(MyNode)") {
		t.Errorf("FormatStatusLine() = %q, want to contain (MyNode)", got)
	}
	if !strings.HasPrefix(got, "Last sync: 10m ago") {
		t.Errorf("FormatStatusLine() = %q, want prefix %q", got, "Last sync: 10m ago")
	}
}

func TestSyncStatusSyncing(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{
		HasSynced:   true,
		SyncRunning: true,
		NodeLabel:   "MyNode",
	}

	got := ss.FormatStatusLine()
	if !strings.Contains(got, "syncing...") {
		t.Errorf("FormatStatusLine() = %q, want to contain syncing...", got)
	}
}

func TestSyncStatusProgress(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{SyncRunning: true, SyncProgress: 50}
	got := ss.FormatSyncProgress()
	if !strings.Contains(got, "50%") {
		t.Errorf("FormatSyncProgress() = %q, want to contain 50%%", got)
	}
	if !strings.Contains(got, "[") || !strings.Contains(got, "]") {
		t.Errorf("FormatSyncProgress() missing brackets: %q", got)
	}
}

func TestSyncStatusProgressNotRunning(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{SyncRunning: false, SyncProgress: 100}
	got := ss.FormatSyncProgress()
	if got != "" {
		t.Errorf("FormatSyncProgress() = %q, want empty when not running", got)
	}
}

func TestSyncStatusProgressZero(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{SyncRunning: true, SyncProgress: 0}
	got := ss.FormatSyncProgress()
	if !strings.Contains(got, "0%") {
		t.Errorf("FormatSyncProgress() = %q, want to contain 0%%", got)
	}
}

func TestSyncStatusProgressFull(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{SyncRunning: true, SyncProgress: 100}
	got := ss.FormatSyncProgress()
	if !strings.Contains(got, "100%") {
		t.Errorf("FormatSyncProgress() = %q, want to contain 100%%", got)
	}
}
