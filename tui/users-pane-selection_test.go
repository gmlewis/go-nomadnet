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

// The users pane is an INTERACTIVE list in the Go port: exactly the selected
// row carries the theme's list_focus highlight WHILE THE PANE HOLDS FOCUS
// (Python's pane is a plain urwid.ListBox whose member rows are
// AttrMap(entry, style, "list_focus") — the focus map applies on focus and
// the row's own palette style otherwise, Channels.py:714). An unfocused pane
// renders the selected member like any other row (Bug#2), and the focused
// highlight replaces the member's palette fg with the list_focus foreground
// so the row stays readable (Bug#3).

// TestRoomWidgetUsersPaneSelectionHighlight pins that exactly ONE member row
// (the list's selection, the first member by default) carries the list_focus
// background while every other row and the count row stay default-styled.
func TestRoomWidgetUsersPaneSelectionHighlight(t *testing.T) {
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

	wantFg, wantBg := ListFocusColors(ThemeDark)

	// The selected row (the first member) carries the highlight...
	selectedY := countY + 1
	if row := usersRowText(screen, selectedY); !strings.Contains(row, "alice") {
		t.Fatalf("selected row %v = %q, want the alice row", selectedY, row)
	}
	_, _, bg := usersCell(screen, selectedY, 1)
	if bg != wantBg {
		t.Errorf("selected row bg = %v, want the list_focus highlight %v (fg %v)", bg, wantBg, wantFg)
	}

	// ...the non-selected rows do not...
	for i, name := range []string{"bob", "carol"} {
		y := countY + 2 + i
		row := usersRowText(screen, y)
		if !strings.Contains(row, name) {
			t.Fatalf("member %q row %v = %q, want the member row", name, y, row)
		}
		_, _, bg := usersCell(screen, y, 1)
		if bg == wantBg {
			t.Errorf("non-selected member %q row %v bg = %v, want the pane's default (only the selected row highlights)", name, y, bg)
		}
	}

	// ...and neither does the "N users" count row above the list.
	_, _, bg = usersCell(screen, countY, 1)
	if bg != tcell.ColorDefault {
		t.Errorf("count row bg = %v, want the default-styled plain text row", bg)
	}

	// Bug#3: the highlighted row's foreground is the list_focus foreground
	// (the AttrMap focus map REPLACES the member's palette color, urwid
	// Channels.py:714), so the row reads as black-on-light-gray instead of
	// keeping the palette fg over the highlight background.
	_, fg, _ := usersCell(screen, selectedY, 1)
	if fg != wantFg {
		t.Errorf("selected row fg = %v, want the list_focus foreground %v", fg, wantFg)
	}
}

// TestRoomWidgetUsersPaneUnfocusedNoHighlight pins Bug#2: with the cursor
// elsewhere (the users pane NOT focused), the selected member row renders
// like any other row — the member's palette foreground on the default
// background, no highlight — mirroring Python's AttrMap(entry, style,
// "list_focus") whose focus map only applies on focus (Channels.py:714).
func TestRoomWidgetUsersPaneUnfocusedNoHighlight(t *testing.T) {
	t.Parallel()

	members := []ChannelMember{
		{Nick: "alice", Hash: "0102030405060708090a0b0c0d0e0f10", Online: true},
		{Nick: "bob", Hash: "02030405060708090a0b0c0d0e0f1001", Online: true},
	}
	screen, rw := renderUsersPane(t, members)
	// Blur the pane: focus moves into the room's message body, the way the
	// Left/Right walk leaves the users pane.
	rw.app.SetFocus(rw.messagesArea)
	redrawRoomWidget(t, rw, screen)

	_, wantBg := ListFocusColors(ThemeDark)

	// No member row carries the highlight background...
	for _, m := range members {
		y := -1
		for yy := 1; yy < 36; yy++ {
			if strings.Contains(usersRowText(screen, yy), m.Nick) {
				y = yy
				break
			}
		}
		if y < 0 {
			t.Fatalf("member %q row not found", m.Nick)
		}
		_, _, bg := usersCell(screen, y, 1)
		if bg == wantBg {
			t.Errorf("unfocused pane member %q row bg = %v, want no highlight (Python renders the row with its own style)", m.Nick, bg)
		}
		// ...and every row keeps its OWN member's palette foreground.
		_, fg, _ := usersCell(screen, y, 1)
		if fg != NickColorByHashHexColor(m.Hash, DefaultNickPalette(ThemeDark)) {
			t.Errorf("unfocused pane member %q row fg = %v, want the member's palette color", m.Nick, fg)
		}
	}
}
