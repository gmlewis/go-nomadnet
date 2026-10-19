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
	"strconv"
	"strings"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
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

	// OnOpenRoom opens a room a click on the panel asked for: a #room link in
	// the MOTD or a row in the joined/available lists (Python's
	// _ChatLinkDelegate._open_room / ChannelListEntry click signal).
	OnOpenRoom func(room string)

	// Region maps: the index in a tview region tag back to the room it wraps.
	motdLinks   []string
	joinedLinks []string
	availLinks  []string
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
		SetRegions(true).
		SetTextColor(GetThemeColors(app.Theme)["scrollbar"]))
	// The address click is wired here, once, on top of the wheel capture so both
	// keep working; refreshView only re-tags the text it writes.
	MakeAddrClickable(app, "Hub Address", hia.view, "")
	// tview hit-tests region tags on a click and highlights the one under the
	// cursor. The view holds both address regions and room-link regions, and
	// tview keeps only one highlight handler, so this handler serves both.
	hia.view.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) == 0 {
			return
		}
		id := added[0]
		// Clear the inverted region before the pane switches away.
		hia.view.Highlight()
		if hia.OnOpenRoom != nil {
			if idx, ok := regionIndex(id, "motdroom"); ok && idx < len(hia.motdLinks) {
				hia.OnOpenRoom(hia.motdLinks[idx])
				return
			} else if idx, ok := regionIndex(id, "joinedroom"); ok && idx < len(hia.joinedLinks) {
				hia.OnOpenRoom(hia.joinedLinks[idx])
				return
			} else if idx, ok := regionIndex(id, "availroom"); ok && idx < len(hia.availLinks) {
				hia.OnOpenRoom(hia.availLinks[idx])
				return
			}
		}
		dispatchAddrRegion(hia.app, hia.view, "Hub Address", nil, added)
	})

	hia.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(hia.view, 0, 1, false)
	hia.widget.SetBorder(true)
	// Border titles are drawn with tview.Print (tag-parsed); the hub display
	// name is external text.
	hia.widget.SetTitle(fmt.Sprintf(" %v ", escapeTviewTags(hubName)))
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
	HubVersion  string
	MOTD        string
	AutoReconn  bool
	AutoList    bool
	AutoWho     bool
	JoinedRooms []string
	AvailRooms  []string
	// AvailRoomTopics carries each advertised room's topic (Python's
	// Available rooms list shows a "↳ topic" line under the room,
	// Channels.py:1806-1824). Nil when the provider does not supply topics.
	AvailRoomTopics map[string]string
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
	hia.widget.SetTitle(fmt.Sprintf(" %v ", escapeTviewTags(snap.Name)))
	hia.refreshView()
}

// dividerRow renders one full-width divider row: Python's
// urwid.Divider(g["divider1"]) (Channels.py:1783) repeats the divider glyph
// across the whole row, where the Go port previously wrote a single glyph.
// The width comes from the laid-out widget rect (falling back to a sane
// default before the first draw).
func (hia *HubInfoArea) dividerRow() string {
	_, _, w, _ := hia.widget.GetInnerRect()
	if w <= 0 {
		w = 60
	}
	return strings.Repeat(hia.app.Glyphs["divider1"], w)
}

// refreshView rebuilds the text display from current data, mirroring
// Python's _show_hub_info (Channels.py:1745-1816): the header block, the
// status line, the server line, the auto toggles, the divider, the status
// hint, the MOTD, and the joined/available rooms.
func (hia *HubInfoArea) refreshView() {
	snap := hia.snapshot
	if snap == nil {
		SetAddrRowText(hia.view, "[gray]No hub info available[-]")
		return
	}
	colors := GetThemeColors(hia.app.Theme)
	glyphs := hia.app.Glyphs
	check, cross := glyphs["check"], glyphs["cross"]

	var sb strings.Builder
	sb.WriteString("\n")
	// Hub name, advertised server name, status text and MOTD are external text
	// rendered into a dynamic-colors TextView; escape each so a bracket run
	// survives tview's tag parser.
	fmt.Fprintf(&sb, "  Hub      : %v\n", escapeTviewTags(snap.Name))
	fmt.Fprintf(&sb, "  Address  : %v\n", escapeTviewTags(snap.Address))
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
	fmt.Fprintf(&sb, "  [#%06x]Status   : %v (%v)[-]\n", uint32(statusColor), statusLabel, escapeTviewTags(snap.StatusText))
	if snap.ServerName != "" {
		// Python _show_hub_info (Channels.py:1764-1766): the advertised
		// server name carries " v<version>" when the welcome provides one.
		ver := ""
		if snap.HubVersion != "" {
			ver = " v" + snap.HubVersion
		}
		fmt.Fprintf(&sb, "  Server   : %v%v\n", escapeTviewTags(snap.ServerName), ver)
	}

	autoLine := func(label string, on bool, key string, verb string) {
		glyph, state := cross, "Off"
		attrColor := colors["list_unknown"]
		if on {
			glyph, state, attrColor = check, "On", colors["list_trusted"]
		}
		fmt.Fprintf(&sb, "  [#%06x]%v  : %v %v  (%v to %v)[-]\n", uint32(attrColor), label, glyph, state, key, verb)
	}
	autoLine("AutoRcn ", snap.AutoReconn, "Ctrl-T", "toggle")
	// Python lines 1776/1781: the AutoList/AutoWho hints read "Ctrl-E to
	// edit" (they open the edit-hub dialog), unlike AutoRcn's "to toggle".
	autoLine("AutoList", snap.AutoList, "Ctrl-E", "edit")
	autoLine("AutoWho ", snap.AutoWho, "Ctrl-E", "edit")
	sb.WriteString(hia.dividerRow() + "\n")

	switch snap.Status {
	case 2:
		sb.WriteString("  Connected. Use Ctrl-A to add a room.\n")
	case 1:
		sb.WriteString("[#afafaf]  Connecting...[-]\n")
	default:
		sb.WriteString("  Use Ctrl-R to connect.\n")
	}

	if snap.MOTD != "" {
		sb.WriteString(hia.dividerRow() + "\n  MOTD:\n")
		hia.motdLinks = hia.motdLinks[:0]
		for line := range strings.SplitSeq(snap.MOTD, "\n") {
			fmt.Fprintf(&sb, "  %v\n", hia.motdLine(line))
		}
	} else {
		hia.motdLinks = hia.motdLinks[:0]
	}

	if len(snap.JoinedRooms) > 0 {
		sb.WriteString(hia.dividerRow() + "\n  Joined rooms:\n")
		hia.joinedLinks = hia.joinedLinks[:0]
		for _, r := range snap.JoinedRooms {
			fmt.Fprintf(&sb, "    %v\n", hia.roomRegion("joinedroom", r))
		}
	} else {
		hia.joinedLinks = hia.joinedLinks[:0]
	}
	if len(snap.AvailRooms) > 0 {
		sb.WriteString(hia.dividerRow() + "\n  Available rooms:\n")
		hia.availLinks = hia.availLinks[:0]
		for _, r := range snap.AvailRooms {
			fmt.Fprintf(&sb, "    %v\n", hia.roomRegion("availroom", r))
			// Python renders the room's topic as a "↳ topic" line under it
			// (Channels.py:1806-1824).
			if topic := snap.AvailRoomTopics[r]; topic != "" {
				fmt.Fprintf(&sb, "      %v %v\n", glyph(glyphs, "arrow_r"), escapeTviewTags(topic))
			}
		}
	} else {
		hia.availLinks = hia.availLinks[:0]
	}
	// SetAddrRowText re-tags the hub address in the rebuilt block, so the click
	// wired at construction keeps working across refreshes.
	SetAddrRowText(hia.view, sb.String())
}

// regionIndex parses the index out of a region id such as "motdroom3".
func regionIndex(id, prefix string) (int, bool) {
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	idx, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil || idx < 0 {
		return 0, false
	}
	return idx, true
}

// motdLine renders one MOTD line with every #room reference wrapped in a
// clickable region, recording the targets in motdLinks. Python renders the
// MOTD through _linkify_motd with a _ChatLinkDelegate that opens room links
// (Channels.py:1229-1240,1795-1801).
func (hia *HubInfoArea) motdLine(line string) string {
	spans := motdRoomSpans(line)
	if len(spans) == 0 {
		return escapeTviewTags(line)
	}
	var b strings.Builder
	prev := 0
	for _, span := range spans {
		b.WriteString(escapeTviewTags(line[prev:span.Start]))
		fmt.Fprintf(&b, `["motdroom%v"]`, len(hia.motdLinks))
		hia.motdLinks = append(hia.motdLinks, span.Room)
		b.WriteString(escapeTviewTags(line[span.Start:span.End]))
		b.WriteString(`[""]`)
		prev = span.End
	}
	b.WriteString(escapeTviewTags(line[prev:]))
	return b.String()
}

// roomRegion renders "#room" as a clickable region and records the link target
// under prefix ("joinedroom" or "availroom").
func (hia *HubInfoArea) roomRegion(prefix, room string) string {
	var idx int
	switch prefix {
	case "joinedroom":
		idx = len(hia.joinedLinks)
		hia.joinedLinks = append(hia.joinedLinks, room)
	default:
		idx = len(hia.availLinks)
		hia.availLinks = append(hia.availLinks, room)
	}
	return fmt.Sprintf(`["%v%v"]#%v[""]`, prefix, idx, escapeTviewTags(room))
}
