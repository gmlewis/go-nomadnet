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
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestHubInfoAreaCreation(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	hia := NewHubInfoArea(app, "MyHub")
	if hia == nil {
		t.Fatal("NewHubInfoArea returned nil")
	}
}

func TestHubInfoAreaHubName(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	hia := NewHubInfoArea(app, "MyHub")
	if hia.HubName() != "MyHub" {
		t.Errorf("HubName = %q, want %q", hia.HubName(), "MyHub")
	}
}

func TestHubInfoAreaKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	hia := NewHubInfoArea(app, "MyHub")

	var fired []string
	hia.OnNewHub = func() { fired = append(fired, "new_hub") }
	hia.OnJoinRoom = func() { fired = append(fired, "join_room") }
	hia.OnConnect = func() { fired = append(fired, "connect") }
	hia.OnDisconnect = func() { fired = append(fired, "disconnect") }
	hia.OnToggleAutoReconnect = func() { fired = append(fired, "auto_reconnect") }
	hia.OnEditHub = func() { fired = append(fired, "edit_hub") }
	hia.OnRemoveHub = func() { fired = append(fired, "remove_hub") }
	hia.OnToggleChannelList = func() { fired = append(fired, "toggle_list") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-n", tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModNone), "new_hub"},
		{"ctrl-a", tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone), "join_room"},
		{"ctrl-r", tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone), "connect"},
		{"ctrl-w", tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone), "disconnect"},
		{"ctrl-t", tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModNone), "auto_reconnect"},
		{"ctrl-e", tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "edit_hub"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "remove_hub"},
		{"ctrl-y", tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone), "toggle_list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := hia.HandleKey(tt.event)
			if result != nil {
				t.Errorf("key %s was not consumed", tt.name)
			}
			if len(fired) != 1 || fired[0] != tt.want {
				t.Errorf("key %s fired %v, want [%s]", tt.name, fired, tt.want)
			}
		})
	}
}

func TestHubInfoAreaSetMOTD(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	hia := NewHubInfoArea(app, "MyHub")

	hia.SetMOTD("Welcome to the hub!")
	if hia.MOTD() != "Welcome to the hub!" {
		t.Errorf("MOTD = %q, want %q", hia.MOTD(), "Welcome to the hub!")
	}
}

func TestHubInfoAreaSetRooms(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	hia := NewHubInfoArea(app, "MyHub")

	rooms := []string{"general", "random", "dev"}
	hia.SetRooms(rooms)
	if len(hia.Rooms()) != 3 {
		t.Errorf("Rooms count = %d, want 3", len(hia.Rooms()))
	}
}

func TestHubInfoAreaSetAvailableRooms(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	hia := NewHubInfoArea(app, "MyHub")

	available := []string{"general", "random", "dev", "test"}
	hia.SetAvailableRooms(available)
	if len(hia.AvailableRooms()) != 4 {
		t.Errorf("AvailableRooms count = %d, want 4", len(hia.AvailableRooms()))
	}
}
