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

package main

import (
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-reticulum/rrc"
)

// statusCapture collects the hub statuses reported by the manager's change
// callback.
//
// The callback is not a single-goroutine observable: ConnectAsync fires it on
// the selecting goroutine before it spawns the connect worker, and the worker
// then fires it again (and again via scheduleReconnect) from its own
// goroutine. The collector is therefore locked rather than assuming the two
// never overlap.
type statusCapture struct {
	mu       sync.Mutex
	statuses []int
}

func (c *statusCapture) append(status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses = append(c.statuses, status)
}

// snapshot returns a copy of the statuses seen so far, so an assertion never
// reads the slice the callback is appending to.
func (c *statusCapture) snapshot() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.statuses...)
}

// Selecting a hub row in the channels list must make the hub active, show its
// info panel, and — only for a hub whose AutoRcn (auto-reconnect) toggle is on —
// auto-connect it when it is disconnected or failed.
//
// Python's ChannelsDisplay._select_hub runs _maybe_autoconnect before
// _show_hub_info (Channels.py:1716-1720, 1736-1741), but its _maybe_autoconnect
// dials any disconnected or failed hub without consulting auto_reconnect, so
// selecting a hub row the user has taken off automatic connection still brought
// the link up. The port gates that dial on AutoRcn: explicit Ctrl-R connect and
// sending in a room remain the ways to bring a hub up by hand.
//
// ConnectAsync flips the status to CONNECTING synchronously and then notifies
// the manager before it spawns the connect worker, so the notification is the
// observable that cannot race with that worker.
func TestSelectHubRowAutoConnectsWhenAutoRcnOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        int
		autoReconnect bool
		wantNotify    bool
		wantStatus    int
	}{
		{name: "disconnected hub with AutoRcn on is auto-connected", status: rrc.StatusDisconnected, autoReconnect: true, wantNotify: true, wantStatus: rrc.StatusConnecting},
		{name: "failed hub with AutoRcn on is auto-connected", status: rrc.StatusFailed, autoReconnect: true, wantNotify: true, wantStatus: rrc.StatusConnecting},
		{name: "connected hub with AutoRcn on is left alone", status: rrc.StatusConnected, autoReconnect: true, wantNotify: false, wantStatus: rrc.StatusConnected},
		{name: "disconnected hub with AutoRcn off is left alone", status: rrc.StatusDisconnected, wantNotify: false, wantStatus: rrc.StatusDisconnected},
		{name: "failed hub with AutoRcn off is left alone", status: rrc.StatusFailed, wantNotify: false, wantStatus: rrc.StatusFailed},
		{name: "connected hub with AutoRcn off is left alone", status: rrc.StatusConnected, wantNotify: false, wantStatus: rrc.StatusConnected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := rrc.NewManager(tempDir(t), nil)
			a := &app.App{RRC: mgr}
			hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "RNS Community")
			hub.AutoReconnect = tt.autoReconnect
			hub.Status = tt.status

			var statuses statusCapture
			mgr.SetChangeCallback(func() {
				// Snapshot, never hub.Status: this callback also runs on the
				// connect worker goroutine, which writes the status under the
				// hub lock.
				status, _, _, _ := hub.Snapshot()
				statuses.append(status)
			})
			// Stop the connect worker before the test returns: with no
			// transport configured it fails, and a failed hub schedules
			// reconnects, which would keep notifying (and appending) for the
			// life of the test binary.
			t.Cleanup(mgr.Shutdown)

			selectHubRow(a, 0)

			if got := mgr.ActiveHub(); got != hub {
				t.Errorf("active hub after selecting hub row 0 = %v, want the selected hub", got)
			}
			seen := statuses.snapshot()
			if tt.wantNotify {
				if len(seen) == 0 {
					t.Fatal("selecting a disconnected/failed hub row with AutoRcn on did not start a connect")
				}
				if seen[0] != tt.wantStatus {
					t.Errorf("status at the connect notification = %v, want %v", seen[0], tt.wantStatus)
				}
				return
			}
			if len(seen) != 0 {
				t.Errorf("selecting a hub row with status %v and AutoRcn %v fired %v change notification(s) with statuses %v, want none",
					tt.status, tt.autoReconnect, len(seen), seen)
			}
			if got := hub.GetHubStatus(); got != tt.status {
				t.Errorf("hub status after selecting its row = %v, want %v (unchanged)", got, tt.status)
			}
		})
	}
}

// TestSelectHubRowIgnoresUnknownHub pins the bounds guard: an index with no
// hub must not panic and must not touch the manager's active hub.
func TestSelectHubRowIgnoresUnknownHub(t *testing.T) {
	t.Parallel()

	mgr := rrc.NewManager(tempDir(t), nil)
	a := &app.App{RRC: mgr}

	selectHubRow(a, 7)

	if got := mgr.ActiveHub(); got != nil {
		t.Errorf("active hub after selecting an out-of-range hub row = %v, want nil", got)
	}
}

// Selecting a room row shows that room and makes it the hub's active room; it
// dials the hub only when the hub's AutoRcn toggle is on, exactly like the
// hub-row path. Python's _select_room auto-connects through the ungated
// _maybe_autoconnect (Channels.py:1722-1741), so clicking a room of a hub the
// user had taken off automatic connection still brought the link up.
func TestSelectRoomRowGatedByAutoRcn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		autoReconnect bool
		wantConnect   bool
	}{
		{name: "room row of a hub with AutoRcn on auto-connects the hub", autoReconnect: true, wantConnect: true},
		{name: "room row of a hub with AutoRcn off leaves the hub down", wantConnect: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := rrc.NewManager(tempDir(t), nil)
			a := &app.App{RRC: mgr}
			hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "RNS Community")
			hub.AutoReconnect = tt.autoReconnect
			hub.AddRoom("general")
			hub.Status = rrc.StatusDisconnected

			var statuses statusCapture
			mgr.SetChangeCallback(func() {
				status, _, _, _ := hub.Snapshot()
				statuses.append(status)
			})
			// The connect worker must be stopped before the test returns: with
			// no transport configured it fails, and a failed hub schedules
			// reconnects that would keep notifying for the life of the binary.
			t.Cleanup(mgr.Shutdown)

			if got := selectRoomRow(a, 0, "general"); got != hub {
				t.Fatalf("selectRoomRow returned %v, want the hub at index 0", got)
			}

			if got := mgr.ActiveHub(); got != hub {
				t.Errorf("active hub after selecting a room row = %v, want the selected hub", got)
			}
			if got := mgr.ActiveRoom(); got != "general" {
				t.Errorf("active room after selecting room row %q = %q, want %q", "general", got, "general")
			}
			seen := statuses.snapshot()
			if !tt.wantConnect {
				if len(seen) != 0 {
					t.Errorf("selecting a room row on a hub with AutoRcn off fired %v change notification(s) %v, want none", len(seen), seen)
				}
				if got := hub.GetHubStatus(); got != rrc.StatusDisconnected {
					t.Errorf("hub status after its room row was selected = %v, want %v (unchanged)", got, rrc.StatusDisconnected)
				}
				return
			}
			if len(seen) == 0 {
				t.Fatal("selecting a room row on a hub with AutoRcn on did not start a connect")
			}
			if seen[0] != rrc.StatusConnecting {
				t.Errorf("status at the connect notification = %v, want %v", seen[0], rrc.StatusConnecting)
			}
		})
	}
}

// TestSelectRoomRowIgnoresUnknownHub pins the room-row bounds guard.
func TestSelectRoomRowIgnoresUnknownHub(t *testing.T) {
	t.Parallel()

	mgr := rrc.NewManager(tempDir(t), nil)
	a := &app.App{RRC: mgr}

	if got := selectRoomRow(a, 7, "general"); got != nil {
		t.Errorf("selectRoomRow for an out-of-range index = %v, want nil", got)
	}
	if got := mgr.ActiveHub(); got != nil {
		t.Errorf("active hub after selecting an out-of-range room row = %v, want nil", got)
	}
}
