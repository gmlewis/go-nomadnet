// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"fmt"
	"sort"

	"github.com/gdamore/tcell/v2"
)

// Hub status constants, mirroring the RRC protocol enum (rrc.StatusDisconnected
// = 0 … rrc.StatusFailed = 3). Defined here so the tui package can render hub
// status without importing the rrc package; the app layer adapts *rrc.RRCHub to
// HubView and these values are the stable wire enum, not an implementation
// detail.
const (
	hubStatusDisconnected = 0
	hubStatusConnecting   = 1
	hubStatusConnected    = 2
	hubStatusFailed       = 3
)

// HubView is the read-only view of an RRC hub the channels list renders. The
// app layer adapts *rrc.RRCHub to this interface so the tui package does not
// import rrc (mirrors the SendDeps injection pattern). Room slices carry the
// hub's joined rooms, rooms that have received messages, and the unread /
// mention room sets.
type HubView interface {
	Name() string
	Status() int
	JoinedRooms() []string
	MessageRooms() []string
	UnreadRooms() []string
	MentionRooms() []string
}

// HubListEntryKind identifies a channels-list row as a hub header, a room under
// a hub, or the blank spacer that separates consecutive hubs.
type HubListEntryKind int

const (
	RowHub HubListEntryKind = iota
	RowRoom
	RowSpacer
)

// HubListEntry is one row of the channels hub/room list, mirroring one element
// of Python's _compose_list_widgets list_widgets (Channels.py:1599-1662).
// Label is the rendered text; Style is the tview style key for the row color;
// HubIdx locates the hub (for hub and room rows); Room names the room (room
// rows only). Spacer rows carry no label.
type HubListEntry struct {
	Kind   HubListEntryKind
	Label  string
	Style  string
	HubIdx int
	Room   string
}

// ComposeHubList builds the channels hub/room list rows from the given hubs,
// mirroring Python Channels._compose_list_widgets (Channels.py:1599-1662):
// for each hub a status-glyph + name header, followed by the sorted union of
// its joined rooms and message-bearing rooms (empty names skipped), each as a
// "   <marker> #<room>" row styled by room state — mentioned → irc_mention,
// unread → list_unresponsive, not-joined → list_unknown, joined → list_trusted
// only when the hub is connected (else list_unknown). A blank spacer row
// separates consecutive hubs. No hubs yields no rows (the no-hubs empty state
// is rendered by the list's empty widget, not here).
func ComposeHubList(hubs []HubView, glyphs GlyphSet) []HubListEntry {
	if len(hubs) == 0 {
		return nil
	}
	check := glyphs["check"]
	info := glyphs["info"]
	cross := glyphs["cross"]
	warning := glyphs["warning"]
	unreadGlyph := glyphs["unread"]

	var out []HubListEntry
	for i, hub := range hubs {
		if i > 0 {
			out = append(out, HubListEntry{Kind: RowSpacer, HubIdx: i})
		}

		var statusGlyph, hubStyle string
		switch hub.Status() {
		case hubStatusConnected:
			statusGlyph = check
			hubStyle = "list_trusted"
		case hubStatusConnecting:
			statusGlyph = info
			hubStyle = "list_unresponsive"
		case hubStatusFailed:
			statusGlyph = cross
			hubStyle = "list_untrusted"
		default:
			statusGlyph = " "
			hubStyle = "list_unknown"
		}
		out = append(out, HubListEntry{
			Kind:   RowHub,
			Label:  statusGlyph + " " + hub.Name(),
			Style:  hubStyle,
			HubIdx: i,
		})

		// Room set = sorted union of joined rooms and message-bearing rooms,
		// skipping empty names (Python: sorted(hub.rooms | set(hub.messages)).
		roomSet := map[string]bool{}
		for _, r := range hub.JoinedRooms() {
			if r != "" {
				roomSet[r] = true
			}
		}
		for _, r := range hub.MessageRooms() {
			if r != "" {
				roomSet[r] = true
			}
		}
		rooms := make([]string, 0, len(roomSet))
		for r := range roomSet {
			rooms = append(rooms, r)
		}
		sort.Strings(rooms)

		unread := map[string]bool{}
		for _, r := range hub.UnreadRooms() {
			unread[r] = true
		}
		mentioned := map[string]bool{}
		for _, r := range hub.MentionRooms() {
			mentioned[r] = true
		}
		joined := map[string]bool{}
		for _, r := range hub.JoinedRooms() {
			joined[r] = true
		}

		for _, room := range rooms {
			marker := " "
			roomStyle := "list_unknown"
			switch {
			case mentioned[room]:
				marker = warning
				roomStyle = "irc_mention"
			case unread[room]:
				marker = unreadGlyph
				roomStyle = "list_unresponsive"
			case !joined[room]:
				marker = " "
				roomStyle = "list_unknown"
			default:
				marker = " "
				if hub.Status() == hubStatusConnected {
					roomStyle = "list_trusted"
				} else {
					roomStyle = "list_unknown"
				}
			}
			out = append(out, HubListEntry{
				Kind:   RowRoom,
				Label:  "   " + marker + " #" + room,
				Style:  roomStyle,
				HubIdx: i,
				Room:   room,
			})
		}
	}
	return out
}

// HubListRowText renders a HubListEntry as a tview color-tagged string for the
// channels list, applying the entry's style color from the theme palette.
// Spacer rows render as an empty string. The unfocused foreground color is
// the style's palette color; the focused background is the shared list_focus
// grey (set globally on the list via ApplyListFocusStyle), matching Python's
// AttrMap(entry, style, "list_focus") where every focus variant uses the same
// 0xaaaaaa background. An unknown style key falls back to the uncolored label.
func HubListRowText(entry HubListEntry, colors map[string]tcell.Color) string {
	if entry.Kind == RowSpacer {
		return ""
	}
	c, ok := colors[entry.Style]
	if !ok {
		return entry.Label
	}
	return fmt.Sprintf("[#%06x]%s[-]", uint32(c), entry.Label)
}
