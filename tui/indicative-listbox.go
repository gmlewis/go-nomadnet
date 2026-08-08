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
	List        *tview.List
	emptyText   string          // centered placeholder drawn when the List has no items
	emptyWidget tview.Primitive // optional widget drawn in the list area when empty
}

// NewIndicativeListBox wraps the given List with indicator bars. If list is
// nil a new (empty) List is created. The caller may continue to configure the
// List (AddItem, SetSelectedFunc, ShowSecondaryText, …) via the List field.
func NewIndicativeListBox(list *tview.List) *IndicativeListBox {
	if list == nil {
		list = tview.NewList()
	}
	return &IndicativeListBox{Box: tview.NewBox(), List: list}
}

// SetEmptyText sets the centered placeholder drawn in the list area when the
// wrapped List has no items, mirroring nomadnet's empty list body
// `[urwid.Text(empty_label, align='center')]` (Conversations.py:496). Without a
// real item the indicator bars still render "───" above and below it.
func (i *IndicativeListBox) SetEmptyText(text string) { i.emptyText = text }

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
			tview.Print(screen, i.emptyText, lx, ly, lw, tview.AlignCenter, tcell.ColorDefault)
		}
	}

	top, bottom := i.indicators()
	if h >= 3 {
		tview.Print(screen, top, x, y, w, tview.AlignCenter, tcell.ColorDefault)
		tview.Print(screen, bottom, x, y+h-1, w, tview.AlignCenter, tcell.ColorDefault)
	} else if h == 2 {
		tview.Print(screen, top, x, y, w, tview.AlignCenter, tcell.ColorDefault)
	}

	if i.HasFocus() {
		count := i.List.GetItemCount()
		if count > 0 {
			current := i.List.GetCurrentItem()
			offset, _ := i.List.GetOffset()
			lx, ly, _, lh := i.listRect()
			row := current - offset
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

// MouseHandler delegates to the wrapped List (whose rect was set in SetRect).
//
// When the mouse-debug logger is enabled (GONOMADNET_MOUSE_DEBUG), wheel events
// are traced: the event point, whether it falls in the IndicativeListBox's full
// rect vs the inset List rect (the inset is one row top/bottom for the ▲/▼
// indicator bars — wheel over those rows would otherwise no-op because
// tview.List bails on !InRect), the current offset, item count and visible
// height, plus the consumed result. This localizes live-only wheel failures on
// the Network Announce Stream / Saved Nodes lists.
func (i *IndicativeListBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	base := i.List.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		consumed, capture = base(action, event, setFocus)
		if mouseDebug != nil && (action == tview.MouseScrollUp || action == tview.MouseScrollDown) {
			x, y := event.Position()
			_, _, _, listH := i.List.GetRect()
			off, _ := i.List.GetOffset()
			dbgMouse("ILB wheel %v at (%v,%v): inBox=%v inList=%v offset=%v items=%v listH=%v consumed=%v",
				action, x, y, i.InRect(x, y), i.List.InRect(x, y), off, i.List.GetItemCount(), listH, consumed)
		}
		return consumed, capture
	}
}
