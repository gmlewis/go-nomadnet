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

package app

// HubView is the read-only view of an RRC hub that rendering surfaces display.
// Defining this interface in package app decouples the core application and
// RRC manager from terminal UI widget dependencies (e.g. tview/tcell), allowing
// headless and embedded targets to compile without UI packages.
type HubView interface {
	Name() string
	Status() int
	JoinedRooms() []string
	MessageRooms() []string
	UnreadRooms() []string
	MentionRooms() []string

	// Hub info fields.
	AddressHex() string
	StatusText() string
	ServerName() string
	HubVersion() string
	MOTD() string
	AutoReconnect() bool
	AutoList() bool
	AutoWho() bool
	AvailableRoomList() []string
}
