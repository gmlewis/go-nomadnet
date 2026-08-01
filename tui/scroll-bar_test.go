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

// drawScrollBar renders a ScrollBar on a simulation screen and returns the
// joined cell text per row, so the thumb position/height can be asserted
// against the urwid ScrollBar layout.
func drawScrollBar(t *testing.T, s *ScrollBar, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, h)
	s.SetRect(0, 0, w, h)
	s.Text.Focus(func(p tview.Primitive) {})
	s.Draw(screen)
	screen.Sync()
	rows := make([]string, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// scrollBarColumn returns the runes of the rightmost column (where the
// scrollbar is drawn), top to bottom.
func scrollBarColumn(rows []string, w int) []rune {
	col := make([]rune, len(rows))
	for y, r := range rows {
		runes := []rune(r)
		if len(runes) >= w {
			col[y] = runes[w-1]
		} else {
			col[y] = ' '
		}
	}
	return col
}

// TestScrollBarOverflowThumb verifies the scrollbar is drawn when content
// overflows: a TextView with many short lines at height 10 shows a thumb (┃) in
// the rightmost column with trough (space) above and below.
func TestScrollBarOverflowThumb(t *testing.T) {
	t.Parallel()
	tv := tview.NewTextView()
	tv.SetScrollable(true)
	tv.SetWrap(false)
	// 40 short lines, each shorter than the width so no wrapping.
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("line")
		b.WriteString(itoa(i))
		b.WriteByte('\n')
	}
	tv.SetText(b.String())

	s := NewScrollBar(tv)
	rows := drawScrollBar(t, s, 20, 10)

	// rowsMax(40) > h(10) → scrollbar present in the rightmost column.
	col := scrollBarColumn(rows, 20)
	hasThumb := false
	for _, c := range col {
		if c == scrollBarThumb {
			hasThumb = true
		}
	}
	if !hasThumb {
		t.Errorf("rightmost column = %q, want a ┃ thumb (content overflows)", string(col))
	}

	// At pos 0 the thumb sits at the top: top_height == 0, so the first thumb
	// row is row 0. Verify row 0 is the thumb.
	if col[0] != scrollBarThumb {
		t.Errorf("row 0 rightmost = %q, want ┃ (thumb at top when pos=0)", string(col[0]))
	}
}

// TestScrollBarFitsNoBar verifies no scrollbar is drawn (and the TextView gets
// the full width) when the content fits the visible height.
func TestScrollBarFitsNoBar(t *testing.T) {
	t.Parallel()
	tv := tview.NewTextView()
	tv.SetScrollable(true)
	tv.SetWrap(false)
	tv.SetText("only a few lines\nfit here\n") // 2 lines

	s := NewScrollBar(tv)
	rows := drawScrollBar(t, s, 20, 10)

	col := scrollBarColumn(rows, 20)
	for i, c := range col {
		if c == scrollBarThumb {
			t.Errorf("rightmost column row %d = ┃, want no thumb (content fits): col=%q", i, string(col))
		}
	}
	// The TextView should have used the full width, so "only a few lines"
	// (15 chars) appears starting at column 0 of row 0.
	if !strings.HasPrefix(rows[0], "only a few lines") {
		t.Errorf("row 0 = %q, want the TextView content at full width", rows[0])
	}
}

// TestScrollBarScrolledDown verifies the thumb moves down when scrolled: with
// the content scrolled to the end, the thumb's bottom row is the last row.
func TestScrollBarScrolledDown(t *testing.T) {
	t.Parallel()
	tv := tview.NewTextView()
	tv.SetScrollable(true)
	tv.SetWrap(false)
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("line")
		b.WriteString(itoa(i))
		b.WriteByte('\n')
	}
	tv.SetText(b.String())
	tv.ScrollTo(1<<20, 0) // scroll to the very end; Draw clamps lineOffset to posmax

	s := NewScrollBar(tv)
	rows := drawScrollBar(t, s, 20, 10)

	col := scrollBarColumn(rows, 20)
	// Scrolled to the bottom → thumb's bottom row is the last row (row 9).
	if col[len(col)-1] != scrollBarThumb {
		t.Errorf("last row rightmost = %q, want ┃ (thumb at bottom when scrolled to end): col=%q",
			string(col[len(col)-1]), string(col))
	}
}