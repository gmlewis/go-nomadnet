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
	"github.com/gmlewis/tcell/v2"
)

// clipScreen is a screen that only accepts what falls inside one rectangle, and
// drops the rest.
//
// urwid gives every widget the size it is to draw in and keeps the canvas it
// returns at that size: a widget whose content does not fit is *clipped*, and
// nothing it draws can reach a cell outside its own rectangle. tview has no such
// rule. A primitive is told its rectangle with SetRect and is trusted to respect
// it, and the one that does not is the Flex: its items are given the heights they
// ask for, so a column of fixed-height rows that do not fit the Flex's own height
// draws its last rows *below* it — over whatever is underneath, which for a
// dialog is the border it is inside of, the pane below it, and the shortcut bar at
// the bottom of the screen.
//
// This is that rule for the places where it matters: drawing a primitive through a
// clipScreen gives it the view of the screen its rectangle is, so the rows and
// columns outside it are simply not there. It is the whole mechanism of urwid's
// clipping, and it is deliberately not a scroll: the content that does not fit is
// what urwid would have dropped as well.
//
// The rectangle is inclusive of both corners, in screen cells, and is set once
// when the clip is made. Everything the Screen interface needs to do besides
// drawing — sizes, events, styles — is the wrapped screen's own business and is
// forwarded untouched.
type clipScreen struct {
	tcell.Screen

	x0, y0 int // top-left cell of the rectangle, inclusive
	x1, y1 int // bottom-right cell of the rectangle, inclusive
}

// newClipScreen wraps screen so that only the cells of the given rectangle accept
// drawing.
func newClipScreen(screen tcell.Screen, x, y, w, h int) clipScreen {
	return clipScreen{
		Screen: screen,
		x0:     x,
		y0:     y,
		x1:     x + w - 1,
		y1:     y + h - 1,
	}
}

// holds reports whether a cell is inside the rectangle.
func (c clipScreen) holds(x, y int) bool {
	return x >= c.x0 && x <= c.x1 && y >= c.y0 && y <= c.y1
}

// SetContent writes a cell, unless it is outside the rectangle.
func (c clipScreen) SetContent(x, y int, primary rune, combining []rune, style tcell.Style) {
	if !c.holds(x, y) {
		return
	}
	c.Screen.SetContent(x, y, primary, combining, style)
}

// SetCell writes a cell, unless it is outside the rectangle.
func (c clipScreen) SetCell(x, y int, style tcell.Style, ch ...rune) {
	if !c.holds(x, y) {
		return
	}
	c.Screen.SetCell(x, y, style, ch...)
}

// Put writes a grapheme, unless it starts outside the rectangle. A grapheme that
// is dropped displays nothing, which is the width Put reports.
func (c clipScreen) Put(x, y int, str string, style tcell.Style) (string, int) {
	if !c.holds(x, y) {
		return str, 0
	}
	return c.Screen.Put(x, y, str, style)
}

// PutStr writes a string, unless it starts outside the rectangle.
func (c clipScreen) PutStr(x, y int, str string) {
	if !c.holds(x, y) {
		return
	}
	c.Screen.PutStr(x, y, str)
}

// PutStrStyled writes a styled string, unless it starts outside the rectangle.
func (c clipScreen) PutStrStyled(x, y int, str string, style tcell.Style) {
	if !c.holds(x, y) {
		return
	}
	c.Screen.PutStrStyled(x, y, str, style)
}

// Fill fills the part of the rectangle a requested fill overlaps, and nothing
// else: a fill of the whole screen, which is how a widget paints its background,
// is a fill of the widget's own rectangle here.
func (c clipScreen) Fill(r rune, style tcell.Style) {
	w, h := c.Screen.Size()
	rx0, ry0 := max(c.x0, 0), max(c.y0, 0)
	rx1, ry1 := min(c.x1, w-1), min(c.y1, h-1)
	for y := ry0; y <= ry1; y++ {
		for x := rx0; x <= rx1; x++ {
			c.Screen.SetContent(x, y, r, nil, style)
		}
	}
}

// Clear clears the rectangle, not the screen.
func (c clipScreen) Clear() { c.Fill(' ', tcell.StyleDefault) }

// ShowCursor shows the cursor only where it is inside the rectangle. A cursor
// outside it is a cursor on whatever the clipped widget is drawn over, so it is
// hidden rather than moved.
func (c clipScreen) ShowCursor(x, y int) {
	if !c.holds(x, y) {
		c.Screen.HideCursor()
		return
	}
	c.Screen.ShowCursor(x, y)
}

// LockRegion locks the part of the rectangle a requested lock overlaps.
func (c clipScreen) LockRegion(x, y, width, height int, lock bool) {
	rx0, ry0 := max(x, c.x0), max(y, c.y0)
	rx1, ry1 := min(x+width-1, c.x1), min(y+height-1, c.y1)
	if rx1 < rx0 || ry1 < ry0 {
		return
	}
	c.Screen.LockRegion(rx0, ry0, rx1-rx0+1, ry1-ry0+1, lock)
}
