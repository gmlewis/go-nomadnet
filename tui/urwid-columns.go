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
	focusIndex  int
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
		focusIndex:  -1,
	}
}

// isFocusable returns true if child can receive focus (e.g. UrwidButton, ReadlineEdit, RadioButton, Checkbox).
// Plain layout spacers like *tview.Box or static *tview.TextView return false.
func isFocusable(p tview.Primitive) bool {
	if p == nil {
		return false
	}
	if s, ok := p.(interface{ IsSelectable() bool }); ok {
		return s.IsSelectable()
	}
	// Plain *tview.Box (created with tview.NewBox()) is a layout spacer.
	if _, isBox := p.(*tview.Box); isBox {
		return false
	}
	return p.InputHandler() != nil
}

// FocusIndex returns the index of the currently focused child, or -1.
func (c *urwidColumns) FocusIndex() int {
	return c.focusIndex
}

// SetFocusIndex sets the focused child index if valid and focusable.
func (c *urwidColumns) SetFocusIndex(index int) *urwidColumns {
	if index >= 0 && index < len(c.children) && isFocusable(c.children[index]) {
		if c.focusIndex >= 0 && c.focusIndex < len(c.children) && c.children[c.focusIndex] != c.children[index] {
			if bl, ok := c.children[c.focusIndex].(interface{ Blur() }); ok {
				bl.Blur()
			}
		}
		c.focusIndex = index
	}
	return c
}

// moveFocus shifts focus by delta among focusable children.
func (c *urwidColumns) moveFocus(delta int, setFocus func(p tview.Primitive)) {
	n := len(c.children)
	if n == 0 {
		return
	}
	var focusable []int
	for i, child := range c.children {
		if isFocusable(child) {
			focusable = append(focusable, i)
		}
	}
	if len(focusable) == 0 {
		return
	}

	if c.focusIndex >= 0 && c.focusIndex < n {
		if bl, ok := c.children[c.focusIndex].(interface{ Blur() }); ok {
			bl.Blur()
		}
	}

	currPos := -1
	for pos, idx := range focusable {
		if idx == c.focusIndex {
			currPos = pos
			break
		}
	}
	if currPos == -1 {
		if delta >= 0 {
			currPos = 0
		} else {
			currPos = len(focusable) - 1
		}
	} else {
		currPos = (currPos + delta%len(focusable) + len(focusable)) % len(focusable)
	}

	c.focusIndex = focusable[currPos]
	nw := c.children[c.focusIndex]
	if setFocus != nil {
		setFocus(nw)
	} else {
		nw.Focus(func(tview.Primitive) {})
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
	if len(c.children) == 0 {
		return
	}
	if c.focusIndex < 0 || c.focusIndex >= len(c.children) || !isFocusable(c.children[c.focusIndex]) {
		c.focusIndex = -1
		for i, child := range c.children {
			if isFocusable(child) {
				c.focusIndex = i
				break
			}
		}
	}
	if c.focusIndex >= 0 && c.focusIndex < len(c.children) {
		c.children[c.focusIndex].Focus(delegate)
	}
}

func (c *urwidColumns) Blur() {
	c.Box.Blur()
	for _, child := range c.children {
		if bl, ok := child.(interface{ Blur() }); ok {
			bl.Blur()
		}
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
		if len(c.children) == 0 {
			return
		}

		for i, child := range c.children {
			if child.HasFocus() {
				c.focusIndex = i
				break
			}
		}

		if c.focusIndex < 0 || c.focusIndex >= len(c.children) {
			return
		}

		focusedChild := c.children[c.focusIndex]

		// For text input fields (e.g. ReadlineEdit or tview.InputField), plain KeyLeft and KeyRight move the text cursor.
		// Tab and Backtab move column focus.
		// For other widgets (e.g. UrwidButton), KeyLeft, KeyRight, KeyTab, KeyBacktab all move column focus.
		isTextInput := false
		if _, isRL := focusedChild.(*ReadlineEdit); isRL {
			isTextInput = true
		} else if _, isIF := focusedChild.(*tview.InputField); isIF {
			isTextInput = true
		}

		switch event.Key() {
		case tcell.KeyRight:
			if !isTextInput {
				c.moveFocus(1, setFocus)
				return
			}
		case tcell.KeyLeft:
			if !isTextInput {
				c.moveFocus(-1, setFocus)
				return
			}
		case tcell.KeyTab:
			c.moveFocus(1, setFocus)
			return
		case tcell.KeyBacktab:
			c.moveFocus(-1, setFocus)
			return
		}

		if h := focusedChild.InputHandler(); h != nil {
			h(event, setFocus)
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
