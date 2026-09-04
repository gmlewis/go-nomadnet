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
	"github.com/rivo/tview"
)

// renderUsersPane draws a RoomWidget with the given members at the fleet's
// geometry (120 total cols: chat 98 + users pane 22) and returns the screen.
func renderUsersPane(t *testing.T, members []ChannelMember) (tcell.Screen, *RoomWidget) {
	t.Helper()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.RRCRender.JustifyMsgs = true
	app.RRCRender.NickColors = true
	rw := NewRoomWidget(app, "RaspPi Local Hub", "test")
	rw.SetMembers(members)
	// The users pane is FOCUSED in this harness (the selection/highlight
	// tests describe keyboard navigation on the focused pane): Python only
	// paints the focused row's list_focus highlight while the users list box
	// has focus (AttrMap(entry, style, "list_focus"), Channels.py:714).
	app.SetFocus(rw.usersList)

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(120, 37)

	widget := rw.Widget().(*tview.Flex)
	widget.SetRect(0, 0, 120, 37)
	widget.Draw(screen)
	return screen, rw
}

// usersCell returns the rune and colors at the given column inside the users
// pane (pane col 0 = screen col 99: the pane's border sits at screen col 98
// and its 20-wide inner starts at 99).
func usersCell(s tcell.Screen, y, col int) (rune, tcell.Color, tcell.Color) {
	r, _, style, _ := cellContent(s, 99+col, y)
	fg, bg, _ := style.Decompose()
	return r, fg, bg
}

// usersRowText returns the users pane's rendered text on one row.
func usersRowText(s tcell.Screen, y int) string {
	var row []rune
	for col := range 20 {
		r, _, _, _ := cellContent(s, 99+col, y)
		if r == 0 {
			r = ' '
		}
		row = append(row, r)
	}
	return string(row)
}

// TestRoomWidgetUsersPaneRows pins the users-pane row model to Python
// _refresh_users_pane (Channels.py:679-716): the " N users" count row is a
// DEFAULT-styled plain row (urwid.Text — no selection highlight), followed
// by the member rows on CONSECUTIVE rows (no phantom blank between them).
// Measured live on the 2026-09-03 12:32 full-fleet capture: the Go nodes
// painted a phantom blank row after EVERY member row (the fork's
// tview.List default ShowSecondaryText(true) renders each member's empty
// secondary text) and the count row carried the selection highlight
// (fg 0,0,0 / bg 175,175,175).
func TestRoomWidgetUsersPaneRows(t *testing.T) {
	t.Parallel()

	members := []ChannelMember{
		{Nick: "alice", Hash: "0102030405060708090a0b0c0d0e0f10", Online: true},
		{Nick: "bob", Hash: "02030405060708090a0b0c0d0e0f1001", Online: true},
		{Nick: "carol", Hash: "030405060708090a0b0c0d0e0f100102", Online: true},
	}
	screen, _ := renderUsersPane(t, members)

	findRow := func(needle string) int {
		for y := 1; y < 36; y++ {
			if strings.Contains(usersRowText(screen, y), needle) {
				return y
			}
		}
		return -1
	}

	countY, fg, bg := -1, tcell.ColorDefault, tcell.ColorDefault
	for y := 1; y < 36; y++ {
		if strings.Contains(usersRowText(screen, y), "3 users") {
			countY = y
			_, fg, bg = usersCell(screen, y, 1)
			break
		}
	}
	if countY < 0 {
		t.Fatal("count row not found in the users pane")
	}
	if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
		t.Errorf("count row style = fg %v / bg %v, want the default-styled plain text of Python's count row (Channels.py:694)", fg, bg)
	}

	// The member rows follow the count row on consecutive rows — any blank
	// row between them is the phantom secondary-text row.
	for i, name := range []string{"alice", "bob", "carol"} {
		y := findRow(name)
		if y < 0 {
			t.Fatalf("member %q row not found", name)
		}
		if y != countY+1+i {
			t.Errorf("member %q row = %v, want %v (Python renders members on consecutive rows — a phantom blank row shifts this)", name, y, countY+1+i)
		}
	}
}

// TestRoomWidgetUsersPaneCountRowText pins the count row's text (" N users"
// with the plural only for N != 1, Channels.py:694) and that it is NOT a
// List item anymore (moving it out of the List keeps it from ever taking
// the selection style).
func TestRoomWidgetUsersPaneCountRowText(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	rw := NewRoomWidget(app, "hub1", "general")

	rw.SetMembers([]ChannelMember{
		{Nick: "alice", Hash: "0102030405060708090a0b0c0d0e0f10"},
		{Nick: "bob", Hash: "02030405060708090a0b0c0d0e0f1001"},
	})
	if got := rw.usersCount.GetText(true); got != " 2 users" {
		t.Errorf("count row = %q, want %q", got, " 2 users")
	}

	rw.SetMembers(nil)
	if got := rw.usersCount.GetText(true); got != " 0 users" {
		t.Errorf("empty count row = %q, want \" 0 users\"", got)
	}
}

// TestRoomWidgetUsersRowFill pins the users rows' attribute fill to Python's
// urwid AttrMap rows (Channels.py:705: the member entry is wrapped in
// AttrMap(entry, style) whose fill paints the member's fg across the FULL
// pane width): the row's trailing spaces carry the member's palette fg to
// the pane edge. Measured live on the 2026-09-03 12:32 full-fleet capture.
// The SELECTED (focused) row instead carries the list_focus style across the
// whole width (the focus map replaces the row's own style) — that state is
// pinned by TestRoomWidgetUsersPaneSelectionHighlight.
func TestRoomWidgetUsersRowFill(t *testing.T) {
	t.Parallel()

	members := []ChannelMember{
		{Nick: "alice", Hash: "0102030405060708090a0b0c0d0e0f10", Online: true},
		{Nick: "bob", Hash: "02030405060708090a0b0c0d0e0f1001", Online: true},
	}
	screen, _ := renderUsersPane(t, members)

	rowY := -1
	for y := 1; y < 36; y++ {
		if strings.Contains(usersRowText(screen, y), "bob") {
			rowY = y
			break
		}
	}
	if rowY < 0 {
		t.Fatal("bob row not found")
	}

	want := NickColorByHashHexColor("02030405060708090a0b0c0d0e0f1001", DefaultNickPalette(ThemeDark))
	// The pane is 20 cols wide; the label " → bob" ends well before the
	// edge — the trailing cells carry the member's fg like Python's fill.
	for _, col := range []int{15, 18, 19} {
		_, fg, _ := usersCell(screen, rowY, col)
		if fg != want {
			t.Errorf("bob row col %v fg = %v, want %v (the AttrMap fill covers the full pane width)", col, fg, want)
		}
	}
}
