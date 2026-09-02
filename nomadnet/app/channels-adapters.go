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

// Package app adapts the core RRC manager/hubs to the tui-defined HubView
// interface the channels list renders. The tui package does not import rrc;
// this file is the seam (mirrors the SendDeps injection pattern).

package app

import (
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/gmlewis/go-nomadnet/tui"
)

// rrcHubView adapts a *rrc.RRCHub to the tui.HubView interface, reading live
// hub state through the RRCHub locked accessors so background RRC goroutines
// can mutate the hub without racing the UI render.
type rrcHubView struct {
	hub *rrc.RRCHub
}

// Name returns the hub's display name.
func (v rrcHubView) Name() string { return v.hub.GetHubName() }

// Status returns the hub's connection status as the rrc.Status* enum int
// (matches the tui HubView contract: 0=disconnected … 3=failed).
func (v rrcHubView) Status() int { return v.hub.GetHubStatus() }

// JoinedRooms returns the sorted list of joined room names.
func (v rrcHubView) JoinedRooms() []string { return v.hub.JoinedRoomList() }

// MessageRooms returns the sorted list of rooms that have message buffers.
func (v rrcHubView) MessageRooms() []string { return v.hub.MessageRoomList() }

// UnreadRooms returns the sorted list of rooms with unread messages.
func (v rrcHubView) UnreadRooms() []string { return v.hub.UnreadRoomList() }

// MentionRooms returns the sorted list of rooms with unread mentions.
func (v rrcHubView) MentionRooms() []string { return v.hub.MentionRoomList() }

// HubViews adapts the RRC manager's hubs to the tui.HubView interface the
// channels list renders. The returned slice is built from a locked snapshot
// of the hub list, so AddHub/RemoveHub mutations on the RRC worker don't race
// the UI read. Returns a non-nil empty slice when there are no hubs.
func (a *App) HubViews() []tui.HubView {
	if a.RRC == nil {
		return []tui.HubView{}
	}
	hubs := a.RRC.HubsSnapshot()
	views := make([]tui.HubView, len(hubs))
	for i, h := range hubs {
		views[i] = rrcHubView{hub: h}
	}
	return views
}

// AddressHex returns the hub's destination hash as lowercase hex.
func (v rrcHubView) AddressHex() string { return v.hub.HubAddressHex() }

// StatusText returns the detailed connection status text.
func (v rrcHubView) StatusText() string { return v.hub.GetStatusText() }

// ServerName returns the hub's advertised server name.
func (v rrcHubView) ServerName() string { return v.hub.GetHubName() }

// MOTD returns the hub's message of the day.
func (v rrcHubView) MOTD() string { return v.hub.GetMOTD() }

// AutoReconnect reports the auto-reconnect toggle state.
func (v rrcHubView) AutoReconnect() bool { return v.hub.GetAutoReconnect() }

// AutoList reports the auto room-list toggle state.
func (v rrcHubView) AutoList() bool { return v.hub.GetAutoList() }

// AutoWho reports the auto who toggle state.
func (v rrcHubView) AutoWho() bool { return v.hub.GetAutoWho() }

// AvailableRoomList returns the sorted names of the rooms the hub advertises
// but the client has not joined.
func (v rrcHubView) AvailableRoomList() []string { return v.hub.GetAvailableRoomList() }

// compile-time guard: rrcHubView satisfies tui.HubView.
var _ tui.HubView = rrcHubView{}
