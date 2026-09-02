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
	snapshot       *HubInfoSnapshot

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

// HubInfoSnapshot carries the whole hub-info panel state, built by the
// wiring layer from a HubView (Python _show_hub_info's hub argument).
type HubInfoSnapshot struct {
	Name        string
	Address     string
	Status      int
	StatusText  string
	ServerName  string
	MOTD        string
	AutoReconn  bool
	AutoList    bool
	AutoWho     bool
	JoinedRooms []string
	AvailRooms  []string
}

// HubStatusLabels map the HubView status ints to Python's status labels
// (Channels.py:1747-1752).
var HubStatusLabels = map[int]string{
	0: "Disconnected",
	1: "Connecting",
	2: "Connected",
	3: "Failed",
}

// SetHubInfo repopulates the panel from a snapshot and re-renders.
func (hia *HubInfoArea) SetHubInfo(snap HubInfoSnapshot) {
	hia.hubName = snap.Name
	hia.motd = snap.MOTD
	hia.rooms = snap.JoinedRooms
	hia.availableRooms = snap.AvailRooms
	hia.snapshot = &snap
	hia.widget.SetTitle(fmt.Sprintf(" %v ", snap.Name))
	hia.refreshView()
}

// refreshView rebuilds the text display from current data, mirroring
// Python's _show_hub_info (Channels.py:1745-1816): the header block, the
// status line, the server line, the auto toggles, the divider, the status
// hint, the MOTD, and the joined/available rooms.
func (hia *HubInfoArea) refreshView() {
	snap := hia.snapshot
	if snap == nil {
		hia.view.SetText("[gray]No hub info available[-]")
		return
	}
	colors := GetThemeColors(hia.app.Theme)
	glyphs := hia.app.Glyphs
	check, cross := glyphs["check"], glyphs["cross"]

	var sb strings.Builder
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "  Hub      : %v\n", snap.Name)
	fmt.Fprintf(&sb, "  Address  : %v\n", snap.Address)
	statusLabel := HubStatusLabels[snap.Status]
	statusColor := colors["list_unknown"]
	switch snap.Status {
	case 1:
		statusColor = colors["list_unresponsive"]
	case 2:
		statusColor = colors["connected_status"]
	case 3:
		statusColor = colors["list_untrusted"]
	}
	fmt.Fprintf(&sb, "  [#%06x]Status   : %v (%v)[-]\n", uint32(statusColor), statusLabel, snap.StatusText)
	if snap.ServerName != "" {
		fmt.Fprintf(&sb, "  Server   : %v\n", snap.ServerName)
	}

	autoLine := func(label string, on bool, key string) {
		glyph, attr := cross, "list_unknown"
		state := "Off"
		if on {
			glyph, attr, state = check, "list_trusted", "On"
		}
		fmt.Fprintf(&sb, "  [%v]%v  : %v %v  (%v to toggle)[-]\n", attr, label, glyph, state, key)
	}
	autoLine("AutoRcn ", snap.AutoReconn, "Ctrl-T")
	autoLine("AutoList", snap.AutoList, "Ctrl-E")
	autoLine("AutoWho ", snap.AutoWho, "Ctrl-E")
	sb.WriteString(hia.app.Glyphs["divider1"] + "\n")

	switch snap.Status {
	case 2:
		sb.WriteString("  Connected. Use Ctrl-A to add a room.\n")
	case 1:
		sb.WriteString("[#afafaf]  Connecting...[-]\n")
	default:
		sb.WriteString("  Use Ctrl-R to connect.\n")
	}

	if snap.MOTD != "" {
		sb.WriteString(hia.app.Glyphs["divider1"] + "\n  MOTD:\n")
		for _, line := range strings.Split(snap.MOTD, "\n") {
			fmt.Fprintf(&sb, "  %v\n", line)
		}
	}

	if len(snap.JoinedRooms) > 0 {
		sb.WriteString(hia.app.Glyphs["divider1"] + "\n  Joined rooms:\n")
		for _, r := range snap.JoinedRooms {
			fmt.Fprintf(&sb, "    #%v\n", r)
		}
	}
	if len(snap.AvailRooms) > 0 {
		sb.WriteString(hia.app.Glyphs["divider1"] + "\n  Available rooms:\n")
		for _, r := range snap.AvailRooms {
			fmt.Fprintf(&sb, "    #%v\n", r)
		}
	}
	hia.view.SetText(sb.String())
}
