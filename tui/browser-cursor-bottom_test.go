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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// wrapModelPage is a three-line page whose middle line wraps into two
// pre-wrapped rows in the emitted tview text:
//
//	aaaa bbbb cccc dddd   (line 0, one row)
//	eeee ffff gggg        (line 0's wrapped second row)
//	last `Link`:/page/x.mu]  (line 1, one row)
//	final                 (line 2, one row)
//
// The nav row model must count line 0 as TWO display rows, so the hardware
// cursor on line 2 lands on the LAST rendered row.
const wrapModelPage = "aaaa bbbb cccc dddd eeee ffff gggg\nlast `[Link`:/page/x.mu]\nfinal"

// TestBrowserRowModelCountsWrappedLines pins rowsAbove/lineRowCount against the
// rendered layout when a line ABOVE the cursor pre-wraps into multiple rows.
// The regression this pins: lineTexts held one entry per emitted ROW (not per
// line), so every currentLines index past the wrap shifted rowsAbove one row
// short and the hardware cursor drew one row above the focused line (the TTP
// node bottom-of-page off-by-one).
func TestBrowserRowModelCountsWrappedLines(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	const W = 20
	bd.content.SetRect(0, 0, W, 10)
	bd.currentMarkup = wrapModelPage
	bd.renderPage()

	if len(bd.currentLines) != len(bd.lineTexts) {
		t.Fatalf("lineTexts entries=%d, want one per rendered line (%d): %v",
			len(bd.lineTexts), len(bd.currentLines), bd.lineTexts)
	}
	wrapped := findLine(bd, "bbbb")
	if wrapped < 0 {
		t.Fatalf("no wrapped line")
	}
	link := findLine(bd, "Link")
	final := findLine(bd, "final")
	if link < 0 || final < 0 {
		t.Fatalf("missing lines: link=%d final=%d", link, final)
	}

	// "aaaa bbbb cccc dddd eeee ffff gggg" (33 runes) wraps at width 20 into
	// "aaaa bbbb cccc dddd" (19) + "eeee ffff gggg" (14) = 2 rows.
	if got := bd.lineRowCount(wrapped); got != 2 {
		t.Errorf("lineRowCount(wrapped)=%d, want 2", got)
	}
	if got := bd.rowsAbove(link); got != 2 {
		t.Errorf("rowsAbove(link idx %d)=%d, want 2 (the wrapped line's two rows)",
			link, got)
	}
	if got := bd.rowsAbove(final); got != 3 {
		t.Errorf("rowsAbove(final idx %d)=%d, want 3", final, got)
	}
	if got := bd.totalWrappedRows(); got != 4 {
		t.Errorf("totalWrappedRows=%d, want 4 (2+1+1)", got)
	}
}

// cursorRecorderScreen records ShowCursor calls so the test can read back the
// hardware-cursor position the Draw pass chose.
type cursorRecorderScreen struct {
	tcell.Screen
	shownX, shownY int
	shown          bool
}

func (c *cursorRecorderScreen) ShowCursor(x, y int) {
	c.shownX, c.shownY, c.shown = x, y, true
	c.Screen.ShowCursor(x, y)
}

func (c *cursorRecorderScreen) HideCursor() {
	c.shown = false
	c.Screen.HideCursor()
}

// TestBrowserCursorBottomOnFinalLine reproduces the user's bug: on the TTP
// node's index page (a wrapping paragraph mid-page, centered link menu on the
// final line) at the real 103×29 browser content size, pressing Down to the
// last selectable line and then past it (extra line-scroll Downs) must draw
// the hardware cursor ON the final line's rendered row — not one row above.
func TestBrowserCursorBottomOnFinalLine(t *testing.T) {
	t.Parallel()
	fixture := filepath.Join("..", "nomadnet", "micron", "testdata", "parity", "ttp-index.mu")
	markup, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("ttp fixture not readable: %v", err)
	}

	const W, H = 103, 29
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, W, H)
	bd.currentMarkup = string(markup)
	bd.renderPage()

	home := findLine(bd, "Directory")
	if home < 0 {
		t.Fatalf("no Home menu line")
	}
	if bd.currentLines[home].Align != micron.AlignCenter {
		t.Fatalf("fixture changed: final menu line is no longer center-aligned: %v", bd.currentLines[home].Align)
	}
	bd.focusLine = home
	bd.lineCursors[home] = 0
	bd.ensureVisible()
	// The user pressed Down several more times after reaching the last line:
	// each press line-scrolls (Scrollable SCROLL_LINE_DOWN) and clamps at the
	// document bottom.
	for range 3 {
		bd.handleInput(key(tcell.KeyDown, 0))
	}

	screen := &cursorRecorderScreen{Screen: tcell.NewSimulationScreen("")}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	screen.SetSize(W, H)
	bd.cursorHasKeypress = true
	bd.cursorLastKeypress = time.Now()
	bd.content.Focus(func(tview.Primitive) {})
	bd.content.Draw(screen)

	if !screen.shown {
		t.Fatalf("hardware cursor not shown after Down navigation")
	}
	// Locate the rendered Home menu row on the drawn screen.
	homeRow := -1
	for y := range H {
		var sb strings.Builder
		for x := range W {
			str, _, _ := screen.Get(x, y)
			sb.WriteString(str)
		}
		if strings.Contains(sb.String(), "Directory") {
			homeRow = y
			break
		}
	}
	if homeRow < 0 {
		t.Fatalf("Home menu row not rendered in the %dx%d viewport after ensureVisible", W, H)
	}
	if gotX, gotY := screen.shownX, screen.shownY; gotY != homeRow {
		wantX, _, _ := bd.cursorScreenXY()
		t.Errorf("hardware cursor at (%d,%d), but Home menu renders at row %d (model (%d,%d)); cursor must sit on the focused line",
			gotX, gotY, homeRow, wantX, homeRow)
	}
}
