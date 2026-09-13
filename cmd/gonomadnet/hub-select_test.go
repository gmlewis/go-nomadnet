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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-reticulum/rrc"
)

// Selecting a hub row in the channels list must make the hub active, show its
// info panel, and auto-connect it when it is disconnected or failed — Python's
// ChannelsDisplay._select_hub runs _maybe_autoconnect before _show_hub_info
// (Channels.py:1716-1720, 1736-1741). The Go port wired only the room half of
// that pair, so after the 2026-09-13 incident left the RNS Community hub
// disconnected for three hours, selecting the hub row — the obvious recovery
// gesture — did nothing and only a send attempt or /connect brought it back.
//
// ConnectAsync flips the status to CONNECTING synchronously and then notifies
// the manager before it spawns the connect worker, so the notification is the
// observable that cannot race with that worker.
func TestSelectHubRowAutoConnects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		wantNotify bool
		wantStatus int
	}{
		{name: "disconnected hub is auto-connected", status: rrc.StatusDisconnected, wantNotify: true, wantStatus: rrc.StatusConnecting},
		{name: "failed hub is auto-connected", status: rrc.StatusFailed, wantNotify: true, wantStatus: rrc.StatusConnecting},
		{name: "connected hub is left alone", status: rrc.StatusConnected, wantNotify: false, wantStatus: rrc.StatusConnected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := rrc.NewManager(tempDir(t), nil)
			a := &app.App{RRC: mgr}
			hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "RNS Community")
			hub.Status = tt.status

			var statuses []int
			mgr.SetChangeCallback(func() {
				statuses = append(statuses, hub.Status)
			})

			selectHubRow(a, 0)

			if got := mgr.ActiveHub(); got != hub {
				t.Errorf("active hub after selecting hub row 0 = %v, want the selected hub", got)
			}
			if tt.wantNotify {
				if len(statuses) == 0 {
					t.Fatal("selecting a disconnected/failed hub row did not start a connect (Python _select_hub → _maybe_autoconnect)")
				}
				if statuses[0] != tt.wantStatus {
					t.Errorf("status at the connect notification = %v, want %v", statuses[0], tt.wantStatus)
				}
				return
			}
			if len(statuses) != 0 {
				t.Errorf("selecting a connected hub row fired %v change notification(s) with statuses %v, want none", len(statuses), statuses)
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
