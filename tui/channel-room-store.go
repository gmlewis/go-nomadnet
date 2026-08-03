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
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ChannelRoomStore manages chat rooms across all hubs.
// Thread-safe for concurrent access.
type ChannelRoomStore struct {
	mu    sync.RWMutex
	rooms map[string]*ChannelRoomInfo
	order []string
}

// ChannelRoomInfo holds room state for display.
type ChannelRoomInfo struct {
	HubAddr string
	Name    string
	Topic   string
	Members int
	Joined  bool
	Unread  bool
}

// NewChannelRoomStore creates an empty channel room store.
func NewChannelRoomStore() *ChannelRoomStore {
	return &ChannelRoomStore{rooms: make(map[string]*ChannelRoomInfo)}
}

// Add adds or updates a room entry.
func (crs *ChannelRoomStore) Add(info ChannelRoomInfo) {
	crs.mu.Lock()
	defer crs.mu.Unlock()

	key := info.HubAddr + "/" + info.Name
	crs.rooms[key] = &info
	crs.order = append(crs.order, key)
}

// Get returns the room entry, or nil if not found.
func (crs *ChannelRoomStore) Get(hubAddr, name string) *ChannelRoomInfo {
	crs.mu.RLock()
	defer crs.mu.RUnlock()
	key := hubAddr + "/" + name
	return crs.rooms[key]
}

// Delete removes a room entry.
func (crs *ChannelRoomStore) Delete(hubAddr, name string) {
	crs.mu.Lock()
	defer crs.mu.Unlock()
	key := hubAddr + "/" + name
	delete(crs.rooms, key)
	for i, k := range crs.order {
		if k == key {
			crs.order = append(crs.order[:i], crs.order[i+1:]...)
			break
		}
	}
}

// List returns all rooms in insertion order.
func (crs *ChannelRoomStore) List() []ChannelRoomInfo {
	crs.mu.RLock()
	defer crs.mu.RUnlock()

	out := make([]ChannelRoomInfo, 0, len(crs.order))
	for _, key := range crs.order {
		if r, ok := crs.rooms[key]; ok {
			out = append(out, *r)
		}
	}
	return out
}

// ByHub returns rooms for a specific hub, sorted by name.
func (crs *ChannelRoomStore) ByHub(hubAddr string) []ChannelRoomInfo {
	crs.mu.RLock()
	defer crs.mu.RUnlock()

	var result []ChannelRoomInfo
	for _, r := range crs.rooms {
		if r.HubAddr == hubAddr {
			result = append(result, *r)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// UnreadByHub returns unread room count for a specific hub.
func (crs *ChannelRoomStore) UnreadByHub(hubAddr string) int {
	crs.mu.RLock()
	defer crs.mu.RUnlock()
	n := 0
	for _, r := range crs.rooms {
		if r.HubAddr == hubAddr && r.Unread {
			n++
		}
	}
	return n
}

// SearchByName returns rooms whose name contains the query.
func (crs *ChannelRoomStore) SearchByName(query string) []ChannelRoomInfo {
	all := crs.List()
	if query == "" {
		return all
	}
	q := strings.ToLower(query)
	var result []ChannelRoomInfo
	for _, r := range all {
		if strings.Contains(strings.ToLower(r.Name), q) {
			result = append(result, r)
		}
	}
	return result
}

// FormatRoomEntry formats a room entry for display in the channel list.
func FormatRoomEntry(room ChannelRoomInfo) string {
	prefix := "  "
	if room.Unread {
		prefix = "[!] "
	} else if room.Joined {
		prefix = "[*] "
	}
	text := fmt.Sprintf("%s#%s", prefix, room.Name)
	secondary := fmt.Sprintf("%v members — %s", room.Members, room.Topic)
	return text + "\n" + secondary
}

// SortedRooms returns rooms sorted by hub name, then room name.
func (crs *ChannelRoomStore) SortedRooms() []ChannelRoomInfo {
	rooms := crs.List()
	sort.SliceStable(rooms, func(i, j int) bool {
		if rooms[i].HubAddr != rooms[j].HubAddr {
			return rooms[i].HubAddr < rooms[j].HubAddr
		}
		return rooms[i].Name < rooms[j].Name
	})
	return rooms
}
