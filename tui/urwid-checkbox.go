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

// urwidCheckBoxChecked/Unchecked are urwid's CheckBox state glyphs
// (urwid/widget/wimp.py: CheckBox.states = {True: "[X]", False: "[ ]"}).
const (
	urwidCheckBoxChecked   = "[X]"
	urwidCheckBoxUnchecked = "[ ]"
)

// urwidCheckBoxIndicatorWidth is urwid's CheckBox.reserve_columns (wimp.py):
// the 3-rune state glyph plus one trailing space, so the label starts at the
// 5th column ("[X] Pin to top").
const urwidCheckBoxIndicatorWidth = 4

// UrwidCheckBox ports urwid's CheckBox (urwid/widget/wimp.py:258-380) for
// nomadnet dialogs (e.g. the Peer Info "Pin to top" checkbox,
// Conversations.py:890). It renders "[X] label" when checked or "[ ] label"
// when unchecked — unlike tview.Checkbox, which draws only the bare label and
// a standalone cursor cell — in the default text style with no focus color.
// Focus is indicated by a hardware cursor on the middle cell of the state
// glyph (urwid's SelectableIcon cursor position 1), shown via screen.ShowCursor
// (cursor parity with the original).
type UrwidCheckBox struct {
	*tview.Box
	label    string
	checked  bool
	onChange func(bool)
}

// NewUrwidCheckBox creates a checkbox with the given label and initial state.
func NewUrwidCheckBox(label string, checked bool) *UrwidCheckBox {
	return &UrwidCheckBox{
		Box:     tview.NewBox(),
		label:   label,
		checked: checked,
	}
}

// SetChecked sets the checked state (no group semantics — checkboxes are
// independent, unlike RadioButton).
func (cb *UrwidCheckBox) SetChecked(checked bool) {
	if cb.checked == checked {
		return
	}
	cb.checked = checked
	if cb.onChange != nil {
		cb.onChange(checked)
	}
}

// IsChecked reports whether the checkbox is currently checked.
func (cb *UrwidCheckBox) IsChecked() bool { return cb.checked }

// SetChangedFunc installs a callback fired when the checked state changes
// (urwid's on_state_change / "change" signal; nomadnet's dialogs use the
// value read back at Save time, so wiring a callback is optional).
func (cb *UrwidCheckBox) SetChangedFunc(fn func(checked bool)) *UrwidCheckBox {
	cb.onChange = fn
	return cb
}

// Draw renders "[X] label" / "[ ] label" in the default text style and, when
// focused, shows the hardware cursor on the middle cell of the state glyph
// (urwid SelectableIcon cursor position 1).
func (cb *UrwidCheckBox) Draw(screen tcell.Screen) {
	cb.Box.DrawForSubclass(screen, cb)
	x, y, w, h := cb.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	glyph := urwidCheckBoxUnchecked
	if cb.checked {
		glyph = urwidCheckBoxChecked
	}
	px := x
	for _, r := range glyph {
		screen.SetContent(px, y, r, nil, style)
		px++
	}
	// urwidCheckBoxIndicatorWidth is 4: the 3-rune glyph plus one space.
	for px < x+urwidCheckBoxIndicatorWidth {
		screen.SetContent(px, y, ' ', nil, style)
		px++
	}
	for _, r := range cb.label {
		if px >= x+w {
			break
		}
		screen.SetContent(px, y, r, nil, style)
		px++
	}
	// urwid's CheckBox wraps the state icon in a SelectableIcon whose cursor
	// position is the middle cell; show it when focused so the hardware cursor
	// matches the original.
	if cb.HasFocus() {
		screen.ShowCursor(x+1, y)
	}
}

// InputHandler toggles the checkbox on Space/Enter (urwid CheckBox.keypress
// maps both " " and "enter" to toggle_state).
func (cb *UrwidCheckBox) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return cb.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyRune:
			if event.Rune() != ' ' {
				return
			}
		case tcell.KeyEnter:
		default:
			return
		}
		cb.SetChecked(!cb.checked)
	})
}

// MouseHandler toggles the checkbox on left click, like urwid's CheckBox.
func (cb *UrwidCheckBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return cb.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick {
			return false, nil
		}
		mx, my := event.Position()
		if !cb.InRect(mx, my) {
			return false, nil
		}
		setFocus(cb)
		cb.SetChecked(!cb.checked)
		return true, nil
	})
}
