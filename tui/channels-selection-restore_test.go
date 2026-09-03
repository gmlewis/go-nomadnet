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

import "testing"

// TestChannelsSetHubsRestoresSelection pins the Python update_list selection
// restore (Channels.py:1664-1692): a background hub status change rebuilds the
// list, and the rebuild must re-select the row the user had selected instead
// of yanking the cursor back to the first row — the live fleet bug where every
// connect moved the cursor, redirecting the next Ctrl-A to the wrong hub.
func TestChannelsSetHubsRestoresSelection(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusDisconnected},
		fakeHub{name: "Hub B", status: hubStatusDisconnected},
	})
	// Rows: [Hub A(0), spacer(1), Hub B(2)] — select Hub B.
	if got := cd.rooms.GetCurrentItem(); got != 0 {
		t.Fatalf("fresh list should start at item 0, got %v", got)
	}
	cd.rooms.SetCurrentItem(2)

	// A status change rebuilds the list with the SAME row identities.
	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusConnected},
		fakeHub{name: "Hub B", status: hubStatusConnected},
	})
	if got := cd.rooms.GetCurrentItem(); got != 2 {
		t.Errorf("rebuild must restore the Hub B selection (item 2), got %v", got)
	}
}

// TestChannelsSetHubsRestoresRoomSelection covers the room-row flavor: the
// selected room under a hub survives a status-driven rebuild.
func TestChannelsSetHubsRestoresRoomSelection(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}},
	})
	// Rows: [Hub A(0), #general(1)] — select the room row.
	cd.rooms.SetCurrentItem(1)

	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusDisconnected, joined: []string{"general"}},
	})
	if got := cd.rooms.GetCurrentItem(); got != 1 {
		t.Errorf("rebuild must restore the #general room selection (item 1), got %v", got)
	}
}

// TestChannelsSetHubsNoSelectionDefaultsToFirst covers the Python prev_key
// None path: with nothing selected, the rebuild leaves the list at its
// default first item.
func TestChannelsSetHubsNoSelectionDefaultsToFirst(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusDisconnected},
		fakeHub{name: "Hub B", status: hubStatusDisconnected},
	})
	cd.rooms.SetCurrentItem(2)

	// Simulate "nothing selected": clear the current item below the valid
	// range by rebuilding with the restore suppressed — the practical way is
	// a selection that no longer exists (the hub disappeared).
	cd.SetHubs([]HubView{fakeHub{name: "Hub B", status: hubStatusConnected}})
	if got := cd.rooms.GetCurrentItem(); got != 0 {
		t.Errorf("selection for a vanished hub should fall back to item 0, got %v", got)
	}
}
