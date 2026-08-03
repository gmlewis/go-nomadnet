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

func TestChannelRoomStoreAdd(t *testing.T) {
	t.Parallel()

	store := NewChannelRoomStore()
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "general", Members: 10, Joined: true})

	room := store.Get("hub1", "general")
	if room == nil {
		t.Fatal("Get returned nil")
	}
	if room.Members != 10 {
		t.Errorf("Members = %v, want 10", room.Members)
	}
}

func TestChannelRoomStoreDelete(t *testing.T) {
	t.Parallel()

	store := NewChannelRoomStore()
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "general"})
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "random"})

	store.Delete("hub1", "general")
	if store.Get("hub1", "general") != nil {
		t.Error("deleted room still found")
	}
	if store.Get("hub1", "random") == nil {
		t.Error("non-deleted room missing")
	}
}

func TestChannelRoomStoreByHub(t *testing.T) {
	t.Parallel()

	store := NewChannelRoomStore()
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "random"})
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "general"})
	store.Add(ChannelRoomInfo{HubAddr: "hub2", Name: "dev"})

	rooms := store.ByHub("hub1")
	if len(rooms) != 2 {
		t.Errorf("ByHub count = %v, want 2", len(rooms))
	}
	// Should be sorted by name
	if rooms[0].Name != "general" {
		t.Errorf("first = %q, want general", rooms[0].Name)
	}
}

func TestChannelRoomStoreUnreadByHub(t *testing.T) {
	t.Parallel()

	store := NewChannelRoomStore()
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "general", Unread: true})
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "random", Unread: false})
	store.Add(ChannelRoomInfo{HubAddr: "hub2", Name: "dev", Unread: true})

	if got := store.UnreadByHub("hub1"); got != 1 {
		t.Errorf("UnreadByHub('hub1') = %v, want 1", got)
	}
}

func TestChannelRoomStoreSearch(t *testing.T) {
	t.Parallel()

	store := NewChannelRoomStore()
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "general"})
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "random"})

	results := store.SearchByName("gen")
	if len(results) != 1 {
		t.Errorf("Search returned %v, want 1", len(results))
	}
}

func TestFormatRoomEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		room     ChannelRoomInfo
		contains string
	}{
		{
			name:     "joined room",
			room:     ChannelRoomInfo{Name: "general", Members: 10, Joined: true},
			contains: "#general",
		},
		{
			name:     "unread room",
			room:     ChannelRoomInfo{Name: "random", Unread: true},
			contains: "[!]",
		},
		{
			name:     "topic",
			room:     ChannelRoomInfo{Name: "dev", Topic: "Development chat"},
			contains: "Development chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatRoomEntry(tt.room)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("FormatRoomEntry() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

func TestSortedRooms(t *testing.T) {
	t.Parallel()

	store := NewChannelRoomStore()
	store.Add(ChannelRoomInfo{HubAddr: "hub2", Name: "alpha"})
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "beta"})
	store.Add(ChannelRoomInfo{HubAddr: "hub1", Name: "alpha"})

	sorted := store.SortedRooms()
	if len(sorted) != 3 {
		t.Fatalf("count = %v, want 3", len(sorted))
	}
	// Sorted by hub addr, then name
	if sorted[0].HubAddr != "hub1" || sorted[0].Name != "alpha" {
		t.Errorf("first = %s/%s, want hub1/alpha", sorted[0].HubAddr, sorted[0].Name)
	}
}
