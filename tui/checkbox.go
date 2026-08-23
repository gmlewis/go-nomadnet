// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
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

// checkboxCheckedGlyph is the 3-rune checked indicator of a urwid CheckBox
// (urwid/widget/wimp.py: CheckBox.states[True] = "[X]").
const checkboxCheckedGlyph = "[X]"

// checkboxUncheckedGlyph is the unchecked indicator (CheckBox.states[False] =
// "[ ]").
const checkboxUncheckedGlyph = "[ ]"

// checkboxIndicatorWidth is 4: the 3-rune glyph plus one trailing space
// (urwid's CheckBox.reserve_columns), so the rendered row is "[X] Label".
const checkboxIndicatorWidth = 4

// UrwidCheckbox is a checkbox matching urwid's CheckBox glyph format:
// "[X] label" when checked, "[ ] label" when unchecked. It draws directly
// to the screen (via SetContent) so tview's color-tag parser never sees the
// square brackets. Python source: urwid/widget/wimp.py CheckBox.
type UrwidCheckbox struct {
	*tview.Box
	label    string
	checked  bool
	onChange func(bool)
	onFocus  func(tview.Primitive)
}

// newUrwidCheckbox creates a checkbox in the urwid glyph format.
func newUrwidCheckbox(label string, checked bool) *UrwidCheckbox {
	return &UrwidCheckbox{
		Box:     tview.NewBox(),
		label:   label,
		checked: checked,
	}
}

// SetLabel sets the checkbox label text.
func (uc *UrwidCheckbox) SetLabel(label string) *UrwidCheckbox {
	uc.label = label
	return uc
}

// Label returns the label text.
func (uc *UrwidCheckbox) Label() string { return uc.label }

// SetChecked sets the checked state, firing the change callback.
func (uc *UrwidCheckbox) SetChecked(checked bool) *UrwidCheckbox {
	if uc.checked == checked {
		return uc
	}
	uc.checked = checked
	if uc.onChange != nil {
		uc.onChange(checked)
	}
	return uc
}

// IsChecked reports whether the checkbox is currently checked.
func (uc *UrwidCheckbox) IsChecked() bool { return uc.checked }

// SetChangedFunc sets a callback fired when the checked state changes.
func (uc *UrwidCheckbox) SetChangedFunc(fn func(bool)) *UrwidCheckbox {
	uc.onChange = fn
	return uc
}

// SetFocusFunc sets a callback fired when the checkbox gains focus.
func (uc *UrwidCheckbox) SetFocusFunc(fn func(tview.Primitive)) *UrwidCheckbox {
	uc.onFocus = fn
	return uc
}

// Draw renders "[X] label" or "[ ] label" directly via SetContent so tview's
// color-tag parser never interprets the square brackets.
func (uc *UrwidCheckbox) Draw(screen tcell.Screen) {
	uc.Box.DrawForSubclass(screen, uc)
	x, y, w, h := uc.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	glyph := checkboxUncheckedGlyph
	if uc.checked {
		glyph = checkboxCheckedGlyph
	}
	px := x
	for _, r := range glyph {
		screen.SetContent(px, y, r, nil, style)
		px++
	}
	for px < x+checkboxIndicatorWidth {
		screen.SetContent(px, y, ' ', nil, style)
		px++
	}
	for _, r := range uc.label {
		if px >= x+w {
			break
		}
		screen.SetContent(px, y, r, nil, style)
		px++
	}
}

// Focus calls the focus callback when gaining focus.
func (uc *UrwidCheckbox) Focus(delegate func(p tview.Primitive)) {
	if uc.onFocus != nil {
		uc.onFocus(uc)
	}
	uc.Box.Focus(delegate)
}

// InputHandler toggles the checkbox on Space/Enter.
func (uc *UrwidCheckbox) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return uc.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyRune:
			if event.Rune() != ' ' {
				return
			}
		case tcell.KeyEnter:
		default:
			return
		}
		uc.SetChecked(!uc.checked)
	})
}

// MouseHandler toggles on left click.
func (uc *UrwidCheckbox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return uc.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick {
			return false, nil
		}
		mx, my := event.Position()
		if !uc.InRect(mx, my) {
			return false, nil
		}
		setFocus(uc)
		uc.SetChecked(!uc.checked)
		return true, nil
	})
}
