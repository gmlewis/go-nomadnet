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

// IndicativeMessages wraps the room's message TextView with the centered
// top/bottom indicator bars of Python's _StickyMessageListBox (an
// IndicativeListBox, vendor/additional_urwid_widgets/widgets/
// indicative_listbox.py): the top bar shows "───" when the first message row
// is visible and "▲" when content is hidden above; the bottom bar shows
// "───" when the last row is visible (the sticky tail) and "▼" when content
// is hidden below. Both bars are centered and consume one viewport row each,
// exactly like the wrapped-List IndicativeListBox (tui/indicative-listbox.go).
type IndicativeMessages struct {
	*tview.Box
	Text *tview.TextView
}

// NewIndicativeMessages wraps the given message TextView with the indicator
// bars. The TextView keeps all rendering, scrolling, wheel and color-tag
// behavior; this primitive only reserves one row above and one below and
// draws the bars after the TextView (so its line index is current).
func NewIndicativeMessages(tv *tview.TextView) *IndicativeMessages {
	return &IndicativeMessages{Box: tview.NewBox(), Text: tv}
}

// messagesRect returns the rect to assign to the wrapped TextView (the full
// rect minus one row top and bottom when height >= 3), mirroring
// IndicativeListBox.listRect.
func (m *IndicativeMessages) messagesRect() (int, int, int, int) {
	x, y, w, h := m.GetRect()
	if h >= 3 {
		return x, y + 1, w, h - 2
	}
	return x, y, w, h
}

// SetRect lays out the wrapped TextView with the indicator rows reserved.
func (m *IndicativeMessages) SetRect(x, y, w, h int) {
	m.Box.SetRect(x, y, w, h)
	tx, ty, tw, th := m.messagesRect()
	m.Text.SetRect(tx, ty, tw, th)
}

// Draw draws the wrapped TextView then the two indicator bars.
func (m *IndicativeMessages) Draw(screen tcell.Screen) {
	m.Box.DrawForSubclass(screen, m)
	m.Text.Draw(screen)
	x, y, w, h := m.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	top, bottom := m.indicators()
	// The bars are centered like urwid Text, which pads the LEFT side with
	// (w-len+1)/2 spaces — a ceil, matching IndicativeListBox.Draw.
	centerBarX := func(s string) int { return x + max((w-len(s)+1)/2, 0) }
	if h >= 3 {
		tview.Print(screen, top, centerBarX(top), y, w, tview.AlignLeft, tcell.ColorDefault)
		tview.Print(screen, bottom, centerBarX(bottom), y+h-1, w, tview.AlignLeft, tcell.ColorDefault)
	} else if h == 2 {
		tview.Print(screen, top, centerBarX(top), y, w, tview.AlignLeft, tcell.ColorDefault)
	}
}

// indicators returns the (top, bottom) bar strings for the TextView's current
// scroll position: "───" when the respective end is visible, "▲"/"▼" when
// content is hidden beyond it. Content that fits the viewport exposes both
// ends (both "───"), like the vendor ILB with an all-visible list.
func (m *IndicativeMessages) indicators() (top, bottom string) {
	_, _, _, th := m.messagesRect()
	visible := max(th, 1)
	row, _ := m.Text.GetScrollOffset()
	total := m.Text.GetWrappedLineCount()

	top = "───"
	if row > 0 {
		top = "▲"
	}
	bottom = "───"
	if total > visible && row+visible < total {
		bottom = "▼"
	}
	return top, bottom
}

// Focus forwards to the wrapped TextView.
func (m *IndicativeMessages) Focus(delegate func(p tview.Primitive)) {
	m.Box.Focus(delegate)
	m.Text.Focus(delegate)
}

// InputHandler forwards to the wrapped TextView (through the Box's capture
// chain, matching IndicativeListBox's delegation).
func (m *IndicativeMessages) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.Text.InputHandler()
}

// MouseHandler forwards to the wrapped TextView (the fork's handler signature
// returns consumed+capture like IndicativeListBox.MouseHandler).
func (m *IndicativeMessages) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return m.Text.MouseHandler()
}
