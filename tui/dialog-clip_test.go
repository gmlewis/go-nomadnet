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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// A dialog draws inside its own box and inside the slot it was placed in, and
// nowhere else.
//
// tview hands a primitive a rectangle and trusts it to stay in it. A Flex does
// not: its items are given the heights they ask for, so a column of fixed-height
// rows taller than the Flex draws its last rows below it, over whatever is there —
// the dialog's own bottom border, the pane under it, the shortcut bar at the bottom
// of the screen. urwid, which is the original here, gives every widget a size and
// clips its canvas to that size, so nothing it draws can reach outside it.
//
// The dialog is where this is easy to see, because its content is a fixed column of
// rows: the Peer Info dialog is 28 rows and the appliance's tablet has 21 rows in
// its client with the soft keyboard up, and the last of those rows is the client's
// own shortcut bar. What the reader saw was the dialog's explanation of an unknown
// peer printed over the shortcut bar, with the dialog's border drawn on top of the
// pane's — the border lines not lining up. These tests pin both halves: the box
// clips its content, and the overlay clips the box to the slot.

// TestDialogContentIsClippedToItsBox draws a dialog whose content is taller than
// the dialog, and requires that nothing outside the dialog's rectangle changed.
func TestDialogContentIsClippedToItsBox(t *testing.T) {
	t.Parallel()

	const (
		sw, sh = 40, 12
		boxW   = 20
		boxH   = 6
	)
	// A content column of more rows than the box can hold: the urwid PACK shape of
	// every nomadnet dialog that overflows.
	rows := tview.NewFlex().SetDirection(tview.FlexRow)
	for i := range 10 {
		rows.AddItem(NewUrwidLeftText("content row"), 1, 0, false)
		_ = i
	}
	dialog := NewDialogLineBox("Tall", rows, nil).SetBorderInside(true)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(sw, sh)

	// A marker everywhere else, so a cell the dialog touched is a changed cell.
	screen.Fill('~', tcell.StyleDefault)
	dialog.SetRect(2, 1, boxW, boxH)
	dialog.Draw(screen)
	screen.Sync()

	for y := range sh {
		for x := range sw {
			r, _, _, _ := cellContent(screen, x, y)
			inside := x >= 2 && x < 2+boxW && y >= 1 && y < 1+boxH
			if inside {
				continue
			}
			if r != '~' {
				got := dialogRow(screen, y, sw)
				t.Fatalf("the dialog drew outside its own box at (%v,%v): %q\nrow %v: %q",
					x, y, string(r), y, got)
			}
		}
	}

	// And the content that does not fit is gone, not moved: the box's last inner row
	// holds content and its bottom border is its own.
	if got := dialogRow(screen, 1+boxH-1, sw); !strings.Contains(got, "└") || !strings.Contains(got, "┘") {
		t.Errorf("the dialog's bottom border is missing: %q", got)
	}
}

// TestPeerInfoDialogStaysInsideItsSlot opens the Peer Info dialog on the
// appliance's own geometry — a 163x21 client, the grid a Galaxy Tab A9+ has with
// the soft keyboard up — through the real main display chrome, and requires that
// the only cells it changes are the ones inside the conversations list column. The
// dialog is 28 rows tall and the column is 19, so it is clipped at the column's
// bottom: the shortcut bar and the conversation pane beside it are not the dialog's
// to draw on.
func TestPeerInfoDialogStaysInsideItsSlot(t *testing.T) {
	t.Parallel()

	const (
		sw, sh = 163, 21
		// The conversations list column: urwid GIVEN 52, full content height.
		slotX, slotY, slotW, slotH = 0, 1, 52, 19
	)
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	app.Main.SetDisplay("conversations", cd.Widget())

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(sw, sh)

	root := app.Main.Root()
	root.SetRect(0, 0, sw, sh)
	root.Draw(screen)
	screen.Sync()
	before := snapshot(screen, sw, sh)

	cd.ShowPeerInfoDialog(PeerInfoEntry{
		SourceHash: "2a6105f57145860441a62fe3b2a1352c",
		TrustLevel: TrustUnknown,
	}, PeerInfoDialogHooks{IsKnown: func(string) bool { return false }}, func(PeerInfoEntry) {})

	root.SetRect(0, 0, sw, sh)
	root.Draw(screen)
	screen.Sync()
	after := snapshot(screen, sw, sh)

	for y := range sh {
		for x := range sw {
			if before[y][x] == after[y][x] {
				continue
			}
			if x >= slotX && x < slotX+slotW && y >= slotY && y < slotY+slotH {
				continue
			}
			t.Fatalf("the Peer Info dialog drew at (%v,%v), outside the conversations slot %vx%v at (%v,%v):\n"+
				"was: %q\nnow: %q",
				x, y, slotW, slotH, slotX, slotY, string(before[y][x]), string(after[y][x]))
		}
	}

	// The dialog is taller than the slot, so its bottom border lies outside it: what
	// is left at the slot's bottom row is the pane's own border, whole.
	last := dialogRow(screen, slotY+slotH-1, slotW)
	if !strings.HasPrefix(last, "└") || !strings.HasSuffix(strings.TrimRight(last, " "), "┘") {
		t.Errorf("the list pane's bottom border is not intact under the dialog: %q", last)
	}
}

// dialogRow is one screen row's text, with blanks for the cells no widget wrote.
func dialogRow(screen tcell.Screen, y, w int) string {
	var b strings.Builder
	for x := range w {
		r, _, _, _ := cellContent(screen, x, y)
		if r == 0 {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}

// snapshot is the screen's text, one string per row.
func snapshot(screen tcell.Screen, sw, sh int) [][]rune {
	out := make([][]rune, sh)
	for y := range sh {
		out[y] = []rune(dialogRow(screen, y, sw))
	}
	return out
}
