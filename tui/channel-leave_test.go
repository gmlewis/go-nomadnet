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

// The leave-flow parity (Python Channels.py:1044-1053 and 1120-1126): parting
// the SHOWN room returns the right pane to the placeholder; parting another
// room keeps the shown room open. The composer's "/part [room]" and the Ctrl-X
// leave both route through ChannelsDisplay.LeaveRoom.

// leaveCD builds a ChannelsDisplay with a connected hub and an open room.
func leaveCD(t *testing.T) (*App, *ChannelsDisplay, *[]string) {
	t.Helper()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)
	left := &[]string{}
	cd.OnLeaveRoom = func(room string) { *left = append(*left, room) }
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)
	return app, cd, left
}

// TestLeaveRoomShownRoomShowsPlaceholder pins Python's /part and leave_room
// tail: parting the currently shown room ends with show_placeholder.
func TestLeaveRoomShownRoomShowsPlaceholder(t *testing.T) {
	t.Parallel()

	_, cd, left := leaveCD(t)
	if cd.paneMode != "room" {
		t.Fatalf("paneMode = %q, want room after ShowRoom", cd.paneMode)
	}

	cd.LeaveRoom("general")

	if want := []string{"general"}; len(*left) != 1 || (*left)[0] != want[0] {
		t.Errorf("OnLeaveRoom targets = %v, want %v", *left, want)
	}
	if cd.paneMode == "room" {
		t.Error("paneMode still room after parting the shown room (Python show_placeholder, Channels.py:1050)")
	}
}

// TestLeaveRoomOtherRoomKeepsRoom pins the /part <other-room> branch: the
// shown room stays open (Python: placeholder only when target == self.room).
func TestLeaveRoomOtherRoomKeepsRoom(t *testing.T) {
	t.Parallel()

	_, cd, left := leaveCD(t)

	cd.LeaveRoom("otherroom")

	if want := []string{"otherroom"}; len(*left) != 1 || (*left)[0] != want[0] {
		t.Errorf("OnLeaveRoom targets = %v, want %v", *left, want)
	}
	if cd.paneMode != "room" {
		t.Errorf("paneMode = %q, want room kept after parting another room", cd.paneMode)
	}
}
