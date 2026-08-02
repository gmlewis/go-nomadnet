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

// pileFiller replicates urwid.Filler(body, valign=TOP, height=PACK) wrapping a
// urwid.Pile of fixed-height flow widgets. It renders the pile at its natural
// height at the top of the available box. When the content overflows the box it
// trims the TOP so the focused (selectable) item stays visible at the bottom —
// exactly urwid's Filler.render cursor-trim (urwid/widget/filler.py:228-238):
//
//	if canv.rows() > maxrow and canv.cursor is not None:
//	    cy = canv.cursor[1]
//	    if cy >= maxrow:
//	        canv.trim(cy - maxrow + 1, maxrow)
//
// i.e. the cursor row is forced into the last visible row by trimming rows off
// the top. With no selectable item (no cursor) it clips the bottom instead
// (urwid's trim(0, maxrow)), keeping the top rows.
//
// This is what makes Python's AnnounceInfo (Network.py:59-256) fit the bordered
// left-pane slot at 80x24: the node Pile is 11 rows but the slot is 9 rows
// inner, so urwid trims the top 2 (Time + Addr) to keep the focused button row
// visible at the bottom. tview's Flex does not clip overflow — it lets children
// spill past their allocation and corrupt the layout below — so a dedicated
// primitive is required for parity.
//
// Limitation: a partial top-trim that splits a multi-row item shows that item's
// TOP rows rather than its bottom rows (tview primitives render top-first at a
// reduced height; urwid's canvas trim removes the top). This only occurs at
// extreme short pane heights (the AnnounceInfo multi-row item is the 2-row
// "Announce Data" block, which a partial trim reaches only when the slot inner
// height drops to ~3 rows); at the 80x24 parity target the trim is always a
// whole number of 1-row header items.
type pileFiller struct {
	*tview.Box
	items      []pfItem
	selectable []int // indices into items[] of selectable items, in order
	focusIndex int   // index into selectable[] of the focused item, -1 if none
	visible    []bool
	onEsc      func() // dismiss handler, matching urwid Pile/WidgetWrap Esc
}

type pfItem struct {
	widget     tview.Primitive
	height     int // natural height in rows
	selectable bool
}

// newPileFiller builds an empty top-filled pile. Add items with AddItem; the
// first selectable item becomes the focus (matching urwid.Pile, whose
// focus_position defaults to the first selectable widget). Override with
// SetFocusIndex (e.g. KnownNodeInfo sets focus to the last item — the button
// row — via pile.focus_position = len-1, Network.py:803).
func newPileFiller() *pileFiller {
	return &pileFiller{Box: tview.NewBox(), focusIndex: -1}
}

// AddItem appends a fixed-height item. selectable items participate in focus;
// the first one added becomes the pile's focus.
func (p *pileFiller) AddItem(widget tview.Primitive, height int, selectable bool) *pileFiller {
	if height < 0 {
		height = 0
	}
	idx := len(p.items)
	p.items = append(p.items, pfItem{widget: widget, height: height, selectable: selectable})
	p.visible = append(p.visible, false)
	if selectable {
		p.selectable = append(p.selectable, idx)
		if p.focusIndex < 0 {
			p.focusIndex = 0
		}
	}
	return p
}

// FocusIndex returns the index (into selectable[]) of the focused selectable
// item, or -1 if there are no selectable items.
func (p *pileFiller) FocusIndex() int { return p.focusIndex }

// SetFocusIndex sets the focused selectable item by its index into
// selectable[], clamped to the valid range. Use this to override the
// first-selectable default (e.g. KnownNodeInfo focuses the button row).
func (p *pileFiller) SetFocusIndex(i int) {
	if len(p.selectable) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(p.selectable) {
		i = len(p.selectable) - 1
	}
	p.focusIndex = i
}

// focusedItem returns the currently focused selectable item, or nil.
func (p *pileFiller) focusedItem() tview.Primitive {
	if p.focusIndex < 0 || p.focusIndex >= len(p.selectable) {
		return nil
	}
	return p.items[p.selectable[p.focusIndex]].widget
}

// SetEscHandler installs a dismiss handler invoked when Esc is pressed, matching
// Python's AnnounceInfo/KnownNodeInfo keypress intercepting "esc" before the
// pile forwards it (Network.py:60-65, 595-598). The page-level InputCapture
// forwards Esc unconsumed, so without this Esc would be lost when focus is on
// the pile's widgets.
func (p *pileFiller) SetEscHandler(fn func()) { p.onEsc = fn }

// moveFocus moves focus by delta (wrapping) among the selectable items,
// blurring the old widget and focusing the new so cursor/highlight rendering
// tracks the change (urwid Pile.keypress Tab/Up/Down focus movement).
func (p *pileFiller) moveFocus(delta int, setFocus func(tview.Primitive)) {
	n := len(p.selectable)
	if n == 0 {
		return
	}
	if old := p.focusedItem(); old != nil {
		if bl, ok := old.(interface{ Blur() }); ok {
			bl.Blur()
		}
	}
	if p.focusIndex < 0 {
		p.focusIndex = 0
	}
	p.focusIndex = (p.focusIndex + delta%n + n) % n
	if nw := p.focusedItem(); nw != nil {
		nw.Focus(func(tview.Primitive) {})
	}
	if setFocus != nil {
		setFocus(p)
	}
}

// SetRect lays out the children within the box, applying urwid's top-trim when
// the natural content height overflows the available height.
func (p *pileFiller) SetRect(x, y, w, h int) {
	p.Box.SetRect(x, y, w, h)
	// The filler itself has no border/padding, so the inner rect is the box.
	ix, iy, iw, ih := x, y, w, h

	for i := range p.visible {
		p.visible[i] = false
	}

	if iw <= 0 || ih <= 0 {
		for _, it := range p.items {
			it.widget.SetRect(ix, iy, 0, 0)
		}
		return
	}

	fixedTotal := 0
	flexibleCount := 0
	for _, it := range p.items {
		if it.height > 0 {
			fixedTotal += it.height
		} else {
			flexibleCount++
		}
	}

	flexHeight := 0
	if flexibleCount > 0 && ih > fixedTotal {
		flexHeight = (ih - fixedTotal) / flexibleCount
	}

	total := fixedTotal + flexHeight*flexibleCount

	// Compute the top-trim in rows (urwid Filler.render cursor-trim), based on
	// the currently focused selectable item's position.
	topTrim := 0
	if total > ih {
		if fi := p.focusedItem(); fi != nil {
			focusedTop := 0
			for _, it := range p.items {
				if it.widget == fi {
					break
				}
				itemH := it.height
				if itemH == 0 {
					itemH = flexHeight
				}
				focusedTop += itemH
			}
			topTrim = focusedTop - ih + 1
			if topTrim < 0 {
				topTrim = 0
			}
			if max := total - ih; topTrim > max {
				topTrim = max
			}
		}
		// No selectable item: urwid trims(0, maxrow) → keep the top rows.
	}

	// Lay out children within the visible window [iy, iy+ih). The virtual
	// content top sits at iy - topTrim; items fully outside the window are
	// skipped (not drawn), items straddling the top are drawn at a reduced
	// height from the box top.
	cy := iy - topTrim
	for i, it := range p.items {
		itemH := it.height
		if itemH == 0 {
			itemH = flexHeight
		}
		itemTop := cy
		itemBottom := cy + itemH
		cy = itemBottom

		if itemBottom <= iy || itemTop >= iy+ih {
			// Fully clipped (above the window or below it) — mark invisible.
			continue
		}
		drawY := itemTop
		drawH := itemH
		if drawY < iy {
			drawH = itemBottom - iy
			drawY = iy
		}
		if drawY+drawH > iy+ih {
			drawH = iy + ih - drawY
		}
		if drawH > 0 {
			it.widget.SetRect(ix, drawY, iw, drawH)
			p.visible[i] = true
		}
	}
}

// Draw renders the visible items.
func (p *pileFiller) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	for i, it := range p.items {
		if p.visible[i] {
			it.widget.Draw(screen)
		}
	}
}

// Focus, HasFocus, InputHandler and MouseHandler delegate to the focused
// selectable item so the pile behaves like a single selectable group (urwid
// Pile forwards focus to focus_position).
func (p *pileFiller) Focus(delegate func(tview.Primitive)) {
	if fi := p.focusedItem(); fi != nil {
		fi.Focus(delegate)
	}
}

func (p *pileFiller) HasFocus() bool {
	for _, it := range p.items {
		if it.widget.HasFocus() {
			return true
		}
	}
	return false
}

func (p *pileFiller) syncFocusIndex() {
	for idx, selIdx := range p.selectable {
		if p.items[selIdx].widget.HasFocus() {
			p.focusIndex = idx
			return
		}
	}
}

// InputHandler implements urwid-Pile-style focus traversal: Tab/Down moves
// focus to the next selectable item, BackTab/Up to the previous (wrapping),
// and all other keys are forwarded to the focused item's own handler.
func (p *pileFiller) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if len(p.selectable) == 0 {
			return
		}
		if event.Key() == tcell.KeyEscape && p.onEsc != nil {
			p.onEsc()
			return
		}

		p.syncFocusIndex()
		fi := p.focusedItem()

		// If focused item is a List or IndicativeListBox, allow Up/Down to scroll items in the list
		// unless at the list boundaries (index 0 for Up, last index for Down).
		if fi != nil {
			var list *tview.List
			if l, ok := fi.(*tview.List); ok {
				list = l
			} else if ilb, ok := fi.(*IndicativeListBox); ok {
				list = ilb.List
			}

			if list != nil {
				curr := list.GetCurrentItem()
				count := list.GetItemCount()
				switch event.Key() {
				case tcell.KeyUp:
					if curr > 0 {
						if h := fi.InputHandler(); h != nil {
							h(event, setFocus)
						}
						return
					}
					// At top of list: move focus up to previous item in pile
					p.moveFocus(-1, setFocus)
					return
				case tcell.KeyDown:
					if curr < count-1 {
						if h := fi.InputHandler(); h != nil {
							h(event, setFocus)
						}
						return
					}
					// At bottom of list: move focus down to next item in pile
					p.moveFocus(1, setFocus)
					return
				}
			}
		}

		switch event.Key() {
		case tcell.KeyTab:
			p.moveFocus(1, setFocus)
			return
		case tcell.KeyBacktab:
			p.moveFocus(-1, setFocus)
			return
		case tcell.KeyDown:
			if p.focusIndex < len(p.selectable)-1 {
				p.moveFocus(1, setFocus)
				return
			}
		case tcell.KeyUp:
			if p.focusIndex > 0 {
				p.moveFocus(-1, setFocus)
				return
			}
		}

		if fi != nil {
			if h := fi.InputHandler(); h != nil {
				h(event, setFocus)
			}
		}
	}
}

func (p *pileFiller) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		for i, it := range p.items {
			if !p.visible[i] {
				continue
			}
			if h := it.widget.MouseHandler(); h != nil {
				if consumed, capture := h(action, event, setFocus); consumed {
					return consumed, capture
				}
			}
		}
		return false, nil
	}
}
