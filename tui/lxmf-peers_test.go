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

// renderPrimitive draws p at width×height on a simulation screen and returns
// the joined-cell rows (each cell emitted as its rune).
func renderPrimitive(t *testing.T, p tview.Primitive, width, height int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	p.SetRect(0, 0, width, height)
	p.Draw(screen)
	screen.Sync()

	rows := make([]string, height)
	for y := range height {
		var b strings.Builder
		for x := range width {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

func cellAt(rows []string, x, y int) string {
	if y < 0 || y >= len(rows) {
		return " "
	}
	r := []rune(rows[y])
	if x < 0 || x >= len(r) {
		return " "
	}
	return string(r[x])
}

// TestLXMFPeersNoContentLayout pins the empty LXMF peers list against Python's
// LXMFPeers no-content branch (Network.py:1779-1788): a top-filled, centered
// warning-text block — info glyph on the first row, a blank, then "Currently,
// no LXMF nodes are peered" — inside a titled LineBox "LXMF Propagation Peers
// (0)". The widget supplies only the inner content (the border+title come from
// the network left-pane slot).
func TestLXMFPeersNoContentLayout(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	pp := NewLXMFPeersDisplay(app)

	if got := pp.Count(); got != 0 {
		t.Errorf("Count = %v, want 0", got)
	}
	if got := pp.Title(); got != "LXMF Propagation Peers (0)" {
		t.Errorf("Title = %q, want %q", got, "LXMF Propagation Peers (0)")
	}

	// info glyph is "ℹ" for the Unicode glyph set.
	infoGlyph := app.Glyphs["info"]
	rows := renderPrimitive(t, pp.Widget(), 50, 10)

	// Row 0: the info glyph, ceil-left-centered in 50 cols.
	// runewidth("ℹ")=1 → leftPad = (50-1+1)/2 = 25.
	if c := cellAt(rows, 25, 0); c != infoGlyph {
		t.Errorf("info glyph at (25,0) = %q, want %q (row0=%q)", c, infoGlyph, rows[0])
	}
	// Row 1: blank.
	if strings.TrimSpace(rows[1]) != "" {
		t.Errorf("row 1 should be blank, got %q", rows[1])
	}
	// Row 2: "Currently, no LXMF nodes are peered" (35 cols) ceil-left-centered.
	// leftPad = (50-35+1)/2 = 8 → 'C' at col 8.
	wantMsg := "Currently, no LXMF nodes are peered"
	if c := cellAt(rows, 8, 2); c != "C" {
		t.Errorf("message start at (8,2) = %q, want 'C' (row2=%q)", c, rows[2])
	}
	if !strings.Contains(rows[2], wantMsg) {
		t.Errorf("row 2 = %q, want it to contain %q", rows[2], wantMsg)
	}
	// Rows 3-4: blank (the two trailing newlines in Python's SelectText).
	if strings.TrimSpace(rows[3]) != "" {
		t.Errorf("row 3 should be blank, got %q", rows[3])
	}
}

// TestLXMFPeersSetPeersEmpty keeps the no-content branch when SetPeers is given
// an empty slice (real peers are passed once the LXMF router is wired in).
func TestLXMFPeersSetPeersEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	pp := NewLXMFPeersDisplay(app)
	pp.SetPeers(nil)

	if pp.Count() != 0 {
		t.Errorf("Count after SetPeers(nil) = %v, want 0", pp.Count())
	}
	if pp.Title() != "LXMF Propagation Peers (0)" {
		t.Errorf("Title after SetPeers(nil) = %q, want count 0", pp.Title())
	}
}
