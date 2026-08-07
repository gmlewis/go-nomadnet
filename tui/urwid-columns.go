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

// urwidColumnWidthsEx extends urwidColumnWidths to support explicit fixed-width
// columns (e.g. NetworkDisplay's left pane width 52, right pane weight 1).
func urwidColumnWidthsEx(maxcol int, weights []int, fixedWidths []int, dividechars int) []int {
	n := len(weights)
	widths := make([]int, n)
	if n == 0 {
		return widths
	}
	if fixedWidths == nil {
		return urwidColumnWidths(maxcol, weights, dividechars)
	}
	hasFixed := false
	for _, fw := range fixedWidths {
		if fw > 0 {
			hasFixed = true
			break
		}
	}
	if !hasFixed {
		return urwidColumnWidths(maxcol, weights, dividechars)
	}

	shared := maxcol
	for i := 0; i < n; i++ {
		if i > 0 {
			shared -= dividechars
		}
	}

	var weightedIdxs []int
	wtotal := 0
	for i := 0; i < n; i++ {
		if fixedWidths[i] > 0 {
			fw := fixedWidths[i]
			if fw > shared {
				fw = shared
			}
			widths[i] = fw
			shared -= fw
		} else {
			weightedIdxs = append(weightedIdxs, i)
			wtotal += weights[i]
		}
	}

	if len(weightedIdxs) > 0 && shared > 0 {
		grow := shared
		for _, idx := range weightedIdxs {
			if wtotal <= 0 {
				widths[idx] = 1
				continue
			}
			w := weights[idx]
			width := int(float64(grow)*float64(w)/float64(wtotal) + 0.5)
			if width < 1 {
				width = 1
			}
			widths[idx] = width
			grow -= width
			wtotal -= w
		}
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
	fixedWidths []int
	dividechars int
	focusIndex  int
	// selfManaging[i] marks column i as owning its own Left/Right key handling.
	// For a self-managing focused column, Left/Right are forwarded to the
	// column's subtree instead of the parent moving column focus — so (a) the
	// Network browser pane's part-cursor model handles Left/Right (and its own
	// Left-at-start focus release) rather than the outer Columns pane-wrapping,
	// and (b) when the left column holds an AnnounceInfo button row (marked
	// self-managing while the detail view is open), Right moves Back→Connect
	// inside the nested button row instead of jumping to the browser. This
	// mirrors Python urwid Columns.keypress, which forwards the key to the
	// focused column first and only moves column focus on an *unhandled* key —
	// tview InputHandlers can't report consumption, so the self-managing flag
	// marks the columns that would consume Left/Right. Tab/Backtab and all other
	// keys keep their usual (moveFocus / forward) behavior either way.
	selfManaging []bool
}

// newURWIDColumns builds a urwid-style Columns row of the given weighted
// children with blank columns of width dividechars between them.
func newURWIDColumns(dividechars int, children ...tview.Primitive) *urwidColumns {
	weights := make([]int, len(children))
	fixedWidths := make([]int, len(children))
	for i := range children {
		weights[i] = 1
	}
	return &urwidColumns{
		Box:         tview.NewBox(),
		children:    children,
		weights:     weights,
		fixedWidths: fixedWidths,
		dividechars: dividechars,
		focusIndex:  -1,
	}
}

// SetSelfManaging marks column i as owning its Left/Right key handling (see
// urwidColumns.selfManaging). A self-managing focused column receives Left/
// Right via forwarding instead of the parent moving column focus.
func (c *urwidColumns) SetSelfManaging(i int, v bool) *urwidColumns {
	if i >= 0 && i < len(c.children) {
		if c.selfManaging == nil {
			c.selfManaging = make([]bool, len(c.children))
		}
		c.selfManaging[i] = v
	}
	return c
}

// SetFixedWidth sets an explicit fixed character width for column i.
func (c *urwidColumns) SetFixedWidth(i, width int) *urwidColumns {
	if i >= 0 && i < len(c.fixedWidths) {
		c.fixedWidths[i] = width
	}
	return c
}

// FixedWidth returns the explicit fixed character width of column i, or 0 when
// the column is weighted (no fixed width set). It is the read counterpart to
// SetFixedWidth, used by the Network page's fullscreen toggle to save/restore
// the left list pane width.
func (c *urwidColumns) FixedWidth(i int) int {
	if i >= 0 && i < len(c.fixedWidths) {
		return c.fixedWidths[i]
	}
	return 0
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
	widths := urwidColumnWidthsEx(w, c.weights, c.fixedWidths, c.dividechars)
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
	widths := urwidColumnWidthsEx(w, c.weights, c.fixedWidths, c.dividechars)
	max := 1
	for i, child := range c.children {
		cw := widths[i]
		hh := 1
		if hr, ok := child.(interface{ RequiredHeight(int) int }); ok {
			hh = hr.RequiredHeight(cw)
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
	// Route through WrapInputHandler so an input capture installed with
	// SetInputCapture (e.g. NetworkDisplay.handleInput for Ctrl-L toggle) is
	// consulted before the default column/child handling — matching tview's
	// own container primitives (Flex, Grid). Without this, SetInputCapture is
	// silently ignored and page-level shortcuts like Ctrl-L never fire.
	return c.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
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

		// A self-managing focused column owns Left/Right: forward those to its
		// subtree (e.g. the browser part-cursor model, or a nested AnnounceInfo
		// button row) instead of moving column focus. This mirrors Python
		// Columns.keypress forwarding to the focused column first; tview can't
		// report consumption, so the flag marks the columns that would consume
		// Left/Right. Tab/Backtab still move column focus (see the switch below).
		if c.focusIndex < len(c.selfManaging) && c.selfManaging[c.focusIndex] {
			if event.Key() == tcell.KeyRight || event.Key() == tcell.KeyLeft {
				if h := focusedChild.InputHandler(); h != nil {
					h(event, setFocus)
				}
				return
			}
		}

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
	})
}

func (c *urwidColumns) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		mx, my := event.Position()
		for i, child := range c.children {
			if !isFocusable(child) {
				continue
			}
			cx, cy, cw, ch := child.GetRect()
			if mx >= cx && mx < cx+cw && my >= cy && my < cy+ch {
				c.SetFocusIndex(i)
				if setFocus != nil {
					setFocus(child)
				}
				if h := child.MouseHandler(); h != nil {
					if consumed, capture := h(action, event, setFocus); consumed {
						return consumed, capture
					}
				}
				return true, nil
			}
		}
		return false, nil
	}
}
