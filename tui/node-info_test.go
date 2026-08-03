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

// renderNodeInfo draws the NodeInfoDisplay widget on a width×height simulation
// screen and returns the joined cell text per row, mirroring the headless
// parity capture so the layout can be pinned against Python's NodeInfo
// (Network.py:1357-1554).
func renderNodeInfo(t *testing.T, ni *NodeInfoDisplay, width, height int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	ni.Widget().SetRect(0, 0, width, height)
	ni.Widget().Draw(screen)
	screen.Sync()

	rows := make([]string, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		for x := 0; x < width; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// TestNodeInfoNotHostingLayout pins the "Local Node Info" panel's not-hosting
// branch (the only reachable state until node hosting is wired in Phase 5)
// against the Python ground truth (Network.py:1543-1551): a bordered "Local
// Node Info" LineBox wrapping a Pile of a centered info glyph, the centered
// "This instance is not hosting a node" message, and a centered "< Back >"
// button. The inner width is 48 (50-wide panel minus 2 border columns).
//
// Centering parity: urwid.Text(align=CENTER) is ceil-left (extra col to the
// LEFT on odd slack) → the glyph and message use centeredText. urwid
// .Padding(CENTER, PACK) is floor-left (extra col to the RIGHT) → the Back
// button uses a two-equal-spacer Flex row. With a 48-wide inner:
//
//	glyph (width 1):   leftPad = (48-1+1)/2 = 24  → col 25  (ceil-left)
//	message (width 35): leftPad = (48-35+1)/2 = 7  → col 8   (ceil-left)
//	Back button (width 8): left = (48-8)/2 = 20    → col 21  (floor-left)
func TestNodeInfoNotHostingLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	ni := NewNodeInfoDisplay(app, NodeInfoData{HasNode: false})

	if got := ni.Height(); got != 9 {
		t.Fatalf("Height = %v, want 9 (7 content + 2 border)", got)
	}

	rows := renderNodeInfo(t, ni, 50, ni.Height())

	// Row 0: top border with centered title "Local Node Info".
	if !strings.Contains(rows[0], "Local Node Info") {
		t.Errorf("row 0 = %q, want it to contain the title %q", rows[0], "Local Node Info")
	}
	if r := []rune(rows[0]); r[0] != '┌' || r[len(r)-1] != '┐' {
		t.Errorf("row 0 corners = %q…%q, want ┌…┐", string(r[0]), string(r[len(r)-1]))
	}

	// Row 1: first line of urwid.Text("\n"+g["info"]) → blank (borders + spaces).
	blankRow := "│" + strings.Repeat(" ", 48) + "│"
	if got := rows[1]; got != blankRow {
		t.Errorf("row 1 = %q, want a blank bordered line", got)
	}

	// Row 2: the info glyph, ceil-left-centered at col 25.
	infoGlyph := app.Glyphs["info"] // "ℹ" in the unicode set
	if c := getCell(rows, 25, 2); c != infoGlyph {
		t.Errorf("row 2 col 25 = %q, want the info glyph %q (ceil-left centered)", c, infoGlyph)
	}
	if r := []rune(rows[2]); r[0] != '│' || r[len(r)-1] != '│' {
		t.Errorf("row 2 corners = %q…%q, want │…│", string(r[0]), string(r[len(r)-1]))
	}

	// Row 3: first line of the message text → blank.
	if got := rows[3]; got != blankRow {
		t.Errorf("row 3 = %q, want a blank bordered line", got)
	}

	// Row 4: "This instance is not hosting a node" (35 wide), ceil-left at col 8.
	msg := "This instance is not hosting a node"
	if got := rows[4]; !strings.Contains(got, msg) {
		t.Errorf("row 4 = %q, want it to contain %q", got, msg)
	}
	if c := getCell(rows, 8, 4); c != "T" {
		t.Errorf("row 4 col 8 = %q, want 'T' (message starts at col 8, ceil-left)", c)
	}

	// Rows 5-6: the trailing "\n\n" of the message text → blank.
	if got := rows[5]; got != blankRow {
		t.Errorf("row 5 = %q, want a blank bordered line", got)
	}
	if got := rows[6]; got != blankRow {
		t.Errorf("row 6 = %q, want a blank bordered line", got)
	}

	// Row 7: the centered "< Back >" button, floor-left at col 21.
	if c := getCell(rows, 21, 7); c != "<" {
		t.Errorf("row 7 col 21 = %q, want '<' (Back button starts at col 21, floor-left)", c)
	}
	if got := rows[7]; !strings.Contains(got, "Back") {
		t.Errorf("row 7 = %q, want it to contain the Back button label", got)
	}

	// Row 8: bottom border.
	if r := []rune(rows[8]); r[0] != '└' || r[len(r)-1] != '┘' {
		t.Errorf("row 8 corners = %q…%q, want └…┘", string(r[0]), string(r[len(r)-1]))
	}
}

// TestNodeInfoBackButton verifies the Back button fires OnBack when activated.
func TestNodeInfoBackButton(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	ni := NewNodeInfoDisplay(app, NodeInfoData{HasNode: false})
	fired := false
	ni.OnBack = func() { fired = true }

	// Simulate Enter via the button's input handler.
	handler := ni.backBtn.InputHandler()
	if handler == nil {
		t.Fatal("backBtn has no input handler")
	}
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if !fired {
		t.Errorf("Back button activation did not fire OnBack")
	}
}

// TestNodeInfoHeightNotHosting pins the fixed PACK height of the not-hosting
// branch: 7 content rows + 2 border rows = 9.
func TestNodeInfoHeightNotHosting(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	ni := NewNodeInfoDisplay(app, NodeInfoData{HasNode: false})
	if got := ni.Height(); got != 9 {
		t.Errorf("not-hosting Height = %v, want 9", got)
	}
}

// getCell returns the rune at (x, y) from a slice of rendered rows, or ' ' if
// out of range.
func getCell(rows []string, x, y int) string {
	if y < 0 || y >= len(rows) {
		return " "
	}
	r := []rune(rows[y])
	if x < 0 || x >= len(r) {
		return " "
	}
	return string(r[x])
}
