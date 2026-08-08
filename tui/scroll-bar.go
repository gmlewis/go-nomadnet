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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// scrollBarThumb is the scrollbar handle character (urwid ScrollBar thumb_char,
// Guide.py:232 / Browser.py:628): U+2503 BOX DRAWINGS HEAVY VERTICAL.
const scrollBarThumb = '┃'

// scrollBarTrough is the scrollbar trough character (urwid ScrollBar
// trough_char): a plain space, so the trough reads as blank default bg.
const scrollBarTrough = ' '

// scrollableText is the subset of *tview.TextView ScrollBar needs: a primitive
// that can be laid out/drawn and queried for its wrapped row count and scroll
// offset. *tview.TextView satisfies it directly; the Guide's guideReader
// (which embeds *tview.TextView and overrides SetRect to re-render on resize)
// satisfies it too, so the wrapper dispatches to the override.
type scrollableText interface {
	tview.Primitive
	GetWrappedLineCount() int
	GetScrollOffset() (int, int)
	GetText(stripTags bool) string
}

// ScrollBar wraps a scrollable TextView with a right-edge scrollbar, mirroring
// urwid's `ScrollBar(Scrollable(widget), thumb_char="┃", trough_char=" ")`
// (Guide.py:232, Browser.py:628, Log.py). The wrapped TextView owns all
// content, wrapping and keyboard scrolling (Up/Down/PgUp/PgDn/Home/End when
// SetScrollable(true)); this primitive reserves one column on the right for the
// bar and draws it after the TextView so the thumb reflects the current scroll
// position.
//
// The bar is only drawn when the content overflows the visible height — when it
// fits, the TextView gets the full width and no bar is drawn (matching urwid's
// "Canvas fits without scrolling - no scrollbar needed" branch,
// scrollable.py:554-556). The thumb size is proportional to the visible/total
// row ratio and its vertical position to the scroll offset, per urwid's
// scrollable.py:558-568. The thumb uses the "scrollbar" palette color
// (TextUI.py: "#444" → theme.go 0x444444); the trough is default blank.
type ScrollBar struct {
	*tview.Box
	Text scrollableText

	// thumbColor is the foreground color of the thumb character. Defaults to
	// the "scrollbar" palette color; callers (e.g. a themed reader) may override
	// it via SetThumbColor.
	thumbColor tcell.Color

	// row-count cache for Draw. rowsMax (the total wrapped row count) only
	// changes when the Text content or content width changes — NOT on scroll,
	// where only the offset moves. Recomputing it every Draw via GetText(true)
	// tag-strip + WordWrap over the whole document was the guide-scroll hotspot
	// (~60% of scroll CPU in the live pprof), all pure waste on scroll. The
	// cache is keyed on the raw tagged text (GetText(false), a single memcpy —
	// tag-stripping is deterministic, so identical tagged text ⇒ identical
	// stripped text ⇒ identical row count) plus the content width, so a scroll
	// is a cache hit and skips the per-frame full-document re-wrap.
	cachedRaw     string
	cachedWidth   int
	cachedRowsMax int
}

// NewScrollBar wraps the given scrollable TextView with a right-edge scrollbar.
// The caller may continue to configure the TextView (SetText, SetScrollable,
// SetWrap, …) via the Text field. thumbColor defaults to the "scrollbar" theme
// color.
func NewScrollBar(text scrollableText) *ScrollBar {
	if text == nil {
		text = tview.NewTextView()
	}
	return &ScrollBar{Box: tview.NewBox(), Text: text, thumbColor: darkColors["scrollbar"]}
}

// SetThumbColor sets the foreground color used for the thumb character.
func (s *ScrollBar) SetThumbColor(c tcell.Color) *ScrollBar {
	s.thumbColor = c
	return s
}

// SetRect lays out the wrapped TextView. The TextView initially gets width-1
// (reserving the scrollbar column); Draw re-expands it to the full width when
// the content fits and no bar is needed.
func (s *ScrollBar) SetRect(x, y, w, h int) {
	s.Box.SetRect(x, y, w, h)
	s.Text.SetRect(x, y, s.contentWidth(w), h)
}

// contentWidth returns the TextView width when a scrollbar column is reserved
// (w-1, clamped to >=0); the Draw path may give the full width back when the
// content fits.
func (s *ScrollBar) contentWidth(w int) int {
	if w >= 2 {
		return w - 1
	}
	return w
}

// wrappedRowCount returns the total number of wrapped display rows the given
// text occupies at width, matching tview.TextView's wrapping: tview.WordWrap is
// applied per "\n"-separated line (empty lines count as one row). See Draw for
// why this is used instead of (*tview.TextView).GetWrappedLineCount.
func wrappedRowCount(text string, width int) int {
	if width <= 0 || text == "" {
		return 0
	}
	total := 0
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			total++
			continue
		}
		wrapped := tview.WordWrap(line, width)
		if len(wrapped) == 0 {
			total++
		} else {
			total += len(wrapped)
		}
	}
	return total
}

// wrappedRows returns the total wrapped row count of the Text content at the
// content width cw, using the row-count cache. The total only changes when the
// text or content width changes — not on scroll — so a scroll is a cache hit
// and skips the GetText(true) tag-strip + WordWrap over the whole document that
// was the guide-scroll hotspot. The raw tagged text (GetText(false), a single
// memcpy with no per-char work) is the key: stripping is deterministic, so
// identical tagged text yields identical stripped text and thus identical row
// count. See Draw for why the total is computed from the stripped text rather
// than GetWrappedLineCount.
func (s *ScrollBar) wrappedRows(cw int) int {
	raw := s.Text.GetText(false)
	if raw == s.cachedRaw && cw == s.cachedWidth {
		return s.cachedRowsMax
	}
	s.cachedRaw = raw
	s.cachedWidth = cw
	s.cachedRowsMax = wrappedRowCount(s.Text.GetText(true), cw)
	return s.cachedRowsMax
}

// Draw draws the wrapped TextView then, if the content overflows, the scrollbar
// in the rightmost column. When the content fits the TextView is redrawn at the
// full width and no bar is drawn.
func (s *ScrollBar) Draw(screen tcell.Screen) {
	s.Box.DrawForSubclass(screen, s)
	x, y, w, h := s.GetRect()
	if w <= 0 || h <= 0 {
		return
	}

	// First pass: render the TextView at width-1 and measure the total wrapped
	// row count, mirroring urwid's render-at-ow_size then rows_max(ow_size)
	// check. The total comes from the cached wrappedRows (see wrappedRows for
	// why it is computed from the stripped text via tview.WordWrap rather than
	// GetWrappedLineCount, and why it is cached: it does not change on scroll).
	cw := s.contentWidth(w)
	s.Text.SetRect(x, y, cw, h)
	s.Text.Draw(screen)
	rowsMax := s.wrappedRows(cw)
	pos, _ := s.Text.GetScrollOffset()

	if rowsMax <= h || w < 2 {
		// Content fits (or no room for a bar): re-expand the TextView to the
		// full width and redraw with no scrollbar (urwid render_no_scrollbar).
		if cw != w {
			s.Text.SetRect(x, y, w, h)
			s.Text.Draw(screen)
		}
		return
	}

	// Scrollbar needed. Compute thumb geometry per urwid scrollable.py:558-568.
	thumbWeight := math.Min(1.0, float64(h)/float64(rowsMax))
	thumbHeight := max(int(math.Round(thumbWeight*float64(h))), 1)
	posmax := rowsMax - h
	topWeight := 0.0
	if posmax > 0 {
		topWeight = float64(pos) / float64(posmax)
	}
	topHeight := int(float64(h-thumbHeight) * topWeight)
	if topHeight == 0 && topWeight > 0 {
		topHeight = min(1, h-thumbHeight)
	}
	bottomHeight := h - thumbHeight - topHeight

	barX := x + w - 1
	thumbStyle := tcell.StyleDefault.Foreground(s.thumbColor)
	troughStyle := tcell.StyleDefault
	row := y
	for i := 0; i < topHeight && row < y+h; i++ {
		screen.SetContent(barX, row, scrollBarTrough, nil, troughStyle)
		row++
	}
	for i := 0; i < thumbHeight && row < y+h; i++ {
		screen.SetContent(barX, row, scrollBarThumb, nil, thumbStyle)
		row++
	}
	for i := 0; i < bottomHeight && row < y+h; i++ {
		screen.SetContent(barX, row, scrollBarTrough, nil, troughStyle)
		row++
	}
}

// Focus forwards to the wrapped TextView so its scroll/cursor state tracks.
func (s *ScrollBar) Focus(delegate func(p tview.Primitive)) {
	s.Text.Focus(delegate)
}

// Blur forwards to the wrapped TextView.
func (s *ScrollBar) Blur() {
	s.Text.Blur()
}

// HasFocus reports the wrapped TextView's focus state.
func (s *ScrollBar) HasFocus() bool {
	return s.Text.HasFocus()
}

// InputHandler delegates to the wrapped TextView (Up/Down/PgUp/PgDn/Home/End
// scrolling when scrollable).
func (s *ScrollBar) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return s.Text.InputHandler()
}

// scrollWheelNoOp reports whether a wheel action on a scrollable text region
// would change nothing: scrolling up at/below the top, scrolling down at/above
// the bottom, or content that fits the viewport (nothing to scroll at all).
// Returning true lets a MouseHandler decline to consume the event so tview
// skips the full Clear+Draw+Show it would otherwise run for an identical frame
// — the "scroll hangs at the ends" symptom. tview clamps lineOffset in Draw,
// not in its wheel handler (textview.go: MouseScrollUp does
// `t.lineOffset--; consumed = true` with no clamp), so without this guard every
// past-the-end notch is a full no-op redraw.
func scrollWheelNoOp(action tview.MouseAction, offset, height, total int) bool {
	if total <= 0 || height <= 0 || total <= height {
		return true
	}
	switch action {
	case tview.MouseScrollUp:
		return offset <= 0
	case tview.MouseScrollDown:
		return offset >= total-height
	}
	return false
}

// MouseHandler delegates to the wrapped TextView (whose rect was set in
// SetRect/Draw), but short-circuits wheel events at the scroll boundaries: when
// the page is already at the top/bottom the underlying TextView would still
// return consumed=true (it bumps lineOffset past the clamp point and lets Draw
// fix it up), forcing a full no-op redraw per notch. Declining to consume at the
// boundary makes tview skip that redraw (application.go: `if consumed { draw }`).
func (s *ScrollBar) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	base := s.Text.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			_, _, w, h := s.GetRect()
			row, _ := s.Text.GetScrollOffset()
			if scrollWheelNoOp(action, row, h, s.wrappedRows(s.contentWidth(w))) {
				return false, nil
			}
		}
		return base(action, event, setFocus)
	}
}
