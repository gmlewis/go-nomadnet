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

// urwidButtonLeft and urwidButtonRight are urwid's default button brackets
// (urwid/widget/wimp.py: Button.button_left = Text("<"), button_right =
// Text(">")). urwid's Button renders a FLAT "< label >", NOT a bordered box.
const (
	urwidButtonLeft  = "<"
	urwidButtonRight = ">"
)

// urwidButtonDivideChars is urwid's Button.dividechars default (1): one blank
// column between each bracket and the label, so the row is
// "<" + " " + label + " " + ">".
const urwidButtonDivideChars = 1

// UrwidButton ports urwid's flat Button (urwid/widget/wimp.py). It renders
// "< label >" — a left "<" bracket, one blank, the left-justified label padded
// to fill the box width, one blank, and a right ">" bracket — with NO border
// (unlike tview.Button, which draws a bordered box). urwid applies no color to
// a plain Button (the nomadnet "buttons" palette entry is not bound to a plain
// urwid.Button), so the row is drawn in the default text style whether or not
// it has focus; focus is indicated by a hardware cursor on the first label
// cell (urwid's SelectableIcon cursor position 0), shown via screen.ShowCursor.
type UrwidButton struct {
	*tview.Box
	label    string
	selected func()
}

// NewUrwidButton creates a flat "< label >" button with no selected handler.
// Chain SetSelectedFunc to install the activate callback.
func NewUrwidButton(label string) *UrwidButton {
	return &UrwidButton{
		Box:   tview.NewBox(),
		label: label,
	}
}

// SetSelectedFunc installs the callback fired when the button is activated
// (Space/Enter or left click), mirroring urwid Button's on_press.
func (b *UrwidButton) SetSelectedFunc(fn func()) *UrwidButton {
	b.selected = fn
	return b
}

// Label returns the button's label text.
func (b *UrwidButton) Label() string { return b.label }

// Draw renders "< label >" filling the box width in the default text style.
// The label cell is w - 2*dividechars - 2 cells wide (one per bracket); the
// label is left-justified and padded with spaces so the ">" sits at the right
// edge, matching urwid's Columns layout (brackets PACK, label absorbs the rest).
func (b *UrwidButton) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	x, y, w, h := b.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	setRune := func(col int, r rune) {
		if col >= x && col < x+w {
			screen.SetContent(col, y, r, nil, style)
		}
	}
	// Left bracket + dividechars blank.
	px := x
	for _, r := range urwidButtonLeft {
		setRune(px, r)
		px++
	}
	for i := 0; i < urwidButtonDivideChars; i++ {
		setRune(px, ' ')
		px++
	}
	// Label cell width = w - 2 (brackets) - 2*dividechars.
	labelW := w - 2 - 2*urwidButtonDivideChars
	if labelW < 0 {
		labelW = 0
	}
	labelRunes := []rune(b.label)
	for i := 0; i < labelW; i++ {
		var r rune = ' '
		if i < len(labelRunes) {
			r = labelRunes[i]
		}
		setRune(px, r)
		px++
	}
	// dividechars blank + right bracket.
	for i := 0; i < urwidButtonDivideChars; i++ {
		setRune(px, ' ')
		px++
	}
	for _, r := range urwidButtonRight {
		setRune(px, r)
		px++
	}
	// urwid's SelectableIcon places the cursor at label position 0; show it
	// when focused so the hardware cursor matches the original (Phase 0).
	if b.HasFocus() {
		screen.ShowCursor(x+1+urwidButtonDivideChars, y)
	}
}

// InputHandler activates the button on Space/Enter (urwid Button maps both
// " " and "enter" to activate).
func (b *UrwidButton) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyRune:
			if event.Rune() != ' ' {
				return
			}
		case tcell.KeyEnter:
		default:
			return
		}
		if b.selected != nil {
			b.selected()
		}
	})
}

// MouseHandler activates the button on a left click within its rect, like
// urwid's Button.
func (b *UrwidButton) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if action != tview.MouseLeftClick {
			return false, nil
		}
		mx, my := event.Position()
		if !b.InRect(mx, my) {
			return false, nil
		}
		setFocus(b)
		if b.selected != nil {
			b.selected()
		}
		return true, nil
	})
}