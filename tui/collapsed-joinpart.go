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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
