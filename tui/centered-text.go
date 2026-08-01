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

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// centeredText is a top-filled, multi-line text primitive that centers each
// line with urwid's CEIL-left padding — the extra column goes to the LEFT
// when the slack is odd — matching urwid.Text(align='center') inside a
// Filler('top') (e.g. nomadnet's KnownNodes empty-state, Network.py:833-882).
// tview.TextView.AlignCenter floors the left padding instead, so a 1-cell-wide
// glyph or a 29-char line in a 50-wide pane lands one column to the right of
// the original. This primitive closes that gap.
type centeredText struct {
	*tview.Box
	lines []string
	color tcell.Color
}

// newCenteredText builds a top-filled centered-text primitive holding the
// given lines (already split on newlines), drawn in the given color.
func newCenteredText(color tcell.Color, lines ...string) *centeredText {
	return &centeredText{Box: tview.NewBox(), lines: lines, color: color}
}

// GetText returns the lines joined by newlines, for parity with tview.TextView
// callers that inspect the empty-state text.
func (c *centeredText) GetText() string { return strings.Join(c.lines, "\n") }

// Draw renders each line ceil-left-centered at the top of the inner rect.
func (c *centeredText) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, w, h := c.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	for i, line := range c.lines {
		if i >= h {
			break
		}
		rw := runewidth.StringWidth(line)
		// urwid ceil-left centering: leftPad = ceil((w - rw) / 2).
		leftPad := (w - rw + 1) / 2
		if leftPad < 0 {
			leftPad = 0
		}
		if x+leftPad < x+w {
			tview.Print(screen, line, x+leftPad, y+i, w-leftPad, tview.AlignLeft, c.color)
		}
	}
}
