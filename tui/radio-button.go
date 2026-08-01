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

// radioCheckedGlyph is the 3-rune checked indicator of a urwid RadioButton
// (urwid/widget/wimp.py: RadioButton.states[True] = SelectableIcon("(X)", 1)).
const radioCheckedGlyph = "(X)"

// radioUncheckedGlyph is the unchecked indicator (RadioButton.states[False] =
// SelectableIcon("( )", 1)). The cursor sits on the middle cell (position 1).
const radioUncheckedGlyph = "( )"

// radioIndicatorWidth is urwid's CheckBox.reserve_columns for a RadioButton
// (wimp.py:466): the state icon occupies 4 columns, so the label starts one
// column after the 3-rune "(X)"/"( )" glyph — i.e. the rendered row is
// "(X) Label" (glyph + trailing space + label).
const radioIndicatorWidth = 4

// DialogRadioGroup is the shared mutual-exclusion list for a set of
// RadioButtons, mirroring urwid's RadioButton group list (wimp.py:
// RadioButton.__init__ appends self to group). Members are tracked in
// insertion order. (This is distinct from the micron-field RadioGroup in
// field-widget.go, which groups tview.Checkbox widgets.)
type DialogRadioGroup struct {
	Members []*RadioButton
}

// RadioButton is a single radio button, porting urwid's RadioButton
// (urwid/widget/wimp.py:460). It renders "(X) label" when checked or
// "( ) label" when unchecked (4-column indicator cell + label), in the default
// text color — urwid applies no focus color to a radio, only a hardware cursor
// on the middle cell, which is handled separately (Phase 0 cursor parity).
//
// Construction matches urwid's quirk: creating a RadioButton with a checked
// state does NOT uncheck the other members of its group (RadioButton.__init__
// sets _state directly without calling set_state, so the group-uncheck only
// happens on a user toggle). This means a group built as [first-default-True,
// explicit-True, …] briefly shows TWO checked radios, exactly like the
// original (e.g. the New Conversation dialog opens with both "Untrusted" and
// "Unknown" showing "(X)"). Checking a radio via SetChecked/Space/Enter does
// uncheck the others.
type RadioButton struct {
	*tview.Box
	group    *DialogRadioGroup
	label    string
	checked  bool
	onChange func(bool)
}

// NewRadioButton creates a radio button in group with the given label.
//
// If firstTrue is set, the button is checked only when the group is empty
// (urwid's state="first True" default: `state = not group`). The new button is
// appended to group.Members. Per urwid semantics, setting checked during
// construction does NOT uncheck the other members — only a later SetChecked(true)
// or Space/Enter toggle does.
func NewRadioButton(group *DialogRadioGroup, label string, checked, firstTrue bool) *RadioButton {
	if firstTrue {
		checked = len(group.Members) == 0
	}
	rb := &RadioButton{
		Box:     tview.NewBox(),
		group:   group,
		label:   label,
		checked: checked,
	}
	group.Members = append(group.Members, rb)
	return rb
}

// Checked reports whether the radio button is currently selected.
func (rb *RadioButton) Checked() bool { return rb.checked }

// SetChecked sets the checked state. When checking (checked==true) every other
// member of the same group is unchecked, mirroring urwid's
// RadioButton.set_state(True). The onChange callback (if any) fires for this
// button and for each sibling whose state changed. Setting checked==false on a
// radio does not check any sibling (urwid radios cannot all be unset via a
// single toggle); it just unsets this one.
func (rb *RadioButton) SetChecked(checked bool) {
	if rb.checked == checked {
		return
	}
	rb.checked = checked
	if rb.onChange != nil {
		rb.onChange(checked)
	}
	if checked && rb.group != nil {
		for _, m := range rb.group.Members {
			if m != rb && m.checked {
				m.checked = false
				if m.onChange != nil {
					m.onChange(false)
				}
			}
		}
	}
}

// SetChangedFunc sets a callback fired when this button's checked state
// changes (including side-effect unchecks when a sibling is selected).
func (rb *RadioButton) SetChangedFunc(fn func(checked bool)) *RadioButton {
	rb.onChange = fn
	return rb
}

// Label returns the radio button's label text.
func (rb *RadioButton) Label() string { return rb.label }

// Draw renders "(X) label" or "( ) label" in the default text style. urwid
// applies no focus color to a RadioButton (only a hardware cursor on the middle
// cell), so focused and unfocused render identically here.
func (rb *RadioButton) Draw(screen tcell.Screen) {
	rb.Box.DrawForSubclass(screen, rb)
	x, y, w, h := rb.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	glyph := radioUncheckedGlyph
	if rb.checked {
		glyph = radioCheckedGlyph
	}
	px := x
	for _, r := range glyph {
		screen.SetContent(px, y, r, nil, style)
		px++
	}
	// radioIndicatorWidth is 4: the 3-rune glyph plus one trailing space.
	for px < x+radioIndicatorWidth {
		screen.SetContent(px, y, ' ', nil, style)
		px++
	}
	for _, r := range rb.label {
		if px >= x+w {
			break
		}
		screen.SetContent(px, y, r, nil, style)
		px++
	}
}

// InputHandler toggles the radio on Space/Enter (urwid RadioButton maps both
// " " and "enter" to activate), unchecking the other group members.
func (rb *RadioButton) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return rb.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyRune:
			if event.Rune() != ' ' {
				return
			}
		case tcell.KeyEnter:
		default:
			return
		}
		if !rb.checked {
			rb.SetChecked(true)
		}
	})
}

// MouseHandler toggles the radio on left click, like urwid's RadioButton.
func (rb *RadioButton) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return rb.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick {
			return false, nil
		}
		mx, my := event.Position()
		if !rb.InRect(mx, my) {
			return false, nil
		}
		setFocus(rb)
		if !rb.checked {
			rb.SetChecked(true)
		}
		return true, nil
	})
}