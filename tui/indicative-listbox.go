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
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// mouseWheelLines is how many list rows one mouse-wheel notch moves the
// highlight. The wheel is translated into a SetCurrentItem jump (arrow-key
// semantics) instead of tview's default itemOffset-only viewport move, which the
// Draw keep-current-visible clamp cancels at the viewport edges — the "wheel
// stuck at the highlight" bug on the Network Announce Stream / Saved Nodes
// lists. Configurable per launch via GONOMADNET_WHEEL_LINES; tests
// override it via SetMouseWheelLines. This is a set-once-at-startup config var,
// the same pattern as the mouseDebug logger (tui/mouse-debug.go).
var mouseWheelLines = 8

func init() {
	if v := strings.TrimSpace(os.Getenv("GONOMADNET_WHEEL_LINES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			mouseWheelLines = n
		}
	}
}

// SetMouseWheelLines sets the rows-per-wheel-notch multiplier. It is a test
// hook (production reads GONOMADNET_WHEEL_LINES once at startup); tests use it
// to exercise specific multipliers deterministically.
func SetMouseWheelLines(n int) {
	if n >= 1 {
		mouseWheelLines = n
	}
}

// IndicativeListBox wraps a *tview.List with centered top and bottom indicator
// bars, mirroring nomadnet's IndicativeListBox
// (vendor/additional_urwid_widgets/widgets/indicative_listbox.py): the top bar
// shows "───" when the first item is visible (the top end is exposed) and "▲"
// when items are hidden above it; the bottom bar shows "───" when the last item
// is visible and "▼" when items are hidden below it. Both bars are centered.
//
// The wrapped List owns all item rendering, selection and scrolling; this
// primitive only reserves one row above and one below for the indicators and
// draws them after the List (so the List's item offset is current). Focus,
// input and mouse are delegated to the wrapped List so the application can
// focus this primitive exactly as it would a bare *tview.List.
type IndicativeListBox struct {
	*tview.Box
	List *tview.List
	// skipUnselectable, when set, marks rows the Up/Down cursor jumps over
	// (Python's IndicativeListBox walks only selectable widgets; the Channels
	// spacer is a plain urwid.Text that urwid's ListBox skips).
	skipUnselectable func(idx int) bool
	emptyText        string          // centered placeholder drawn when the List has no items
	emptyWidget      tview.Primitive // optional widget drawn in the list area when empty
	// emptyFG/emptyBG, when set, style the emptyText placeholder row. Python
	// paints the Conversations empty-state placeholder with the list_off_focus
	// palette across the full inner width in EVERY focus state (live captures
	// pyaconv_100x28_00/01/02 row 4: fg 0,0,0 bg 135,135,135 — the vendor ILB's
	// off-focus attribute sticks to the placeholder item), so the style is
	// applied unconditionally, not only while focused.
	emptyFG tcell.Color
	emptyBG tcell.Color

	// Selection styles for when the list itself loses focus. Python's ILB
	// highlight_offFocus repaints the SELECTED row with the off-focus palette
	// whenever the widget does not have the focus (vendor indicator
	// indicative_listbox.py:172-182: set_attr_map(highlight_offFocus) at focus
	// transitions), so the highlight stays visible against unfocused entry
	// colors. offFocusSelSet is false until SetHighlightStyles opts in —
	// highlight_offFocus=None (the default) highlights nothing off-focus.
	selFG, selBG   tcell.Color // focused (list_focus)
	offFG, offBG   tcell.Color // unfocused (list_off_focus)
	offFocusSelSet bool
}

// NewIndicativeListBox wraps the given List with indicator bars. If list is
// nil a new (empty) List is created. The caller may continue to configure the
// List (AddItem, SetSelectedFunc, ShowSecondaryText, …) via the List field.
func NewIndicativeListBox(list *tview.List) *IndicativeListBox {
	if list == nil {
		list = tview.NewList()
	}
	ilb := &IndicativeListBox{Box: tview.NewBox(), List: list}
	ilb.List.SetInputCapture(ilb.skipUnselectableCapture)
	return ilb
}

// SetEmptyText sets the centered placeholder drawn in the list area when the
// wrapped List has no items, mirroring nomadnet's empty list body
// `[urwid.Text(empty_label, align='center')]` (Conversations.py:496). Without a
// real item the indicator bars still render "───" above and below it.
func (i *IndicativeListBox) SetEmptyText(text string) { i.emptyText = text }

// SetEmptyStyle sets the foreground/background painted on the emptyText
// placeholder row (list_off_focus for the Conversations empty state). Has no
// effect on the SetEmptyWidget path, whose primitive carries its own styling.
func (i *IndicativeListBox) SetEmptyStyle(fg, bg tcell.Color) {
	i.emptyFG, i.emptyBG = fg, bg
}

// SetHighlightStyles wires the on-focus (list_focus) and off-focus
// (list_off_focus) selection palettes. Before each draw the selected row's
// colors are swapped for the current focus state, mirroring the vendor ILB's
// set_attr_map(highlight_offFocus) / restore at focus transitions (vendor
// indicative_listbox.py:172-182).
func (i *IndicativeListBox) SetHighlightStyles(focusedFG, focusedBG, offFG, offBG tcell.Color) {
	i.selFG, i.selBG, i.offFG, i.offBG = focusedFG, focusedBG, offFG, offBG
	i.offFocusSelSet = true
}

// SetEmptyWidget sets a widget drawn in the list area (between the indicator
// bars) when the wrapped List has no items, mirroring nomadnet's IndicativeListBox
// which holds arbitrary widgets — e.g. the Channels "No hubs yet…" Text
// (Channels.py:1604) is a single Text widget as the only list entry. The widget
// is laid out to the list rect each draw. A non-empty List takes precedence.
func (i *IndicativeListBox) SetEmptyWidget(w tview.Primitive) { i.emptyWidget = w }

// SetRect lays out the wrapped List one row below the top and one row above the
// bottom (reserving space for the indicator bars) when there is room, otherwise
// the List fills the whole rect.
func (i *IndicativeListBox) SetRect(x, y, w, h int) {
	i.Box.SetRect(x, y, w, h)
	listX, listY, listW, listH := i.listRect()
	i.List.SetRect(listX, listY, listW, listH)
}

// listRect returns the rect to assign to the wrapped List (the full rect minus
// one row top and one row bottom when height >= 3).
// SetSkipUnselectable installs the row-rejection predicate used by the
// Up/Down cursor movement (nil disables the skipping).
func (i *IndicativeListBox) SetSkipUnselectable(fn func(idx int) bool) {
	i.skipUnselectable = fn
}

// nextSelectableFrom finds the nearest index to from (searching down for
// down=true, up otherwise) whose row the skip predicate accepts.
func (i *IndicativeListBox) nextSelectableFrom(from int, down bool) int {
	fn := i.skipUnselectable
	if fn == nil {
		return from
	}
	count := i.List.GetItemCount()
	if count == 0 {
		return from
	}
	cur := from
	for range count {
		if !fn(cur) {
			return cur
		}
		if down {
			cur++
			if cur >= count {
				return from
			}
		} else {
			cur--
			if cur < 0 {
				return from
			}
		}
	}
	return from
}

// skipUnselectableCapture intercepts Up/Down on the wrapped list and jumps
// the highlight over rows the skip predicate rejects (the spacer rows).
func (i *IndicativeListBox) skipUnselectableCapture(event *tcell.EventKey) *tcell.EventKey {
	if i.skipUnselectable == nil {
		return event
	}
	switch event.Key() {
	case tcell.KeyDown:
		want := i.List.GetCurrentItem() + 1
		if want > i.List.GetItemCount()-1 {
			// urwid ListBox: Down at the bottom bubbles unhandled and the
			// enclosing containers ignore it, so the highlight STAYS. tview's
			// List instead WRAPS to the first row, teleporting the selection
			// to the top — the live fleet bug where navigating to the last
			// hub row silently reset the cursor to the first hub.
			return nil
		}
		i.List.SetCurrentItem(i.nextSelectableFrom(want, true))
		return nil
	case tcell.KeyUp:
		want := i.List.GetCurrentItem() - 1
		if want < 0 {
			// urwid parity: no wrap at the top either.
			return nil
		}
		i.List.SetCurrentItem(i.nextSelectableFrom(want, false))
		return nil
	}
	return event
}

func (i *IndicativeListBox) listRect() (int, int, int, int) {
	x, y, w, h := i.GetRect()
	if h >= 3 {
		return x, y + 1, w, h - 2
	}
	return x, y, w, h
}

// Draw draws the wrapped List then the two indicator bars.
func (i *IndicativeListBox) Draw(screen tcell.Screen) {
	i.Box.DrawForSubclass(screen, i)
	x, y, w, h := i.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	// Python's vendor ILB swaps the selected row's palette on focus
	// transitions (indicative_listbox.py:172-182): focused → the entry's own
	// focus map (list_focus), unfocused → highlight_offFocus (list_off_focus).
	// Do it before the List draws so the selection renders in the right state.
	if i.offFocusSelSet {
		if i.HasFocus() {
			i.List.SetSelectedTextColor(i.selFG)
			i.List.SetSelectedBackgroundColor(i.selBG)
		} else {
			i.List.SetSelectedTextColor(i.offFG)
			i.List.SetSelectedBackgroundColor(i.offBG)
		}
	}
	i.List.Draw(screen)

	// When the list is empty, draw either the empty widget (a free-form
	// primitive, e.g. the Channels "No hubs yet…" Text) or the centered
	// placeholder text in the list area between the indicator bars.
	if i.List.GetItemCount() == 0 {
		lx, ly, lw, lh := i.listRect()
		if i.emptyWidget != nil {
			i.emptyWidget.SetRect(lx, ly, lw, lh)
			i.emptyWidget.Draw(screen)
		} else if i.emptyText != "" {
			if i.emptyBG != tcell.ColorDefault && i.emptyBG != 0 {
				// Python paints the placeholder row across the full inner
				// width (urwid AttrMap fill) — replicate before the text.
				fill := tcell.StyleDefault.Background(i.emptyBG)
				if i.emptyFG != tcell.ColorDefault && i.emptyFG != 0 {
					fill = fill.Foreground(i.emptyFG)
				}
				for x := lx; x < lx+lw; x++ {
					screen.SetContent(x, ly, ' ', nil, fill)
				}
				tview.Print(screen, i.emptyText, lx, ly, lw, tview.AlignCenter, i.emptyFG)
			} else {
				tview.Print(screen, i.emptyText, lx, ly, lw, tview.AlignCenter, tcell.ColorDefault)
			}
		}
	}

	top, bottom := i.indicators()
	// The bars are centered like urwid Text, which pads the LEFT side with
	// (w-len+1)//2 spaces — a ceil, not the floor tview.AlignCenter uses — so
	// the "▲" sits one column right of a naive centering on odd leftovers.
	// Draw them at the explicitly computed column with AlignLeft.
	centerBarX := func(s string) int { return x + max((w-utf8.RuneCountInString(s)+1)/2, 0) }
	if h >= 3 {
		tview.Print(screen, top, centerBarX(top), y, w, tview.AlignLeft, tcell.ColorDefault)
		tview.Print(screen, bottom, centerBarX(bottom), y+h-1, w, tview.AlignLeft, tcell.ColorDefault)
	} else if h == 2 {
		tview.Print(screen, top, centerBarX(top), y, w, tview.AlignLeft, tcell.ColorDefault)
	}

	if i.HasFocus() {
		count := i.List.GetItemCount()
		if count > 0 {
			current := i.List.GetCurrentItem()
			offset, _ := i.List.GetOffset()
			lx, ly, _, lh := i.listRect()
			// rowHeight is how many PHYSICAL rows one list item occupies: 2 when
			// secondary text is shown (main line + time/badge line — the
			// Conversations list), 1 otherwise (Guide / Saved Nodes lists).
			// urwid renders the hardware cursor on the FIRST row of the focused
			// entry (single widget per entry), so with two-line entries the
			// cursor offset must be scaled by the item height; the previous
			// item-index-as-row math put the cursor one conversation below the
			// highlight and drifted a row per arrow keypress.
			rowHeight := 1
			if i.List.GetShowSecondaryText() {
				rowHeight = 2
			}
			row := (current - offset) * rowHeight
			if row >= 0 && row < lh {
				screen.ShowCursor(lx, ly+row)
			}
		}
	}
}

// indicators returns the (top, bottom) bar strings for the List's current
// scroll position: "───" when the respective end is visible, "▲"/"▼" when items
// are hidden beyond it. An empty list exposes both ends.
func (i *IndicativeListBox) indicators() (top, bottom string) {
	itemOffset, _ := i.List.GetOffset()
	count := i.List.GetItemCount()
	_, _, _, listH := i.listRect()
	visible := max(listH, 1)

	top = "▲"
	if itemOffset <= 0 || count == 0 {
		top = "───"
	}

	bottom = "▼"
	if count == 0 {
		bottom = "───"
	} else if itemOffset+visible-1 >= count-1 {
		bottom = "───"
	}
	return top, bottom
}

// Focus forwards to the wrapped List so its focus highlight tracks correctly.
func (i *IndicativeListBox) Focus(delegate func(p tview.Primitive)) {
	i.Box.Focus(delegate)
	i.List.Focus(delegate)
}

func (i *IndicativeListBox) Blur() {
	i.Box.Blur()
	i.List.Blur()
}

func (i *IndicativeListBox) HasFocus() bool {
	return i.Box.HasFocus() || i.List.HasFocus()
}

// InputHandler delegates to the wrapped List, checking input capture first.
func (i *IndicativeListBox) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		capture := i.GetInputCapture()
		if capture != nil {
			event = capture(event)
			if event == nil {
				return
			}
		}
		if handler := i.List.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	}
}

// MouseHandler intercepts the mouse wheel before delegating the rest to the
// wrapped List. tview's List wheel handler moves itemOffset (the viewport)
// while arrow keys move currentItem (the highlight); the Draw keep-current-
// visible clamp then snaps itemOffset back when the highlight sits at a viewport
// edge, so the wheel "sticks" at the edges and arrow keys do not. Translating
// the wheel into a SetCurrentItem jump (arrow-key semantics) makes the highlight
// follow the wheel and the viewport follow the highlight — never stuck.
//
// The jump is the rows-per-notch multiplier (GONOMADNET_WHEEL_LINES), so one
// notch moves N rows. Lists do not use tview TextView's trackEnd, so this
// per-primitive multiplier (one delivery of N rows) is safe — unlike a root
// re-dispatch, which triggers the TextView trackEnd jump-to-bottom bug on
// TextView-based regions (see applyWheelMultiplier).
//
// The wheel uses i.InRect (the full bar+list rect), not the inset List rect, so
// a wheel over the indicator rows also scrolls (the inset excludes them, so
// List.InRect would bail — the old indicator-row no-op gap). At a scroll
// boundary (next == current) the handler declines to consume so tview skips the
// no-op redraw, the same philosophy as the boundary guard in scroll-bar.go /
// browser-nav.go. SetCurrentItem fires only the changed callback; no
// IndicativeListBox list wires SetChangedFunc (guide.go:179 even comments "no
// SetChangedFunc"), so a wheel fires nothing.
//
// When the mouse-debug logger is enabled (GONOMADNET_MOUSE_DEBUG), wheel events
// are traced with the current/next index and item count.
func (i *IndicativeListBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	base := i.List.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			x, y := event.Position()
			if !i.InRect(x, y) {
				return false, nil
			}
			count := i.List.GetItemCount()
			if count == 0 {
				return false, nil
			}
			current := i.List.GetCurrentItem()
			delta := mouseWheelLines
			next := current - delta
			if action == tview.MouseScrollDown {
				next = current + delta
			}
			next = max(0, min(count-1, next))
			if next == current {
				// Already at the boundary: decline to consume so tview skips
				// the no-op redraw.
				return false, nil
			}
			i.List.SetCurrentItem(next)
			if mouseDebug != nil {
				dbgMouse("ILB wheel %v: current=%v next=%v delta=%v items=%v",
					action, current, next, delta, count)
			}
			return true, nil
		}
		return base(action, event, setFocus)
	}
}
