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
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// IsJoinPartSystem reports whether m is a join/leave system message that is
// eligible for collapse, matching Python's _is_joinpart_system at
// Channels.py:1240: the message must be a system message with non-empty text,
// must not begin with "You " (those are the local user's own join/part notices
// which Python keeps visible), and must end with " joined" or " left".
func IsJoinPartSystem(m ChannelMessage) bool {
	if !m.IsSystem {
		return false
	}
	text := strings.TrimSpace(m.Text)
	if text == "" || strings.HasPrefix(text, "You ") {
		return false
	}
	return strings.HasSuffix(text, " joined") || strings.HasSuffix(text, " left")
}

// CollapseJoinPartMessages filters msgs, replacing each maximal run of
// consecutive join/leave system messages with a single synthetic ChannelMessage
// carrying the collapsed summary label (CollapsedJoinPartLabel). Non-join/part
// messages flush any pending run and are kept verbatim. Mirrors the run/flush
// logic in Python's RoomWidget.update_messages (Channels.py:758-776).
func CollapseJoinPartMessages(msgs []ChannelMessage) []ChannelMessage {
	out := make([]ChannelMessage, 0, len(msgs))
	run := 0
	flush := func() {
		if run > 0 {
			out = append(out, ChannelMessage{
				IsSystem: true,
				Text:     CollapsedJoinPartLabel(run),
			})
			run = 0
		}
	}
	for _, m := range msgs {
		if IsJoinPartSystem(m) {
			run++
			continue
		}
		flush()
		out = append(out, m)
	}
	flush()
	return out
}

// CollapsedJoinPartLabel builds the centered summary label for a run of n
// collapsed join/leave events, matching Python's _collapsed_join_part_widget
// (Channels.py:1250): two spaces, a midline horizontal ellipsis (U+22EF), the
// count with a singular/plural "join/leave event(s)" noun, and a trailing
// ellipsis. The label is the only meaningful content of the Python widget.
func CollapsedJoinPartLabel(n int) string {
	noun := " join/leave events"
	if n == 1 {
		noun = " join/leave event"
	}
	return "  ⋯  " + strconv.Itoa(n) + noun + "  ⋯"
}

// CollapsedJoinPartWidget returns a centered, system-styled tview text widget
// summarising n collapsed join/leave events, matching the urwid widget built by
// Python's _collapsed_join_part_widget (Channels.py:1250-1252) which wraps the
// label in a centered AttrMap tagged "irc_system".
func CollapsedJoinPartWidget(n int) *tview.TextView {
	tv := tview.NewTextView()
	tv.SetTextAlign(tview.AlignCenter)
	tv.SetText(CollapsedJoinPartLabel(n))
	tv.SetTextColor(tcell.ColorGray)
	return tv
}
