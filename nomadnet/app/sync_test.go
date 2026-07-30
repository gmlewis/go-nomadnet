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
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

func newRouterForTest(t *testing.T) *lxmf.Router {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, err := lxmf.NewRouter(ts, id, tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestGetSyncStatusIdle(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Router = newRouterForTest(t)
	if got := a.GetSyncStatus(); got != "Idle" {
		t.Fatalf("got %q, want Idle", got)
	}
	if a.SyncStatusShowPercent() {
		t.Fatal("show percent should be false when idle")
	}
	if a.GetSyncProgress() != 0 {
		t.Fatalf("progress = %v, want 0", a.GetSyncProgress())
	}
}

func TestGetSyncStatusNoRouter(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	if got := a.GetSyncStatus(); got != "Idle" {
		t.Fatalf("got %q, want Idle", got)
	}
	if a.SyncStatusShowPercent() {
		t.Fatal("show percent should be false without router")
	}
	if a.GetSyncProgress() != 0 {
		t.Fatal("progress should be 0 without router")
	}
}

func TestCancelLXMFSyncIdle(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.Router = newRouterForTest(t)
	// Should be a no-op when idle (state == PRIdle).
	a.CancelLXMFSync()
	if a.Router.PropagationTransferState() != lxmf.PRIdle {
		t.Fatal("state should remain idle")
	}
}
