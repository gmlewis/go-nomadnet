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
	"math"
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
	for y := range h {
		var b strings.Builder
		for x := range w {
			c, _, _, _ := cellContent(screen, x, y)
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
	for i := range 40 {
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
			t.Errorf("rightmost column row %v = ┃, want no thumb (content fits): col=%q", i, string(col))
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
	for i := range 40 {
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

// TestScrollBarThumbSizeWrappedContent is the regression test for the
// height-dependent GetWrappedLineCount bug: with wrapped content that overflows
// a small visible height, the thumb size must reflect the TRUE total wrapped row
// count (as measured by a full-height parse), not the partial count
// GetWrappedLineCount returns after a small-height Draw. The old code drew an
// oversized thumb; this test pins the urwid thumb-size formula against the
// ground-truth total.
func TestScrollBarThumbSizeWrappedContent(t *testing.T) {
	t.Parallel()
	const contentW = 10 // ScrollBar content width (w=11 → cw=10)
	tv := tview.NewTextView()
	tv.SetScrollable(true)
	tv.SetWrap(true)
	tv.SetWordWrap(true)
	// "word " is 5 cells; at width 10 two words fit per row ("word word"), so
	// 50 words wrap to 25 rows — well over the visible height of 10.
	tv.SetText(strings.Repeat("word ", 50))

	// Ground truth: a full-height parse wraps every line, so
	// GetWrappedLineCount is reliable here. At a small height it under-counts.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(contentW, 1000)
	tv.SetRect(0, 0, contentW, 1000)
	tv.Draw(screen)
	trueRowsMax := tv.GetWrappedLineCount()
	if trueRowsMax <= 0 {
		t.Fatalf("trueRowsMax = %v, want > 0", trueRowsMax)
	}

	const h = 10
	wantThumb := max(int(math.Round(math.Min(1.0, float64(h)/float64(trueRowsMax))*float64(h))), 1)

	s := NewScrollBar(tv)
	rows := drawScrollBar(t, s, contentW+1, h)
	col := scrollBarColumn(rows, contentW+1)

	thumbStart, thumbEnd := -1, -1
	for i, c := range col {
		if c == scrollBarThumb {
			if thumbStart < 0 {
				thumbStart = i
			}
			thumbEnd = i
		}
	}
	if thumbStart < 0 {
		t.Fatalf("no thumb drawn; col=%q", string(col))
	}
	gotThumb := thumbEnd - thumbStart + 1
	if gotThumb != wantThumb {
		t.Errorf("thumb height = %v (rows %v-%v), want %v (h=%v trueRowsMax=%v); col=%q",
			gotThumb, thumbStart, thumbEnd, wantThumb, h, trueRowsMax, string(col))
	}
}

// TestScrollBarWheelMultiplier pins the ScrollBar's per-primitive wheel
// multiplier (applyWheelMultiplier, installed in NewScrollBar): a wheel notch
// scrolls mouseWheelLines rows in one delivery via ScrollTo, and at the
// top/bottom boundary it declines to consume so tview skips the no-op redraw
// (the fix for the "scroll hangs at the ends" symptom and the trackEnd
// jump-after-topic-switch bug). mid-page notches consume and move the offset
// by N rows, not tview's default 1.
func TestScrollBarWheelMultiplier(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	const delta = 3
	SetMouseWheelLines(delta)

	tv := tview.NewTextView()
	tv.SetScrollable(true)
	tv.SetWrap(false)
	var b strings.Builder
	for i := range 40 {
		b.WriteString("line")
		b.WriteString(itoa(i))
		b.WriteByte('\n')
	}
	tv.SetText(b.String())

	s := NewScrollBar(tv)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	const w, h = 20, 10
	screen.SetSize(w, h)
	s.SetRect(0, 0, w, h)
	s.Text.Focus(func(p tview.Primitive) {})
	s.Draw(screen) // settle the line index + clamp lineOffset to 0

	// total is the TextView's own lineIndex length — the same figure the capture
	// clamps against (GetWrappedLineCount) — so the boundary math agrees. With
	// SetWrap(false) and short lines there is no wrapping, so this also equals
	// the ScrollBar's wrappedRows total.
	total := tv.GetWrappedLineCount()
	posmax := total - h
	if posmax <= 0 {
		t.Fatalf("need content overflow for a boundary test: total=%v h=%v", total, h)
	}

	handler := s.MouseHandler()
	ev := func() *tcell.EventMouse { return tcell.NewEventMouse(w/2, h/2, tcell.ButtonNone, tcell.ModNone) }
	setFocus := func(p tview.Primitive) {}

	if row, _ := tv.GetScrollOffset(); row != 0 {
		t.Fatalf("after Draw at top: lineOffset=%v, want 0", row)
	}

	// At the top, scrolling up is a no-op: must NOT consume, and lineOffset
	// must stay 0 (the old behavior returned consumed=true and left it at -1).
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); consumed {
		t.Error("ScrollUp at top: consumed=true, want false (no-op should skip redraw)")
	}
	if row, _ := tv.GetScrollOffset(); row != 0 {
		t.Errorf("ScrollUp at top moved lineOffset to %v, want unchanged 0", row)
	}

	// Scroll to the bottom and re-Draw so lineOffset clamps to posmax.
	tv.ScrollTo(1<<20, 0)
	s.Draw(screen)
	if row, _ := tv.GetScrollOffset(); row != posmax {
		t.Fatalf("after ScrollTo end: lineOffset=%v, want posmax=%v (total=%v)", row, posmax, total)
	}

	// At the bottom, scrolling down is a no-op: must NOT consume.
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); consumed {
		t.Error("ScrollDown at bottom: consumed=true, want false (no-op should skip redraw)")
	}
	if row, _ := tv.GetScrollOffset(); row != posmax {
		t.Errorf("ScrollDown at bottom moved lineOffset to %v, want unchanged %v", row, posmax)
	}

	// Mid-page: a wheel notch must consume AND move the offset by delta rows.
	mid := posmax / 2
	tv.ScrollTo(mid, 0)
	s.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); !consumed {
		t.Error("ScrollDown at mid: consumed=false, want true")
	}
	if row, _ := tv.GetScrollOffset(); row != mid+delta {
		t.Errorf("ScrollDown at mid moved lineOffset to %v, want %v", row, mid+delta)
	}

	// Scroll up from mid: must consume and decrement by delta.
	tv.ScrollTo(mid, 0)
	s.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); !consumed {
		t.Error("ScrollUp at mid: consumed=false, want true")
	}
	if row, _ := tv.GetScrollOffset(); row != mid-delta {
		t.Errorf("ScrollUp at mid moved lineOffset to %v, want %v", row, mid-delta)
	}
}

// TestScrollBarRowCountCache pins the wrappedRows cache invariant: the cached
// total must equal an independent recomputation, and it must stay valid across
// a scroll (cache hit — text/width unchanged) while being recomputed when the
// text changes (cache miss). This guards the perf fix that stopped Draw from
// re-wrapping the whole document every frame.
func TestScrollBarRowCountCache(t *testing.T) {
	t.Parallel()
	const w, h = 40, 8
	tv := tview.NewTextView()
	tv.SetScrollable(true).SetWrap(true).SetWordWrap(true)
	tv.SetText(benchSyntheticMicron(64)) // large enough to overflow h

	s := NewScrollBar(tv)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	s.SetRect(0, 0, w, h)

	cw := w - 1
	truth := func() int { return wrappedRowCount(tv.GetText(true), cw) }

	// First Draw: cache miss → populates cache. Cached value must match ground truth.
	s.Draw(screen)
	if s.cachedRowsMax != truth() {
		t.Fatalf("after first Draw: cachedRowsMax = %v, want ground truth %v", s.cachedRowsMax, truth())
	}
	wantFirst := s.cachedRowsMax
	if wantFirst <= h {
		t.Fatalf("want content to overflow h=%v for a meaningful scroll test, got rowsMax=%v", h, wantFirst)
	}

	// Scroll the TextView (offset only — text/width unchanged) and re-Draw: this
	// is a cache hit. The cached total must be unchanged and still match truth.
	tv.ScrollTo(wantFirst/2, 0)
	s.Draw(screen)
	if s.cachedRowsMax != wantFirst {
		t.Errorf("after scroll (cache hit): cachedRowsMax = %v, want unchanged %v", s.cachedRowsMax, wantFirst)
	}
	if s.cachedRowsMax != truth() {
		t.Errorf("after scroll: cachedRowsMax = %v, want ground truth %v", s.cachedRowsMax, truth())
	}

	// Change the text (cache miss). The cached total must recompute and match
	// the new ground truth, differing from the old.
	tv.SetText(benchSyntheticMicron(8)) // much shorter → fewer wrapped rows
	s.Draw(screen)
	if s.cachedRowsMax == wantFirst {
		t.Errorf("after SetText (cache miss): cachedRowsMax still %v, want recomputed", s.cachedRowsMax)
	}
	if s.cachedRowsMax != truth() {
		t.Errorf("after SetText: cachedRowsMax = %v, want new ground truth %v", s.cachedRowsMax, truth())
	}

	// Change the width (cache miss). The cached total must recompute at the new
	// width and match truth at that width.
	newCW := w/2 - 1
	s.SetRect(0, 0, w/2, h)
	s.Draw(screen)
	if s.cachedRowsMax != wrappedRowCount(tv.GetText(true), newCW) {
		t.Errorf("after width change: cachedRowsMax = %v, want ground truth at cw=%v = %v",
			s.cachedRowsMax, newCW, wrappedRowCount(tv.GetText(true), newCW))
	}
}
