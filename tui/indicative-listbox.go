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
	List *tview.List
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

	top, bottom := i.indicators()
	if h >= 3 {
		tview.Print(screen, top, x, y, w, tview.AlignCenter, tcell.ColorDefault)
		tview.Print(screen, bottom, x, y+h-1, w, tview.AlignCenter, tcell.ColorDefault)
	} else if h == 2 {
		tview.Print(screen, top, x, y, w, tview.AlignCenter, tcell.ColorDefault)
	}
}

// indicators returns the (top, bottom) bar strings for the List's current
// scroll position: "───" when the respective end is visible, "▲"/"▼" when items
// are hidden beyond it. An empty list exposes both ends.
func (i *IndicativeListBox) indicators() (top, bottom string) {
	itemOffset, _ := i.List.GetOffset()
	count := i.List.GetItemCount()
	_, _, _, listH := i.listRect()
	visible := listH
	if visible < 1 {
		visible = 1
	}

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
	i.List.Focus(delegate)
}

// Blur forwards to the wrapped List.
func (i *IndicativeListBox) Blur() {
	i.List.Blur()
}

// HasFocus reports the wrapped List's focus state.
func (i *IndicativeListBox) HasFocus() bool {
	return i.List.HasFocus()
}

// InputHandler delegates to the wrapped List.
func (i *IndicativeListBox) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return i.List.InputHandler()
}

// MouseHandler delegates to the wrapped List (whose rect was set in SetRect).
func (i *IndicativeListBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return i.List.MouseHandler()
}