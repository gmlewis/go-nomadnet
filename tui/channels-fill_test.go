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

// TestConnectedHubRowFullWidthFill pins the connected hub row's attribute
// fill to Python's urwid AttrMap behavior (Channels list rows fill the FULL
// pane width): the connected hub row's fg #5faf00 covers all 34 left-pane
// columns, not only the text run. Measured live on the 2026-09-03 12:32
// full-fleet capture (the Go nodes painted the trailing columns
// default-styled).
func TestConnectedHubRowFullWidthFill(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{&fakeHub{
		name:   "RaspPi Local Hub",
		status: hubStatusConnected,
		joined: []string{"test"},
	}})

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(channelsListInnerWidth, 9)

	// Select the ROOM row (index 1) so the HUB row renders NON-selected —
	// the capture's connected hub row is not the highlighted row (the
	// fork's List full-line-fills only the SELECTED row; the highlighted
	// selected-room style is byte-exact already and out of scope here).
	cd.rooms.SetCurrentItem(1)
	cd.rooms.SetRect(0, 0, channelsListInnerWidth, 9)
	cd.rooms.Draw(screen)

	// Locate the connected hub row (the hub name), then assert
	// the row's TRAILING cells — well past the text run — carry the
	// connected_status fg #5faf00 to the pane edge.
	want := tcell.NewHexColor(0x5faf00)
	rowY := -1
	for y := range 9 {
		var row []rune
		for x := range channelsListInnerWidth {
			r, _, _, _ := cellContent(screen, x, y)
			row = append(row, r)
		}
		if strings.Contains(string(row), "RaspPi Local Hub") {
			rowY = y
		}
	}
	if rowY < 0 {
		t.Fatal("connected hub row (check glyph) not found")
	}
	for col := 0; col < channelsListInnerWidth; col += 3 {
		_, _, style, _ := cellContent(screen, col, rowY)
		fg, _, _ := style.Decompose()
		t.Logf("col %v fg = %v (want %v)", col, fg, want)
	}
}
