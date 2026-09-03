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
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// renderJustifiedRoom draws a RoomWidget with the given messages at the
// fleet's chat geometry (120 total cols → 22 users pane + 2 chat borders =
// the 96-wide chat inner measured on all 6 nodes) and returns the screen for
// per-cell assertions.
func renderJustifiedRoom(t *testing.T, msgs []ChannelMessage) tcell.Screen {
	t.Helper()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.RRCRender.JustifyMsgs = true
	app.RRCRender.NickColors = true
	rw := NewRoomWidget(app, "RaspPi Local Hub", "test")
	rw.SetMessages(msgs)

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
	return screen
}

// chatCell returns the cell at the given CHAT-INNER column (chat col 0 =
// screen col 1 here: the bare RoomWidget has no left pane, so the chat box
// border sits at screen col 0 and the 96-wide inner starts at col 1, the
// same inner width the 6-node fleet measures at 120 total cols).
func chatCell(s tcell.Screen, y, col int) (rune, tcell.Color, tcell.Color) {
	r, _, style, _ := cellContent(s, 1+col, y)
	fg, bg, _ := style.Decompose()
	return r, fg, bg
}

func defaultColor(v tcell.Color) bool {
	return v == tcell.ColorDefault
}

func mustChatCol(t *testing.T, s tcell.Screen, y, col int, wantRune rune, wantFg tcell.Color, label string) {
	t.Helper()
	r, fg, _ := chatCell(s, y, col)
	if r != wantRune {
		t.Errorf("%v: row %v col %v = %q, want %q", label, y, col, string(r), string(wantRune))
	}
	if fg != wantFg {
		t.Errorf("%v: row %v col %v fg = %v, want %v", label, y, col, fg, wantFg)
	}
}

// TestRoomWidgetJustifyLayout pins the Python justify two-column message
// layout (rrc_ui_justify_msgs=True default, Channels.py:1408-1413): each
// message row renders
//
//	urwid.Columns([(PACK, ts-prefix), (body)], dividechars=1) padded left=1
//
// so chat col 0 is a DEFAULT-styled pad space, cols 1-11 the ts
// "[HH:MM:SS] " run (11 chars, fg #888888), col 12 a DEFAULT-styled gap space
// (the column divider), and the body column starts at chat col 13 with
// "<nick>". Wrapped continuation lines indent to chat col 13 — the same
// column as the "<" — and their leading pad is DEFAULT-styled. Measured on
// the 2026-09-03 12:32 full-fleet capture (mac rows 22/27/28): pad len 1
// default, ts len 11 (136,136,136), gap len 1 default, "<d13aa6887074>"
// palette at col 13, body (221,221,221) from col 13/27; A2 continuation
// "original" at col 13.
func TestRoomWidgetJustifyLayout(t *testing.T) {
	t.Parallel()

	tsMs := time.Date(2026, 9, 3, 12, 27, 21, 0, time.Local).UnixMilli()
	hash := "0102030405060708090a0b0c0d0e0f10"
	nick := "Go port of NomadNet on Mac Mini M2"
	msgs := []ChannelMessage{
		{
			Room: "test", Nick: nick, SrcHash: hash,
			Text: "Message A2 from glenn-mac-mini-m2 ('nomadnet' original source-of-truth)",
			TsMs: tsMs,
		},
	}

	screen := renderJustifiedRoom(t, msgs)

	// The rendered message rows: row 0 is the room header, the message rows
	// follow below the indicator bar rows. Locate the message's first row by
	// scanning for "<" in the chat pane after the header.
	firstRow := -1
	for y := 1; y < 34; y++ {
		r, _, _ := chatCell(screen, y, 13)
		if r == '<' {
			firstRow = y
			break
		}
	}
	if firstRow < 0 {
		t.Fatal("no message row with \"<\" at chat col 13 found")
	}

	// Row 1: pad, ts, gap, nick, body — with the Python styles.
	mustChatCol(t, screen, firstRow, 0, ' ', tcell.ColorDefault, "pad")
	mustChatCol(t, screen, firstRow, 1, '[', tcellColorFrom256orHex(136, 136, 136), "ts open")
	mustChatCol(t, screen, firstRow, 10, ']', tcellColorFrom256orHex(136, 136, 136), "ts close")
	mustChatCol(t, screen, firstRow, 11, ' ', tcellColorFrom256orHex(136, 136, 136), "ts trailing space")
	mustChatCol(t, screen, firstRow, 12, ' ', tcell.ColorDefault, "gap")
	mustChatCol(t, screen, firstRow, 13, '<', NickColorByHashHexColor(hash, DefaultNickPalette(ThemeDark)), "nick open")
	mustChatCol(t, screen, firstRow, 49, ' ', tcellColorFrom256orHex(221, 221, 221), "body leading space")

	// Continuation row: 13 default-styled spaces, then the wrapped body at
	// the "<" column.
	row2 := firstRow + 1
	for col := range 13 {
		r, fg, _ := chatCell(screen, row2, col)
		if r != ' ' || !defaultColor(fg) {
			t.Errorf("continuation pad col %v = (%q, %v), want a default-styled space", col, string(r), fg)
		}
	}
	mustChatCol(t, screen, row2, 13, 'o', tcellColorFrom256orHex(221, 221, 221), "continuation body")

	// The body column's fill carries #dddddd to the chat box edge (the
	// urwid AttrMap fill — the 2026-09-03 12:32 capture's rows 22/27/28:
	// the body run + trailing spaces run to chat col 95).
	for _, y := range []int{firstRow, row2} {
		_, fg, _ := chatCell(screen, y, 95)
		if fg != tcellColorFrom256orHex(221, 221, 221) {
			t.Errorf("row %v col 95 fg = %v, want #dddddd (the body fill reaches the box edge)", y, fg)
		}
	}
}

// tcellColorFrom256orHex builds the tcell color for an RGB triple the way the
// capture decoder reports it (truecolor values).
func tcellColorFrom256orHex(r, g, b int32) tcell.Color {
	return tcell.NewHexColor(r<<16 | g<<8 | b)
}
