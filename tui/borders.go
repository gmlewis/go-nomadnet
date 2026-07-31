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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Single-line (urwid BOX_SYMBOLS.LIGHT) border runes, captured verbatim from
// urwid/widget/constants.py:516 — the default urwid.LineBox characters the
// Python original renders for every bordered pane.
const (
	BorderHorizontal  = '─'
	BorderVertical    = '│'
	BorderTopLeft     = '┌'
	BorderTopRight    = '┐'
	BorderBottomLeft  = '└'
	BorderBottomRight = '┘'
)

// Rounded (urwid BOX_SYMBOLS.LIGHT *_ROUNDED) corner runes, used by the
// Interfaces detail and form boxes (nomadnet/ui/textui/Interfaces.py:1189-1196
// and 1454-1460). The horizontal and vertical lines stay the single-line set.
const (
	BorderTopLeftRounded     = '╭'
	BorderTopRightRounded    = '╮'
	BorderBottomLeftRounded  = '╰'
	BorderBottomRightRounded = '╯'
)

// ApplySingleLineBorders overrides tview's default *Focus border characters
// (double-line ╔═╗) with the single-line ┌─┐ set, so a focused box renders the
// same single-line border the Python original (urwid LineBox) always uses.
// tview's non-focus borders are already single-line. Call once at startup,
// before any Box is drawn.
func ApplySingleLineBorders() {
	tview.Borders.HorizontalFocus = BorderHorizontal
	tview.Borders.VerticalFocus = BorderVertical
	tview.Borders.TopLeftFocus = BorderTopLeft
	tview.Borders.TopRightFocus = BorderTopRight
	tview.Borders.BottomLeftFocus = BorderBottomLeft
	tview.Borders.BottomRightFocus = BorderBottomRight
}

// BorderedBox wraps a tview.Primitive with a manually drawn border and
// centered title, independent of the global tview.Borders. The border is
// single-line by default and rounded (╭─╮╰─╯) when Rounded is true, matching
// the urwid LineBox characters the Python original uses: single-line for most
// panes, rounded for the Interfaces detail/form boxes. Use it where a pane
// needs a non-default border style; everywhere else a plain tview.Box with
// SetBorder(true) plus ApplySingleLineBorders renders the correct single-line
// border.
type BorderedBox struct {
	*tview.Box
	content tview.Primitive
	title   string
	rounded bool
}

// NewBorderedBox returns a BorderedBox wrapping content with the given
// centered title. If rounded is true the corners are ╭╮╰╯.
func NewBorderedBox(title string, content tview.Primitive, rounded bool) *BorderedBox {
	return &BorderedBox{
		Box:     tview.NewBox(),
		content: content,
		title:   title,
		rounded: rounded,
	}
}

// SetRounded toggles rounded corners on the border.
func (b *BorderedBox) SetRounded(rounded bool) *BorderedBox {
	b.rounded = rounded
	return b
}

// Draw renders the border, centered title, and the wrapped content.
func (b *BorderedBox) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)

	x, y, w, h := b.GetRect()
	if w < 2 || h < 2 {
		return
	}

	style := tcell.StyleDefault
	tl, tr, bl, br := BorderTopLeft, BorderTopRight, BorderBottomLeft, BorderBottomRight
	if b.rounded {
		tl, tr, bl, br = BorderTopLeftRounded, BorderTopRightRounded, BorderBottomLeftRounded, BorderBottomRightRounded
	}

	// Corners.
	screen.SetContent(x, y, tl, nil, style)
	screen.SetContent(x+w-1, y, tr, nil, style)
	screen.SetContent(x, y+h-1, bl, nil, style)
	screen.SetContent(x+w-1, y+h-1, br, nil, style)

	// Top and bottom edges.
	for i := 1; i < w-1; i++ {
		screen.SetContent(x+i, y, BorderHorizontal, nil, style)
		screen.SetContent(x+i, y+h-1, BorderHorizontal, nil, style)
	}

	// Left and right edges.
	for i := 1; i < h-1; i++ {
		screen.SetContent(x, y+i, BorderVertical, nil, style)
		screen.SetContent(x+w-1, y+i, BorderVertical, nil, style)
	}

	// Centered title, drawn over the top edge (like urwid LineBox).
	if b.title != "" {
		r := []rune(b.title)
		tx := x + (w-len(r))/2
		for i, c := range r {
			p := tx + i
			if p > x && p < x+w-1 {
				screen.SetContent(p, y, c, nil, style)
			}
		}
	}

	// Content fills the inner rect.
	if b.content != nil {
		b.content.SetRect(x+1, y+1, w-2, h-2)
		b.content.Draw(screen)
	}
}

// InputHandler delegates key events to the wrapped content so a BorderedBox
// participates in focus, mirroring DialogLineBox.
func (b *BorderedBox) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if b.content == nil {
			return
		}
		if handler, ok := b.content.(interface {
			InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive))
		}); ok && handler != nil {
			handler.InputHandler()(event, setFocus)
		}
	})
}
