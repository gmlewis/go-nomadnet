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
	"github.com/gmlewis/tview"
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
	items        []pfItem
	selectable   []int // indices into items[] of selectable items, in order
	focusIndex   int   // index into selectable[] of the focused item, -1 if none
	visible      []bool
	onEsc        func() // dismiss handler, matching urwid Pile/WidgetWrap Esc
	onUpEscape   func() // Up-at-top escape: hand focus to the parent (e.g. menu)
	onDownEscape func() // Down-at-bottom escape: hand focus to the parent
}

type pfItem struct {
	widget tview.Primitive
	height int // natural height in rows
	// heightFn, when non-nil, derives the item's height from the layout width,
	// the way urwid measures a flow widget's natural rows at the width its
	// container renders it at (see AddItemFn). It takes precedence over height.
	heightFn   func(width int) int
	selectable bool
	// listFn, when non-nil, reports the *tview.List this item scrolls. The pile
	// consults it to implement urwid's list-boundary handoff for an item that
	// WRAPS its scrollable list (the bordered list-slot Flex around the Saved
	// Nodes / Announce Stream / Peers content): urwid's ListBox returns the key
	// unhandled at its last/first row, so the enclosing Pile moves
	// focus_position. Items that are themselves a *tview.List or
	// *IndicativeListBox need no provider (see pileFiller.itemList).
	listFn func() *tview.List
	// nested marks an item that owns Up/Down focus inside its own subtree (a
	// nested pile, e.g. the Local Peer panel). The pile forwards Up/Down to it
	// and never moves its own focus, so the nested pile's escape handlers drive
	// the hand-off in both directions (urwid: the child Pile handles the key
	// and only returns it unhandled at its own ends).
	nested bool
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
	return p.appendItem(pfItem{widget: widget, height: height, selectable: selectable})
}

// AddItemFn appends an item whose height is computed at layout time from the
// width the pile is rendered at, which is how urwid gives a flow widget its
// natural (wrapped) rows: a Pile's PACK height follows the wrapped row count of
// each child, so the same urwid.Text is one row in a wide pane and two in a
// narrow one. A fixed height cannot express that — it would either clip the
// wrapped rows or reserve a blank row where the text fits on one line. The
// item is otherwise a fixed-height item: it is not stretched to fill spare
// height (AddItem with height 0 is).
//
// heightFn must return at least 0 (a negative height is clamped to 0, which
// makes the item flexible) and is called with the pile's current inner width.
func (p *pileFiller) AddItemFn(widget tview.Primitive, heightFn func(width int) int, selectable bool) *pileFiller {
	return p.appendItem(pfItem{widget: widget, heightFn: heightFn, selectable: selectable})
}

// appendItem adds a prepared item and registers it for focus when selectable.
// It returns the pile for chaining.
func (p *pileFiller) appendItem(item pfItem) *pileFiller {
	idx := len(p.items)
	p.items = append(p.items, item)
	p.visible = append(p.visible, false)
	if item.selectable {
		p.selectable = append(p.selectable, idx)
		if p.focusIndex < 0 {
			p.focusIndex = 0
		}
	}
	return p
}

// fixedHeight reports the item's height at the given layout width when that
// height is known independently of the box height: an explicit height, or a
// width-derived one. Items without either are the flexible ones (urwid WEIGHT),
// which share whatever height is left over.
func fixedHeight(it pfItem, width int) (int, bool) {
	if it.heightFn != nil {
		return max(it.heightFn(width), 0), true
	}
	if it.height > 0 {
		return it.height, true
	}
	return 0, false
}

// FocusIndex returns the index (into selectable[]) of the focused selectable
// item, or -1 if there are no selectable items.
func (p *pileFiller) FocusIndex() int { return p.focusIndex }

// GetItemCount returns the number of pile items, the read counterpart of
// urwid's len(Pile.contents).
func (p *pileFiller) GetItemCount() int { return len(p.items) }

// GetItem returns the widget held by pile item i, or nil when i is out of
// range. It is the read counterpart of urwid's Pile.contents[i][0] and lets
// callers (and the left-pane invariant tests) address the pile by index the way
// the Python original addresses left_pile.contents.
func (p *pileFiller) GetItem(i int) tview.Primitive {
	if i < 0 || i >= len(p.items) {
		return nil
	}
	return p.items[i].widget
}

// Clear removes every item, resetting the pile to empty (urwid Pile.contents =
// []). State that is rebuilt from scratch (the Network left pane's
// rebuildLeftPanel) clears first so item order can never drift.
func (p *pileFiller) Clear() {
	p.items = nil
	p.selectable = nil
	p.visible = nil
	p.focusIndex = -1
}

// SetItemList installs a list provider for item i, so the pile can apply
// urwid's list-boundary handoff to an item that wraps its scrollable list (see
// pfItem.listFn). It is a no-op for an out-of-range index.
func (p *pileFiller) SetItemList(i int, fn func() *tview.List) {
	if i >= 0 && i < len(p.items) {
		p.items[i].listFn = fn
	}
}

// SetItemNested marks item i as owning Up/Down inside its own subtree (see
// pfItem.nested). It is a no-op for an out-of-range index.
func (p *pileFiller) SetItemNested(i int, nested bool) {
	if i >= 0 && i < len(p.items) {
		p.items[i].nested = nested
	}
}

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

// SetUpEscapeHandler installs a handler invoked when Up is pressed while the
// pile's TOP selectable item has focus, so focus can escape the pile to the
// parent (e.g. the main menu) instead of dead-ending on the top widget. This
// mirrors urwid's MainFrame, where Up at the top of the body moves focus to the
// header. Without it the Announce Stream's tab bar (the top pile item) traps Up.
func (p *pileFiller) SetUpEscapeHandler(fn func()) { p.onUpEscape = fn }

// SetDownEscapeHandler installs a handler invoked when Down is pressed at the
// BOTTOM of the pile's last selectable item, so focus can escape the pile to
// the next outer pile item instead of dead-ending. It is the mirror of
// SetUpEscapeHandler and is what lets a NESTED pile (the Announce Stream's
// tab/filter/list pile) hand focus out to the Network left pile's next item —
// urwid: the inner Pile's Down candidates run off the end, the key returns
// unhandled, and the enclosing Pile moves its focus_position.
func (p *pileFiller) SetDownEscapeHandler(fn func()) { p.onDownEscape = fn }

// itemList resolves the *tview.List whose up/down boundary should hand focus to
// the neighbouring pile item: the explicit provider when one is installed (an
// item that wraps its list, e.g. the bordered list-slot Flex), otherwise a
// bare *tview.List or the List inside an *IndicativeListBox. It returns nil for
// every other item (a urwidColumns bar, a nested pile, a button row), which
// keeps its own focus handling.
func (p *pileFiller) itemList(item pfItem) *tview.List {
	if item.listFn != nil {
		return item.listFn()
	}
	switch v := item.widget.(type) {
	case *tview.List:
		return v
	case *IndicativeListBox:
		return v.List
	}
	return nil
}

// forward hands the event to the focused item's own InputHandler (urwid
// Pile.keypress's `self.focus.keypress(size, key)` step).
func (p *pileFiller) forward(fi tview.Primitive, event *tcell.EventKey, setFocus func(tview.Primitive)) {
	if fi == nil {
		return
	}
	if h := fi.InputHandler(); h != nil {
		h(event, setFocus)
	}
}

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
		if h, ok := fixedHeight(it, iw); ok {
			fixedTotal += h
		} else {
			flexibleCount++
		}
	}

	flexHeight := 0
	if flexibleCount > 0 && ih > fixedTotal {
		flexHeight = (ih - fixedTotal) / flexibleCount
	}

	// Resolve every item's height once: fixed items keep their own (possibly
	// width-derived) height, flexible items share the leftover equally.
	heights := make([]int, len(p.items))
	for i, it := range p.items {
		if h, ok := fixedHeight(it, iw); ok {
			heights[i] = h
		} else {
			heights[i] = flexHeight
		}
	}

	total := 0
	for _, h := range heights {
		total += h
	}

	// Compute the top-trim in rows (urwid Filler.render cursor-trim), based on
	// the currently focused selectable item's position.
	topTrim := 0
	if total > ih {
		if fi := p.focusedItem(); fi != nil {
			focusedTop := 0
			for i, it := range p.items {
				if it.widget == fi {
					break
				}
				focusedTop += heights[i]
			}
			topTrim = max(focusedTop-ih+1, 0)
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
		itemH := heights[i]
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
//
// When no selectable child exists (focusedItem returns nil), the fallback
// calls Box.Focus(delegate) so the pile's own hasFocus flag is set and
// HasFocus() returns true. Without this, tview's event loop gate at
// application.go:439 (root.HasFocus()) silently drops all key events. This
// mirrors tview.Flex.Focus, which has the same Box.Focus fallback.
func (p *pileFiller) Focus(delegate func(tview.Primitive)) {
	if fi := p.focusedItem(); fi != nil {
		fi.Focus(delegate)
		return
	}
	p.Box.Focus(delegate)
}

// blurTree clears the focus flag on w and on every descendant that can hold it.
// Without the descent a wrapper stops the blur one level short: tview's Flex
// (and Box) carry no cascading Blur — Flex.Blur is only the promoted Box flag —
// so a leaf nested inside a Flex keeps hasFocus=true after the container is
// blurred. Two panes then report focus at once and the next Down/Up dispatches
// to the wrong one; focus must be exclusive.
func blurTree(w tview.Primitive) {
	if w == nil {
		return
	}
	switch v := w.(type) {
	case *tview.Flex:
		for i := range v.GetItemCount() {
			blurTree(v.GetItem(i))
		}
	case *pileFiller:
		v.Blur()
		return
	case *urwidColumns:
		for _, child := range v.children {
			blurTree(child)
		}
	case *IndicativeListBox:
		blurTree(v.List)
	case *IndicativeMessages:
		blurTree(v.Text)
	}
	if bl, ok := w.(interface{ Blur() }); ok {
		bl.Blur()
	}
}

// Blur clears focus on the pile AND on everything under its items. The
// inherited tview.Box.Blur only clears the pile's own hasFocus flag, leaving
// the focused child (e.g. the Announce-Stream list, or a Flex-wrapped leaf)
// with hasFocus=true. When app.SetFocus moves focus away from the pile — which
// it does by calling a.focus.Blur() — a.focus is this pileFiller (a container):
// the Announce Stream pile becomes a.focus because pileFiller.moveFocus
// registers the pile itself with the app via setFocus(p), and tview's leaf
// Focus (Box.Focus) never calls the delegate, so the chain stops there. Without
// this override the focused child keeps hasFocus=true after focus moves to the
// browser, so BOTH columns report focus and subsequent Downs dispatch to the
// left list instead of scrolling the browser. Blurring the items' whole
// subtrees releases the left column cleanly. This is safe with moveFocus:
// during setFocus(p) the new child has not been focused yet, so this Blur is a
// no-op on it, and the subsequent pile.Focus -> fi.Focus re-focuses it.
func (p *pileFiller) Blur() {
	p.Box.Blur()
	for _, it := range p.items {
		blurTree(it.widget)
	}
}

func (p *pileFiller) HasFocus() bool {
	for _, it := range p.items {
		if it.widget.HasFocus() {
			return true
		}
	}
	return p.Box.HasFocus()
}

// syncFocusIndex reconciles focusIndex with the actual hasFocus flags so key
// routing matches Python's Pile.focus_position (which is authoritative —
// urwid routes keys to focus_position regardless of leaf focus):
//
//  1. the item at focusIndex has focus → nothing to do;
//  2. a DIFFERENT selectable item gained focus (e.g. a mouse click on the
//     Search field) → adopt it;
//  3. nothing has focus (a.focus is this pile itself — the common state after
//     App.SetFocus(pile) early-returns on the already-focused pile while its
//     children are blurred) → re-focus the item at focusIndex. Without this
//     branch Enter on the announce list would be routed to whichever stale
//     child still held hasFocus (the tab bar) and swallowed.
func (p *pileFiller) syncFocusIndex() {
	if p.focusIndex >= 0 && p.focusIndex < len(p.selectable) &&
		p.items[p.selectable[p.focusIndex]].widget.HasFocus() {
		return
	}
	for idx, selIdx := range p.selectable {
		if p.items[selIdx].widget.HasFocus() {
			p.focusIndex = idx
			return
		}
	}
	if p.focusIndex >= 0 && p.focusIndex < len(p.selectable) {
		if fi := p.focusedItem(); fi != nil {
			fi.Focus(func(tview.Primitive) {})
		}
	}
}

// visibleCount reports how many of the pile's items are currently visible, for
// the mouse-wheel debug trace.
func (p *pileFiller) visibleCount() int {
	n := 0
	for _, v := range p.visible {
		if v {
			n++
		}
	}
	return n
}

// InputHandler implements urwid-Pile-style focus traversal. A focused LIST
// item (or an item wrapping one) keeps Up/Down while its cursor can move and
// hands focus to the neighbouring pile item at its boundary; a NESTED item (a
// pile inside this pile) owns Up/Down and Tab/Backtab inside its own subtree;
// every other key is forwarded to the focused item's own handler, with Tab/Down
// moving focus forward (wrapping) and BackTab/Up backward.
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
		item := p.items[p.selectable[p.focusIndex]]

		// A list item keeps Up/Down while its cursor can move; at the list's
		// boundary the pile takes over (urwid Pile.keypress: the ListBox returns
		// the key unhandled at its first/last row, so the Pile walks its own
		// focus_position — and at the pile's own ends the key escapes outward).
		if list := p.itemList(item); list != nil {
			curr := list.GetCurrentItem()
			count := list.GetItemCount()
			switch event.Key() {
			case tcell.KeyUp:
				if curr > 0 {
					p.forward(fi, event, setFocus)
					return
				}
				if p.focusIndex > 0 {
					p.moveFocus(-1, setFocus)
					return
				}
				if p.onUpEscape != nil {
					p.onUpEscape()
				}
				return
			case tcell.KeyDown:
				if curr < count-1 {
					p.forward(fi, event, setFocus)
					return
				}
				if p.focusIndex < len(p.selectable)-1 {
					p.moveFocus(1, setFocus)
					return
				}
				if p.onDownEscape != nil {
					p.onDownEscape()
				}
				return
			}
		}

		// A nested item (a pile inside this pile) owns Up/Down AND Tab/Backtab
		// inside its own subtree; it hands focus back out through its own escape
		// handlers, so the pile never moves its own focus while that item is
		// focused. Without forwarding Tab the outer pile would steal it and the
		// nested pile's own Tab traversal (tab bar → filter bar → list) would be
		// unreachable.
		if item.nested {
			switch event.Key() {
			case tcell.KeyUp, tcell.KeyDown, tcell.KeyTab, tcell.KeyBacktab:
				p.forward(fi, event, setFocus)
				return
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
			// At the top of the pile: hand focus to the parent (e.g. the
			// main menu) when wired, matching urwid's MainFrame where Up at
			// the top of the body moves focus to the header. Without this
			// the top widget (e.g. the Announce Stream tab bar) traps Up.
			if p.onUpEscape != nil {
				p.onUpEscape()
				return
			}
		}

		p.forward(fi, event, setFocus)
	}
}

func (p *pileFiller) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		isWheel := action == tview.MouseScrollUp || action == tview.MouseScrollDown
		if mouseDebug != nil && isWheel {
			x, y := event.Position()
			dbgMouse("pileFiller wheel %v at (%v,%v): %v visible items", action, x, y, p.visibleCount())
		}
		for i, it := range p.items {
			if !p.visible[i] {
				continue
			}
			if h := it.widget.MouseHandler(); h != nil {
				consumed, capture := h(action, event, setFocus)
				if mouseDebug != nil && isWheel {
					dbgMouse("  pileFiller item[%v] consumed=%v", i, consumed)
				}
				if consumed {
					return consumed, capture
				}
			}
		}
		return false, nil
	}
}
