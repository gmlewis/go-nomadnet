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
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// The 2026-09-03 evening fleet captures: gonomadnet's Users pane painted the
// tview.List's always-on current-item highlight on the first member row
// directly under the "N users" count (ApplyListFocusStyle's list_focus
// background), which reads as a highlighted line under "N users". Python's
// Users pane is a PLAIN urwid.ListBox (Channels.py:626 —
// urwid.ListBox(self.users_walker)) with NO selection rendering at all: the
// member rows are AttrMap-colored plain Texts and urwid paints no focus
// attributes on them, so no row under the count is ever highlighted.

// TestRoomWidgetUsersPaneNoSelectionHighlight pins that: no member row under
// the "N users" count carries the list's selection background — the list
// paints nothing, exactly like Python's plain ListBox.
func TestRoomWidgetUsersPaneNoSelectionHighlight(t *testing.T) {
	t.Parallel()

	members := []ChannelMember{
		{Nick: "alice", Hash: "0102030405060708090a0b0c0d0e0f10", Online: true},
		{Nick: "bob", Hash: "02030405060708090a0b0c0d0e0f1001", Online: true},
		{Nick: "carol", Hash: "030405060708090a0b0c0d0e0f100102", Online: true},
	}
	screen, _ := renderUsersPane(t, members)

	countY := -1
	for y := 1; y < 36; y++ {
		if strings.Contains(usersRowText(screen, y), "3 users") {
			countY = y
			break
		}
	}
	if countY < 0 {
		t.Fatal("count row not found in the users pane")
	}

	for i, name := range []string{"alice", "bob", "carol"} {
		y := countY + 1 + i
		row := usersRowText(screen, y)
		if !strings.Contains(row, name) {
			t.Fatalf("member %q row %v = %q, want the member row", name, y, row)
		}
		_, _, bg := usersCell(screen, y, 1)
		if bg != tcell.ColorDefault {
			t.Errorf("member %q row %v bg = %v, want the pane's default (Python's plain ListBox paints no selection highlight under the count row)", name, y, bg)
		}
	}

	// The row under the last member is blank AND unstyled too.
	if countY+1+len(members) < 36 {
		_, _, bg := usersCell(screen, countY+1+len(members), 1)
		if bg != tcell.ColorDefault {
			t.Errorf("blank row %v bg = %v, want the pane's default", countY+1+len(members), bg)
		}
	}
}
