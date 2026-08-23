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
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// HubInfoArea displays hub details including MOTD, rooms, and
// available rooms, with keyboard shortcuts matching Python's
// HubInfoArea.keypress() at Channels.py:381.
type HubInfoArea struct {
	app            *App
	hubName        string
	widget         *tview.Flex
	view           *tview.TextView
	motd           string
	rooms          []string
	availableRooms []string

	// Keyboard shortcut callbacks
	OnNewHub              func()
	OnJoinRoom            func()
	OnConnect             func()
	OnDisconnect          func()
	OnToggleAutoReconnect func()
	OnEditHub             func()
	OnRemoveHub           func()
	OnToggleChannelList   func()
}

// NewHubInfoArea creates a hub detail panel for the given hub.
func NewHubInfoArea(app *App, hubName string) *HubInfoArea {
	hia := &HubInfoArea{
		app:     app,
		hubName: hubName,
	}

	hia.view = applyWheelMultiplier(tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(GetThemeColors(app.Theme)["scrollbar"]))

	hia.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(hia.view, 0, 1, false)
	hia.widget.SetBorder(true)
	hia.widget.SetTitle(fmt.Sprintf(" %v ", hubName))
	hia.widget.SetInputCapture(hia.handleKey)
	hia.refreshView()

	return hia
}

// HubName returns the hub name.
func (hia *HubInfoArea) HubName() string {
	return hia.hubName
}

// MOTD returns the hub's message of the day.
func (hia *HubInfoArea) MOTD() string {
	return hia.motd
}

// SetMOTD sets the hub's message of the day.
func (hia *HubInfoArea) SetMOTD(motd string) {
	hia.motd = motd
	hia.refreshView()
}

// Rooms returns the list of joined rooms.
func (hia *HubInfoArea) Rooms() []string {
	return hia.rooms
}

// SetRooms sets the list of joined rooms.
func (hia *HubInfoArea) SetRooms(rooms []string) {
	hia.rooms = rooms
	hia.refreshView()
}

// AvailableRooms returns the list of available (unjoined) rooms.
func (hia *HubInfoArea) AvailableRooms() []string {
	return hia.availableRooms
}

// SetAvailableRooms sets the list of available rooms.
func (hia *HubInfoArea) SetAvailableRooms(rooms []string) {
	hia.availableRooms = rooms
	hia.refreshView()
}

// Widget returns the tview primitive.
func (hia *HubInfoArea) Widget() tview.Primitive {
	return hia.widget
}

// HandleKey processes keyboard shortcuts.
// Matches Python's HubInfoArea.keypress() at Channels.py:381.
func (hia *HubInfoArea) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return hia.handleKey(event)
}

func (hia *HubInfoArea) handleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlN:
		if hia.OnNewHub != nil {
			hia.OnNewHub()
		}
		return nil
	case tcell.KeyCtrlA:
		if hia.OnJoinRoom != nil {
			hia.OnJoinRoom()
		}
		return nil
	case tcell.KeyCtrlR:
		if hia.OnConnect != nil {
			hia.OnConnect()
		}
		return nil
	case tcell.KeyCtrlW:
		if hia.OnDisconnect != nil {
			hia.OnDisconnect()
		}
		return nil
	case tcell.KeyCtrlT:
		if hia.OnToggleAutoReconnect != nil {
			hia.OnToggleAutoReconnect()
		}
		return nil
	case tcell.KeyCtrlE:
		if hia.OnEditHub != nil {
			hia.OnEditHub()
		}
		return nil
	case tcell.KeyCtrlX:
		if hia.OnRemoveHub != nil {
			hia.OnRemoveHub()
		}
		return nil
	case tcell.KeyCtrlY:
		if hia.OnToggleChannelList != nil {
			hia.OnToggleChannelList()
		}
		return nil
	}
	return event
}

// refreshView rebuilds the text display from current data.
func (hia *HubInfoArea) refreshView() {
	var sb strings.Builder
	if hia.motd != "" {
		fmt.Fprintf(&sb, "[::b]MOTD:[-] %v\n\n", hia.motd)
	}
	if len(hia.rooms) > 0 {
		sb.WriteString("[::b]Rooms:[-]\n")
		for _, r := range hia.rooms {
			fmt.Fprintf(&sb, "  #%v\n", r)
		}
		sb.WriteString("\n")
	}
	if len(hia.availableRooms) > 0 {
		sb.WriteString("[::b]Available Rooms:[-]\n")
		for _, r := range hia.availableRooms {
			fmt.Fprintf(&sb, "  #%v\n", r)
		}
	}
	if sb.Len() == 0 {
		sb.WriteString("[gray]No hub info available[-]")
	}
	hia.view.SetText(sb.String())
}
