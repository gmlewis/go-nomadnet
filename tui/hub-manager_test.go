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
)

func TestHubManagerAddHub(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hub := hm.AddHub("hub1", "My Hub")
	if hub.Name != "My Hub" {
		t.Errorf("Name = %q, want %q", hub.Name, "My Hub")
	}
	if hub.Address != "hub1" {
		t.Errorf("Address = %q, want %q", hub.Address, "hub1")
	}
	if hub.Status != HubDisconnected {
		t.Errorf("Status = %q, want %q", hub.Status, HubDisconnected)
	}
}

func TestHubManagerGetHub(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")

	hub := hm.GetHub("hub1")
	if hub == nil {
		t.Fatal("GetHub returned nil")
	}
	if hub.Name != "My Hub" {
		t.Errorf("Name = %q, want %q", hub.Name, "My Hub")
	}
}

func TestHubManagerGetMissing(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	if hm.GetHub("nonexistent") != nil {
		t.Error("GetHub on missing should return nil")
	}
}

func TestHubManagerDeleteHub(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")
	hm.DeleteHub("hub1")
	if hm.GetHub("hub1") != nil {
		t.Error("GetHub after delete should return nil")
	}
}

func TestHubManagerListHubs(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "Hub A")
	hm.AddHub("hub2", "Hub B")

	hubs := hm.ListHubs()
	if len(hubs) != 2 {
		t.Errorf("ListHubs() returned %d, want 2", len(hubs))
	}
}

func TestHubManagerAddRoom(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")
	hm.AddRoom("hub1", "general")
	hm.AddRoom("hub1", "random")

	hub := hm.GetHub("hub1")
	if len(hub.Rooms) != 2 {
		t.Errorf("Rooms = %d, want 2", len(hub.Rooms))
	}
}

func TestHubManagerConnectDisconnect(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")

	hm.SetStatus("hub1", HubConnected)
	if hm.GetHub("hub1").Status != HubConnected {
		t.Error("should be connected")
	}

	hm.SetStatus("hub1", HubDisconnected)
	if hm.GetHub("hub1").Status != HubDisconnected {
		t.Error("should be disconnected")
	}
}

func TestHubManagerAutoReconnect(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")

	hub := hm.GetHub("hub1")
	if hub.AutoReconnect {
		t.Error("AutoReconnect should default to false")
	}

	hub.AutoReconnect = true
	got := hm.GetHub("hub1")
	if !got.AutoReconnect {
		t.Error("AutoReconnect should be true")
	}
}

func TestHubManagerRoomJoinLeave(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")
	hm.AddRoom("hub1", "general")

	hub := hm.GetHub("hub1")
	room := hub.GetRoom("general")
	if room == nil {
		t.Fatal("GetRoom returned nil")
	}
	if room.Joined {
		t.Error("room should not be joined initially")
	}

	room.Joined = true
	room.Unread = true

	got := hub.GetRoom("general")
	if !got.Joined {
		t.Error("room should be joined")
	}
	if !got.Unread {
		t.Error("room should be unread")
	}
}

func TestHubManagerRemoveRoom(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")
	hm.AddRoom("hub1", "general")
	hm.RemoveRoom("hub1", "general")

	hub := hm.GetHub("hub1")
	if hub.GetRoom("general") != nil {
		t.Error("GetRoom after remove should return nil")
	}
}

func TestHubManagerUnreadCount(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "My Hub")
	hm.AddRoom("hub1", "general")
	hm.AddRoom("hub1", "random")

	hub := hm.GetHub("hub1")
	hub.GetRoom("general").Unread = true
	hub.GetRoom("random").Unread = true

	if hub.UnreadCount() != 2 {
		t.Errorf("UnreadCount() = %d, want 2", hub.UnreadCount())
	}
}

func TestHubManagerStatusIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status HubStatus
		want   string
	}{
		{HubConnected, "●"},
		{HubDisconnected, "○"},
		{HubReconnecting, "◌"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			got := StatusIcon(tt.status)
			if got != tt.want {
				t.Errorf("StatusIcon(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestHubManagerConnectedCount(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "Hub A")
	hm.AddHub("hub2", "Hub B")
	hm.AddHub("hub3", "Hub C")

	hm.SetStatus("hub1", HubConnected)
	hm.SetStatus("hub2", HubReconnecting)

	if got := hm.ConnectedCount(); got != 1 {
		t.Errorf("ConnectedCount() = %d, want 1", got)
	}
}

func TestHubManagerTotalUnreadCount(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("hub1", "Hub A")
	hm.AddRoom("hub1", "general")
	hm.AddRoom("hub1", "random")
	hm.AddHub("hub2", "Hub B")
	hm.AddRoom("hub2", "dev")

	hm.GetHub("hub1").GetRoom("general").Unread = true
	hm.GetHub("hub2").GetRoom("dev").Unread = true

	if got := hm.TotalUnreadCount(); got != 2 {
		t.Errorf("TotalUnreadCount() = %d, want 2", got)
	}
}

func TestHubSummary(t *testing.T) {
	t.Parallel()

	hub := &HubEntry{
		Name:   "My Hub",
		Status: HubConnected,
		Rooms: map[string]*HubRoom{
			"general": {Name: "general", Joined: true},
			"random":  {Name: "random", Joined: true, Unread: true},
		},
	}

	summary := HubSummary(hub)
	if !strings.Contains(summary, "My Hub") {
		t.Errorf("Summary missing name: %q", summary)
	}
	if !strings.Contains(summary, "2 rooms") {
		t.Errorf("Summary missing room count: %q", summary)
	}
	if !strings.Contains(summary, "1 unread") {
		t.Errorf("Summary missing unread count: %q", summary)
	}
}

func TestHubSummaryNoUnread(t *testing.T) {
	t.Parallel()

	hub := &HubEntry{
		Name:   "Hub",
		Status: HubDisconnected,
		Rooms:  map[string]*HubRoom{"x": {Name: "x"}},
	}

	summary := HubSummary(hub)
	if strings.Contains(summary, "unread") {
		t.Errorf("Summary should not mention unread: %q", summary)
	}
	if !strings.Contains(summary, "○") {
		t.Errorf("Summary missing status icon: %q", summary)
	}
}

func TestSearchHubs(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("addr1", "Alpha Hub")
	hm.AddHub("addr2", "Beta Hub")
	hm.AddHub("addr3", "Gamma Hub")

	tests := []struct {
		name  string
		query string
		count int
	}{
		{"empty", "", 3},
		{"name match", "alpha", 1},
		{"address match", "addr2", 1},
		{"partial", "hub", 3},
		{"case insensitive", "ALPHA", 1},
		{"no match", "zzz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hm.SearchHubs(tt.query)
			if len(got) != tt.count {
				t.Errorf("SearchHubs(%q) returned %d, want %d", tt.query, len(got), tt.count)
			}
		})
	}
}
