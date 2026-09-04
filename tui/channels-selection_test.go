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
)

// The 2026-09-03 20:5x fleet captures: with a room open, Python's channels
// list carries its selection ON the opened room's row (the visible highlight
// the user reads as "the joined room is highlighted"), while gonomadnet's
// selection stayed on the hub row. Python's invariant is selected_key: every
// path that shows a room sets the room row as the selected one (Channels.py
// _select_room / update_list's select_item), and every path that shows a hub
// info panel selects the hub row (_select_hub).

// fakeHubView is a minimal HubView for the selection tests.
type fakeHubView struct {
	name   string
	status int
	rooms  []string
}

func (f *fakeHubView) Name() string                { return f.name }
func (f *fakeHubView) Status() int                 { return f.status }
func (f *fakeHubView) JoinedRooms() []string       { return f.rooms }
func (f *fakeHubView) MessageRooms() []string      { return nil }
func (f *fakeHubView) UnreadRooms() []string       { return nil }
func (f *fakeHubView) MentionRooms() []string      { return nil }
func (f *fakeHubView) AddressHex() string          { return "abc" }
func (f *fakeHubView) StatusText() string          { return "Connected" }
func (f *fakeHubView) ServerName() string          { return f.name }
func (f *fakeHubView) HubVersion() string          { return "0.3.2" }
func (f *fakeHubView) MOTD() string                { return "" }
func (f *fakeHubView) AutoReconnect() bool         { return true }
func (f *fakeHubView) AutoList() bool              { return true }
func (f *fakeHubView) AutoWho() bool               { return true }
func (f *fakeHubView) AvailableRoomList() []string { return nil }

func newSelectionCD(t *testing.T) *ChannelsDisplay {
	t.Helper()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)
	cd.app = app
	hubs := []HubView{
		&fakeHubView{name: "RNS Community", status: hubStatusDisconnected},
		&fakeHubView{name: "RaspPi Local Hub", status: hubStatusConnected, rooms: []string{"test", "test3", "test4"}},
	}
	cd.SetHubs(hubs)
	return cd
}

// roomRowIndex finds the list index of the given hub's room row.
func roomRowIndex(t *testing.T, cd *ChannelsDisplay, hubIdx int, room string) int {
	t.Helper()
	for i, e := range cd.hubEntries {
		if e.Kind == RowRoom && e.HubIdx == hubIdx && e.Room == room {
			return i
		}
	}
	t.Fatalf("no list row for hub %v room %q", hubIdx, room)
	return -1
}

// hubRowIndex finds the list index of the given hub's hub row.
func hubRowIndex(cd *ChannelsDisplay, hubIdx int) int {
	for i, e := range cd.hubEntries {
		if e.Kind == RowHub && e.HubIdx == hubIdx {
			return i
		}
	}
	return -1
}

func TestChannelsShowRoomSelectsRoomRow(t *testing.T) {
	t.Parallel()

	cd := newSelectionCD(t)
	// The selection starts wherever the rebuild left it (the first hub row).
	cd.rooms.SetCurrentItem(0)

	cd.ShowRoom(1, "test4", nil)

	want := roomRowIndex(t, cd, 1, "test4")
	if got := cd.rooms.GetCurrentItem(); got != want {
		t.Errorf("list selection after ShowRoom = %v, want %v (the opened room's row carries the highlight, Python selected_key)", got, want)
	}
}

// TestChannelsShowRoomSelectionSurvivesRebuild pins the full user flow: the
// room view is shown, then a background hub refresh rebuilds the list — the
// selection must stay on the opened room's row (Python update_list's
// prev_key restore, which now keys on the room the view shows).
func TestChannelsShowRoomSelectionSurvivesRebuild(t *testing.T) {
	t.Parallel()

	cd := newSelectionCD(t)
	cd.ShowRoom(1, "test4", nil)
	want := roomRowIndex(t, cd, 1, "test4")
	if got := cd.rooms.GetCurrentItem(); got != want {
		t.Fatalf("list selection after ShowRoom = %v, want %v", got, want)
	}

	// A background refresh (message arrival → SetHubs) rebuilds the list.
	cd.SetHubs([]HubView{
		&fakeHubView{name: "RNS Community", status: hubStatusConnected},
		&fakeHubView{name: "RaspPi Local Hub", status: hubStatusConnected, rooms: []string{"test", "test3", "test4"}},
	})
	if got := cd.rooms.GetCurrentItem(); got != want {
		t.Errorf("list selection after rebuild = %v, want %v (the shown room stays selected)", got, want)
	}
}

func TestChannelsShowHubInfoSelectsHubRow(t *testing.T) {
	t.Parallel()

	cd := newSelectionCD(t)
	cd.rooms.SetCurrentItem(0)

	cd.ShowHubInfo(1)
	want := hubRowIndex(cd, 1)
	if want < 0 {
		t.Fatal("no hub row for hub 1")
	}
	if got := cd.rooms.GetCurrentItem(); got != want {
		t.Errorf("list selection after ShowHubInfo = %v, want %v (the info panel's hub row is selected, Python _select_hub)", got, want)
	}
}
