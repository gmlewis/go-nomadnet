// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package app

import (
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/gmlewis/go-nomadnet/tui"
)

// TestHubViews pins App.HubViews: the RRC manager's hubs are adapted to the
// tui.HubView interface the channels list renders, reading the live hub state
// (name/status/rooms/messages/unread/mentions) through locked accessors. The
// Status() int matches the rrc.Status* enum (the tui HubView contract).
func TestHubViews(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)
	a.RRC = rrc.NewManager(a.StoragePath, nil)

	hub1 := a.RRC.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "Hub 1")
	hub1.SetStatus(rrc.StatusConnected, "ok")
	hub1.AddRoom("random")
	hub1.AddRoom("general")
	// Mutate exported room state directly. No RRC worker goroutine is running
	// (we never called Connect), so this is race-free and avoids needing the
	// unexported hub lock from outside the rrc package.
	hub1.Messages["zzz"] = []*rrc.RRCMessage{{Text: "hi"}}
	hub1.UnreadRooms["onlymsg"] = true
	hub1.MentionRooms["general"] = true

	a.RRC.AddHub([]byte{0x05, 0x06, 0x07, 0x08}, "rrc.hub", "Hub 2")

	views := a.HubViews()
	if len(views) != 2 {
		t.Fatalf("HubViews len = %v, want 2", len(views))
	}
	if got := views[0].Name(); got != "Hub 1" {
		t.Errorf("views[0].Name = %q, want %q", got, "Hub 1")
	}
	if got := views[0].Status(); got != rrc.StatusConnected {
		t.Errorf("views[0].Status = %v, want %v", got, rrc.StatusConnected)
	}
	if got, want := views[0].JoinedRooms(), []string{"general", "random"}; !sliceEqual(got, want) {
		t.Errorf("views[0].JoinedRooms = %v, want %v", got, want)
	}
	if got, want := views[0].MessageRooms(), []string{"general", "random", "zzz"}; !sliceEqual(got, want) {
		t.Errorf("views[0].MessageRooms = %v, want %v", got, want)
	}
	if got, want := views[0].UnreadRooms(), []string{"onlymsg"}; !sliceEqual(got, want) {
		t.Errorf("views[0].UnreadRooms = %v, want %v", got, want)
	}
	if got, want := views[0].MentionRooms(), []string{"general"}; !sliceEqual(got, want) {
		t.Errorf("views[0].MentionRooms = %v, want %v", got, want)
	}
	if got := views[1].Name(); got != "Hub 2" {
		t.Errorf("views[1].Name = %q, want %q", got, "Hub 2")
	}
	if got := views[1].Status(); got != rrc.StatusDisconnected {
		t.Errorf("views[1].Status = %v, want %v", got, rrc.StatusDisconnected)
	}

	// Confirm the returned values satisfy the tui.HubView interface.
	var _ tui.HubView = views[0]
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
