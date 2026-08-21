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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// cursorAlignCase is one (align, width, cursor rune-offset) → expected hardware
// cursor column x, where the golden x is urwid's calc_coords for the same text,
// width, and alignment (the source-of-truth Python LinkableText.get_cursor_coords,
// MicronParser.py:994-1003). y is always 0 for these single-row cases.
type cursorAlignCase struct {
	align micron.Alignment
	width int
	pos   int
	wantX int
}

// minimalRightMenu renders as one right-justified line whose plain text is
// " Home   Authors   Type   Tags" (len 29) with link parts starting at rune
// offsets 1 (Home), 8 (Authors), 18 (Type), 25 (Tags).
const minimalRightMenu = "`r `[Home`:/page/a.mu]   `[Authors`:/page/b.mu]   `[Type`:/page/c.mu]   `[Tags`:/page/d.mu]"

// minimalCenterMenu is the same menu centered (`c) instead of right-justified.
const minimalCenterMenu = "`c `[Home`:/page/a.mu]   `[Authors`:/page/b.mu]   `[Type`:/page/c.mu]   `[Tags`:/page/d.mu]"

// goldenRightMenu and goldenCenterMenu are urwid calc_coords outputs for the
// minimalRightMenu plain text at the widths below (single-row cases, where the
// line fits and urwid's per-row sc equals the full text width 29):
//
//	right  pad = width - 29          → x = pad + pos
//	center pad = (width - 29 + 1)//2 → x = pad + pos   (urwid text_layout.py:177)
var goldenRightMenu = []cursorAlignCase{
	{micron.AlignRight, 120, 1, 92}, {micron.AlignRight, 120, 8, 99}, {micron.AlignRight, 120, 18, 109}, {micron.AlignRight, 120, 25, 116},
	{micron.AlignRight, 100, 1, 72}, {micron.AlignRight, 100, 8, 79}, {micron.AlignRight, 100, 18, 89}, {micron.AlignRight, 100, 25, 96},
	{micron.AlignRight, 60, 1, 32}, {micron.AlignRight, 60, 8, 39}, {micron.AlignRight, 60, 18, 49}, {micron.AlignRight, 60, 25, 56},
}

var goldenCenterMenu = []cursorAlignCase{
	{micron.AlignCenter, 120, 1, 47}, {micron.AlignCenter, 120, 8, 54}, {micron.AlignCenter, 120, 18, 64}, {micron.AlignCenter, 120, 25, 71},
	{micron.AlignCenter, 100, 1, 37}, {micron.AlignCenter, 100, 8, 44}, {micron.AlignCenter, 100, 18, 54}, {micron.AlignCenter, 100, 25, 61},
}

func runCursorAlignCases(t *testing.T, markup string, cases []cursorAlignCase) {
	t.Helper()
	for _, c := range cases {
		app := newTestApp()
		bd := NewBrowserDisplay(app)
		bd.currentMarkup = markup
		bd.renderPage()
		menu := findLine(bd, "Home")
		if menu < 0 {
			t.Fatalf("no menu line containing Home: lines=%d", len(bd.currentLines))
		}
		if got := bd.currentLines[menu].Align; got != c.align {
			t.Fatalf("align=%v want %v", got, c.align)
		}
		bd.content.SetRect(0, 0, c.width, 4)
		bd.focusLine = menu
		bd.lineCursors[menu] = c.pos
		gotX, gotY, ok := bd.cursorScreenXY()
		if !ok {
			t.Errorf("right w=%d pos=%d: cursorScreenXY ok=false", c.width, c.pos)
			continue
		}
		if gotX != c.wantX || gotY != 0 {
			t.Errorf("align=%v w=%d pos=%d: cursor (%d,%d), want (%d,0)",
				c.align, c.width, c.pos, gotX, gotY, c.wantX)
		}
	}
}

func TestCursorScreenXYRightAligned(t *testing.T) {
	t.Parallel()
	runCursorAlignCases(t, minimalRightMenu, goldenRightMenu)
}

func TestCursorScreenXYCenterAligned(t *testing.T) {
	t.Parallel()
	runCursorAlignCases(t, minimalCenterMenu, goldenCenterMenu)
}

// TestCursorScreenXYRetibooksMenu checks the exact right-justified menu the
// user reported (nomadnet/micron/testdata/parity/retibooks-index.mu line 1):
// plain text len 81, link parts at offsets 1,8,18,25,32,44,60,69,76. Golden x
// values are urwid calc_coords at w=120 and w=100 (single-row, sc=81):
//
//	right w=120 pad=39, w=100 pad=19.
func TestCursorScreenXYRetibooksMenu(t *testing.T) {
	t.Parallel()
	fixture := filepath.Join("..", "nomadnet", "micron", "testdata", "parity", "retibooks-index.mu")
	markup, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("retibooks fixture not readable: %v", err)
	}
	cases := []cursorAlignCase{
		{micron.AlignRight, 120, 1, 40}, {micron.AlignRight, 120, 8, 47}, {micron.AlignRight, 120, 18, 57}, {micron.AlignRight, 120, 25, 64},
		{micron.AlignRight, 120, 32, 71}, {micron.AlignRight, 120, 44, 83}, {micron.AlignRight, 120, 60, 99}, {micron.AlignRight, 120, 69, 108},
		{micron.AlignRight, 120, 76, 115},
		{micron.AlignRight, 100, 1, 20}, {micron.AlignRight, 100, 8, 27}, {micron.AlignRight, 100, 18, 37}, {micron.AlignRight, 100, 25, 44},
		{micron.AlignRight, 100, 32, 51}, {micron.AlignRight, 100, 44, 63}, {micron.AlignRight, 100, 60, 79}, {micron.AlignRight, 100, 69, 88},
		{micron.AlignRight, 100, 76, 95},
	}
	runCursorAlignCases(t, string(markup), cases)
}
