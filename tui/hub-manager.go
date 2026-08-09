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

// HubStatus represents the connection state of an RRC hub.
type HubStatus string

const (
	HubConnected    HubStatus = "connected"
	HubDisconnected HubStatus = "disconnected"
	HubReconnecting HubStatus = "reconnecting"
)

// StatusIcon returns a single-character icon for the hub status.
func StatusIcon(status HubStatus) string {
	switch status {
	case HubConnected:
		return "●"
	case HubReconnecting:
		return "◌"
	default:
		return "○"
	}
}

// HubRoom holds state for a single room within a hub.
type HubRoom struct {
	Name   string
	Joined bool
	Unread bool
}

// HubEntry holds all state for a single RRC hub connection.
type HubEntry struct {
	Address       string
	Name          string
	Status        HubStatus
	AutoReconnect bool
	Rooms         map[string]*HubRoom

	// mu guards the Rooms map. The HubManager-level mu (hm.mu) protects the
	// hubs collection and is acquired before hub.mu in every path that holds
	// both (AddRoom/RemoveRoom/TotalUnreadCount), so the lock order is always
	// hm.mu → hub.mu and never the reverse — no deadlock. Standalone HubEntry
	// accessors (GetRoom/UnreadCount/HubSummary/FormatHubStatus) acquire only
	// hub.mu, which synchronizes them against AddRoom/RemoveRoom writes that
	// mutate Rooms from a background RRC-event goroutine. Without this lock,
	// a concurrent AddRoom/RemoveRoom write and a UI-side range/len/read of
	// Rooms triggers "fatal error: concurrent map read/write" /
	// "concurrent map iteration and map write".
	mu sync.RWMutex
}

// GetRoom returns the room by name, or nil if not found.
func (h *HubEntry) GetRoom(name string) *HubRoom {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Rooms[name]
}

// UnreadCount returns the number of rooms with unread messages.
func (h *HubEntry) UnreadCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, r := range h.Rooms {
		if r.Unread {
			n++
		}
	}
	return n
}

// HubManager manages the collection of RRC hubs.
// Thread-safe for concurrent access.
type HubManager struct {
	mu    sync.RWMutex
	hubs  map[string]*HubEntry
	order []string // insertion order for stable listing
}

// NewHubManager creates an empty hub manager.
func NewHubManager() *HubManager {
	return &HubManager{hubs: make(map[string]*HubEntry)}
}

// AddHub creates a new hub entry. Returns a pointer for mutation.
func (hm *HubManager) AddHub(address, name string) *HubEntry {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hub := &HubEntry{
		Address: address,
		Name:    name,
		Status:  HubDisconnected,
		Rooms:   make(map[string]*HubRoom),
	}
	hm.hubs[address] = hub
	hm.order = append(hm.order, address)
	return hub
}

// GetHub returns the hub by address, or nil.
func (hm *HubManager) GetHub(address string) *HubEntry {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.hubs[address]
}

// DeleteHub removes a hub. No-op if not found.
func (hm *HubManager) DeleteHub(address string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	delete(hm.hubs, address)
	for i, a := range hm.order {
		if a == address {
			hm.order = append(hm.order[:i], hm.order[i+1:]...)
			break
		}
	}
}

// SetStatus updates the connection status for a hub.
func (hm *HubManager) SetStatus(address string, status HubStatus) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	if h, ok := hm.hubs[address]; ok {
		h.Status = status
	}
}

// AddRoom adds a room to a hub. Returns the room entry.
func (hm *HubManager) AddRoom(hubAddr, roomName string) *HubRoom {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hub, ok := hm.hubs[hubAddr]
	if !ok {
		return nil
	}
	room := &HubRoom{Name: roomName}
	hub.mu.Lock()
	hub.Rooms[roomName] = room
	hub.mu.Unlock()
	return room
}

// RemoveRoom removes a room from a hub.
func (hm *HubManager) RemoveRoom(hubAddr, roomName string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if hub, ok := hm.hubs[hubAddr]; ok {
		hub.mu.Lock()
		delete(hub.Rooms, roomName)
		hub.mu.Unlock()
	}
}

// ListHubs returns all hubs in insertion order.
func (hm *HubManager) ListHubs() []*HubEntry {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	out := make([]*HubEntry, 0, len(hm.order))
	for _, addr := range hm.order {
		if h, ok := hm.hubs[addr]; ok {
			out = append(out, h)
		}
	}
	return out
}

// SortedHubs returns hubs sorted by (status desc, name asc).
func (hm *HubManager) SortedHubs() []*HubEntry {
	hubs := hm.ListHubs()

	sort.SliceStable(hubs, func(i, j int) bool {
		si := hubStatusOrder(hubs[i].Status)
		sj := hubStatusOrder(hubs[j].Status)
		if si != sj {
			return si < sj
		}
		if hubs[i].Name != hubs[j].Name {
			return hubs[i].Name < hubs[j].Name
		}
		return hubs[i].Address < hubs[j].Address
	})

	return hubs
}

func hubStatusOrder(s HubStatus) int {
	switch s {
	case HubConnected:
		return 0
	case HubReconnecting:
		return 1
	default:
		return 2
	}
}

// ConnectedCount returns the number of connected hubs.
func (hm *HubManager) ConnectedCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	n := 0
	for _, h := range hm.hubs {
		if h.Status == HubConnected {
			n++
		}
	}
	return n
}

// TotalUnreadCount returns the total unread count across all hubs.
func (hm *HubManager) TotalUnreadCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	n := 0
	for _, h := range hm.hubs {
		h.mu.RLock()
		for _, r := range h.Rooms {
			if r.Unread {
				n++
			}
		}
		h.mu.RUnlock()
	}
	return n
}

// HubSummary returns a formatted summary of a hub for display.
// Includes status icon, name, room count, and unread indicator.
func HubSummary(hub *HubEntry) string {
	hub.mu.RLock()
	roomCount := len(hub.Rooms)
	hub.mu.RUnlock()
	unread := hub.UnreadCount()
	icon := StatusIcon(hub.Status)

	if unread > 0 {
		return fmt.Sprintf("%v %v (%v rooms, %v unread)",
			icon, hub.Name, roomCount, unread)
	}
	return fmt.Sprintf("%v %v (%v rooms)", icon, hub.Name, roomCount)
}

// SearchHubs returns hubs whose name or address contains the query
// (case-insensitive). Empty query returns all hubs.
func (hm *HubManager) SearchHubs(query string) []*HubEntry {
	hubs := hm.ListHubs()
	if query == "" {
		return hubs
	}
	q := strings.ToLower(query)
	var result []*HubEntry
	for _, h := range hubs {
		if strings.Contains(strings.ToLower(h.Name), q) ||
			strings.Contains(strings.ToLower(h.Address), q) {
			result = append(result, h)
		}
	}
	return result
}
