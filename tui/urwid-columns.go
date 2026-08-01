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
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// urwidColumnWidths returns the per-column widths urwid's Columns widget would
// assign to a row of all-weighted columns at the given total width, mirroring
// column_widths (urwid/widget/columns.py:767-849) for the common case where the
// columns fit. Each weighted column reserves min_width (1) plus one dividechar
// per column; the remaining space (`shared`) is distributed by weight using
// urwid's subtractive rounding — items are processed in ascending (weight,
// index) order and each takes round(grow*weight/wtotal), so the leftover lands
// on the heaviest/latest column rather than tview Flex's last-item. This matters
// for parity: weights [1,1,3] at maxcol 50 dividechars 1 yield [10,10,28] (urwid)
// but tview Flex would yield [9,9,28] (leftover to the last item).
func urwidColumnWidths(maxcol int, weights []int, dividechars int) []int {
	n := len(weights)
	widths := make([]int, n)
	if n == 0 {
		return widths
	}
	const minWidth = 1
	// Each column first reserves minWidth plus one dividechar (urwid iterates
	// contents and subtracts static_w + dividechars from shared for every
	// column, weighted or not; a weighted column's static_w is min_width).
	shared := maxcol + dividechars
	for i := range weights {
		widths[i] = minWidth
		shared -= minWidth + dividechars
	}

	// Distribute `shared` among the weighted columns in ascending (weight,
	// index) order, matching urwid's `sorted(weighted)` + subtractive loop.
	type witem struct {
		weight int
		idx    int
	}
	weighted := make([]witem, n)
	for i, w := range weights {
		weighted[i] = witem{w, i}
	}
	sort.SliceStable(weighted, func(a, b int) bool {
		if weighted[a].weight != weighted[b].weight {
			return weighted[a].weight < weighted[b].weight
		}
		return weighted[a].idx < weighted[b].idx
	})

	wtotal := 0
	for _, w := range weighted {
		wtotal += w.weight
	}
	grow := shared + n*minWidth
	for _, w := range weighted {
		if wtotal <= 0 {
			widths[w.idx] = minWidth
			continue
		}
		width := int(float64(grow)*float64(w.weight)/float64(wtotal) + 0.5)
		if width < minWidth {
			width = minWidth
		}
		widths[w.idx] = width
		grow -= width
		wtotal -= w.weight
	}
	return widths
}

// urwidColumns lays out a row of weighted children with blank dividechar
// columns between them, replicating urwid's Columns widget sizing (see
// urwidColumnWidths). Each child is given the full height of the row; a child
// shorter than the row is expected to blank-pad below itself (e.g. TabButton
// renders its brackets only on the first row). The row height is the maximum of
// the children's required heights at their computed widths (urwid Columns
// render height = max child height), obtainable via RequiredHeight.
type urwidColumns struct {
	*tview.Box
	children    []tview.Primitive
	weights     []int
	dividechars int
}

// newURWIDColumns builds a urwid-style Columns row of the given weighted
// children with blank columns of width dividechars between them.
func newURWIDColumns(dividechars int, children ...tview.Primitive) *urwidColumns {
	weights := make([]int, len(children))
	for i := range children {
		weights[i] = 1
	}
	return &urwidColumns{
		Box:         tview.NewBox(),
		children:    children,
		weights:     weights,
		dividechars: dividechars,
	}
}

// SetWeight sets the weight of column i.
func (c *urwidColumns) SetWeight(i, weight int) *urwidColumns {
	if i >= 0 && i < len(c.weights) {
		c.weights[i] = weight
	}
	return c
}

// SetRect lays out the children at their urwid-computed widths across the row.
func (c *urwidColumns) SetRect(x, y, w, h int) {
	c.Box.SetRect(x, y, w, h)
	widths := urwidColumnWidths(w, c.weights, c.dividechars)
	cx := x
	for i, child := range c.children {
		cw := widths[i]
		if cw > 0 {
			child.SetRect(cx, y, cw, h)
			cx += cw + c.dividechars
		} else {
			child.SetRect(cx, y, 0, h)
		}
	}
}

// Draw draws each child at its laid-out rect.
func (c *urwidColumns) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	for _, child := range c.children {
		child.Draw(screen)
	}
}

// requiredHeightAt returns the row height urwid would give the columns at width
// w: the maximum of the children's required heights at their computed widths.
// A child implements heightRequest when it has a RequiredHeight(width) method;
// otherwise it is assumed to be one row tall.
func (c *urwidColumns) requiredHeightAt(w int) int {
	widths := urwidColumnWidths(w, c.weights, c.dividechars)
	max := 1
	for i, child := range c.children {
		hh := 1
		if rh, ok := child.(interface{ RequiredHeight(int) int }); ok {
			hh = rh.RequiredHeight(widths[i])
		}
		if hh > max {
			max = hh
		}
	}
	return max
}

// Focus, HasFocus, InputHandler and MouseHandler delegate to the focused child
// so the row behaves like a single selectable group (urwid Columns forwards
// focus to focus_position).
func (c *urwidColumns) Focus(delegate func(p tview.Primitive)) {
	if len(c.children) > 0 {
		c.children[0].Focus(delegate)
	}
}

func (c *urwidColumns) HasFocus() bool {
	for _, child := range c.children {
		if child.HasFocus() {
			return true
		}
	}
	return false
}

func (c *urwidColumns) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		for _, child := range c.children {
			if child.HasFocus() {
				if h := child.InputHandler(); h != nil {
					h(event, setFocus)
				}
				return
			}
		}
	}
}

func (c *urwidColumns) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		for _, child := range c.children {
			if h := child.MouseHandler(); h != nil {
				if consumed, capture := h(action, event, setFocus); consumed {
					return consumed, capture
				}
			}
		}
		return false, nil
	}
}
