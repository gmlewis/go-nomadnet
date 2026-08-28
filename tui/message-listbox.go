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

// messageListBox is the conversation message list: a vertical list of
// per-message entries (one rendered LXMessageWidget equivalent each) with a
// movable focus and per-message autoscroll, mirroring Python's
// messagelist = IndicativeListBox(self.message_widgets)
// (Conversations.py:2286-2287).
//
// Python renders each message as a selectable urwid Pile inside a ListBox;
// Up/Down move the ListBox focus one WHOLE message at a time and the view
// autoscrolls to keep the focused message visible. The wrapper AttrMaps use
// no focus style (IndicativeListBox is built without highlight_offFocus), so
// the movement is visible only through the per-message scroll steps — there
// is deliberately no highlight bar and no cursor here either.
//
// The FocusIndex starts at the newest message (IndicativeListBox
// position=len-1, Conversations.py:2287). TopIsVisible/BottomIsVisible mirror
// IndicativeListBox.top_is_visible/bottom_is_visible (vendor
// indicative_listbox.py render), which ConversationFrame.keypress branches on
// (Conversations.py:1845-1870): Up at the top collapses to the trust banner or
// the menu bar, Down at the bottom moves to the composer.
type messageListBox struct {
	*tview.Box
	entries []*messageListView
	rows    []int // cumulative top row of each entry within the virtual list
	total   int   // total rendered rows across entries
	offset  int   // first visible row of the virtual list
	width   int   // layout width used for the entry heights (0 = unset)
	focus   int   // focused entry index; starts at the last (newest) entry

	// bannerVisible, when set, reports whether the conversation's trust banner
	// is currently visible. The main dispatcher consults it via bodyListAtTop
	// so Up at the message-list top goes to the banner (Python
	// ConversationFrame.keypress → _header_pile.focus_position = 1) instead of
	// the menu bar (Conversations.py:1854-1862).
	bannerVisible func() bool
}

// newMessageListBox creates an empty message list.
func newMessageListBox() *messageListBox {
	return &messageListBox{Box: tview.NewBox()}
}

// EntryCount returns the number of message entries.
func (m *messageListBox) EntryCount() int { return len(m.entries) }

// FocusIndex returns the focused entry index.
func (m *messageListBox) FocusIndex() int { return m.focus }

// Offset returns the first visible row of the virtual list (the scroll
// position), mirroring IndicativeListBox's exposed scroll state.
func (m *messageListBox) Offset() int { return m.offset }

// SetEntries replaces the message entries. Heights are computed at the last
// known width; the focus moves to the newest entry and the view scrolls so it
// is visible (Python IndicativeListBox position=len-1).
func (m *messageListBox) SetEntries(entries []*messageListView) {
	m.entries = entries
	m.layout()
	if n := len(m.entries); n > 0 {
		m.focus = n - 1
		m.scrollToFocus()
	} else {
		m.focus = 0
		m.offset = 0
	}
}

// SetRect lays out the list; a width change re-wraps every entry height and
// re-clamps the scroll position.
func (m *messageListBox) SetRect(x, y, w, h int) {
	if w != m.width {
		m.width = w
		m.layout()
		if m.focus >= len(m.entries) {
			m.focus = max(len(m.entries)-1, 0)
		}
		m.scrollToFocus()
	}
	m.Box.SetRect(x, y, w, h)
}

// layout recomputes each entry's wrapped height at the current width and the
// cumulative row table. Entries with no width yet get a placeholder height of
// their un-wrapped line count (recomputed on the first SetRect).
func (m *messageListBox) layout() {
	m.rows = make([]int, len(m.entries))
	m.total = 0
	for i := range m.entries {
		m.rows[i] = m.total
		m.total += m.entryHeight(i)
	}
}

// entryHeight returns the rendered height of entry i at the layout width,
// matching tview's internal TextView wrapping (both use tview.WordWrap).
func (m *messageListBox) entryHeight(i int) int {
	if m.width <= 0 || i >= len(m.entries) {
		return 1
	}
	text := m.entries[i].GetText(false)
	lines := tview.WordWrap(text, m.width)
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}

// viewport height (the number of visible rows).
func (m *messageListBox) viewHeight() int {
	_, _, _, h := m.GetRect()
	return max(h, 1)
}

// scrollToFocus adjusts the offset so the focused entry is fully visible
// (urwid ListBox change_focus semantics), clamped to the scrollable range.
func (m *messageListBox) scrollToFocus() {
	if m.focus < 0 || m.focus >= len(m.entries) || m.total == 0 {
		m.offset = 0
		return
	}
	h := m.viewHeight()
	top := m.rows[m.focus]
	bottom := top + m.entryHeight(m.focus)
	switch {
	case top < m.offset:
		m.offset = top
	case bottom > m.offset+h:
		m.offset = bottom - h
	}
	m.clampOffset()
}

// clampOffset bounds the offset to [0, total-viewport].
func (m *messageListBox) clampOffset() {
	maxOff := max(m.total-m.viewHeight(), 0)
	m.offset = min(max(m.offset, 0), maxOff)
}

// SetFocusIndex moves the focus to entry i and autoscrolls (no boundary
// return value — callers drive the Python top/bottom transitions themselves).
func (m *messageListBox) SetFocusIndex(i int) {
	if len(m.entries) == 0 {
		return
	}
	m.focus = min(max(i, 0), len(m.entries)-1)
	m.scrollToFocus()
}

// TopIsVisible mirrors IndicativeListBox.top_is_visible: nothing is scrolled
// off the top (vendor indicative_listbox.py render: trim_top==0 and no
// previous position).
func (m *messageListBox) TopIsVisible() bool {
	return m.offset <= 0
}

// BottomIsVisible mirrors IndicativeListBox.bottom_is_visible: the list end
// is reached — either every row fits below the offset or the focused entry is
// the last one (urwid get_next(pos)==(None,None) makes the bottom visible even
// when the last message is cut off by the viewport).
func (m *messageListBox) BottomIsVisible() bool {
	return m.offset+m.viewHeight() >= m.total || m.focus == len(m.entries)-1
}

// FocusedEntryRows returns the [top,bottom) rows of the focused entry within
// the virtual list. ok is false when there are no entries.
func (m *messageListBox) FocusedEntryRows() (top, bottom int, ok bool) {
	if m.focus < 0 || m.focus >= len(m.entries) {
		return 0, 0, false
	}
	top = m.rows[m.focus]
	return top, top + m.entryHeight(m.focus), true
}

// Draw paints the visible portion of every entry. Entries partially scrolled
// off the top are drawn with their TextView scrolled to the first visible
// line; entries cut off at the bottom get a clipped rect.
func (m *messageListBox) Draw(screen tcell.Screen) {
	m.Box.DrawForSubclass(screen, m)
	if len(m.entries) == 0 {
		return
	}
	x, y, w, h := m.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	for i, e := range m.entries {
		top := m.rows[i]
		bottom := top + m.entryHeight(i)
		if bottom <= m.offset || top >= m.offset+h {
			continue
		}
		if w != m.width {
			m.width = w
			m.layout()
		}
		eh := m.entryHeight(i)
		vy := y + top - m.offset
		if top < m.offset {
			// Partially above the viewport: scroll the entry's TextView to its
			// first visible line and clip to the visible rows.
			visible := min(bottom-m.offset, h)
			e.SetRect(x, y, w, visible)
			e.ScrollTo(min(m.offset-top, max(eh-1, 0)), 0)
		} else {
			visible := min(eh, h-(top-m.offset))
			e.SetRect(x, vy, w, visible)
			e.ScrollTo(0, 0)
		}
		e.Draw(screen)
	}
}

// InputHandler moves the focus one entry per Up/Down with autoscroll. Keys at
// the boundaries are returned unconsumed so the surrounding frame capture can
// apply Python's ConversationFrame top/bottom transitions (up → trust banner
// or menu, down → composer); inside the list the keys are consumed.
func (m *messageListBox) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if len(m.entries) == 0 {
			return
		}
		switch event.Key() {
		case tcell.KeyUp:
			if m.focus > 0 {
				m.focus--
				m.scrollToFocus()
				return
			}
		case tcell.KeyDown:
			if m.focus < len(m.entries)-1 {
				m.focus++
				m.scrollToFocus()
				return
			}
		case tcell.KeyHome:
			m.SetFocusIndex(0)
			return
		case tcell.KeyEnd:
			m.SetFocusIndex(len(m.entries) - 1)
			return
		case tcell.KeyPgUp, tcell.KeyPgDn:
			// Move the focus to the entry nearest the opposite viewport edge
			// (urwid moves the focus to the first/last widget in view).
			h := m.viewHeight()
			if event.Key() == tcell.KeyPgUp {
				m.SetFocusIndex(m.entryAtRow(m.offset))
			} else {
				m.SetFocusIndex(m.entryAtRow(m.offset + h - 1))
			}
			return
		}
	})
}

// entryAtRow returns the index of the entry containing the given virtual row.
func (m *messageListBox) entryAtRow(row int) int {
	idx := 0
	for i, r := range m.rows {
		if r <= row {
			idx = i
		} else {
			break
		}
	}
	return idx
}

// MouseHandler translates the wheel into per-entry focus moves (the same
// arrow-key semantics IndicativeListBox uses for its wrapped List), and
// delegates other mouse actions to the entries.
func (m *messageListBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			x, y := event.Position()
			if !m.InRect(x, y) || len(m.entries) == 0 {
				return false, nil
			}
			delta := mouseWheelLines
			next := m.focus - delta
			if action == tview.MouseScrollDown {
				next = m.focus + delta
			}
			before := m.focus
			m.SetFocusIndex(next)
			return m.focus != before, nil
		}
		return false, nil
	}
}
