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

// tabButtonLeft and tabButtonRight are the brackets nomadnet's TabButton uses
// (Conversations.py:82-84: a urwid.Button subclass with button_left="[",
// button_right="]"), so a tab renders "[ label ]".
const (
	tabButtonLeft  = "["
	tabButtonRight = "]"
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
//
// The bracket strings are configurable so the same primitive backs TabButton
// (Conversations.py:82), which only overrides the brackets to "[" / "]".
type UrwidButton struct {
	*tview.Box
	label        string
	leftBracket  string
	rightBracket string
	selected     func()
}

// NewUrwidButton creates a flat "< label >" button with no selected handler.
// Chain SetSelectedFunc to install the activate callback.
func NewUrwidButton(label string) *UrwidButton {
	return &UrwidButton{
		Box:          tview.NewBox(),
		label:        label,
		leftBracket:  urwidButtonLeft,
		rightBracket: urwidButtonRight,
	}
}

// NewTabButton creates a flat "[ label ]" button matching nomadnet's TabButton
// (Conversations.py:82-84), used for the Conversations Trusted/Untrusted tab
// bar. Activation (Enter/Space/click) is wired via SetSelectedFunc to the
// filter-switch callback, like urwid's TabButton on_press → _set_filter.
func NewTabButton(label string) *UrwidButton {
	return &UrwidButton{
		Box:          tview.NewBox(),
		label:        label,
		leftBracket:  tabButtonLeft,
		rightBracket: tabButtonRight,
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

// SetLabel updates the button's label text (used to refresh tab counts without
// rebuilding the button).
func (b *UrwidButton) SetLabel(label string) *UrwidButton { b.label = label; return b }

// Draw renders the bracketed, wrapped label filling the box width in the
// default text style. The label cell is w - 2*dividechars - 2 cells wide (one
// per bracket); the label is left-justified, space-wrapped (urwid SelectableIcon
// wrap=SPACE) and right-padded so the right bracket sits at the right edge,
// matching urwid's Columns layout (brackets PACK, label absorbs the rest). When
// the label wraps, the 1-row bracket Texts are top-aligned and blank-filled
// below, so the brackets appear ONLY on the first row and subsequent rows show
// the wrapped label text — producing, for "Nodes (3)" in a 10-wide button:
//
//	[ Nodes  ]
//	  (3)
//
// (label area 6; "Nodes (3)" wraps to "Nodes" + "(3)"). urwid applies no color
// to a plain Button, so the row is drawn in the default text style. The
// SelectableIcon cursor (label position 0) is shown on the first row when
// focused (Phase 0 hardware-cursor parity).
func (b *UrwidButton) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	x, y, w, h := b.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	style := tcell.StyleDefault
	if b.HasFocus() {
		style = tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack)
	}
	setRune := func(col, row int, r rune) {
		if col >= x && col < x+w && row >= y && row < y+h {
			screen.SetContent(col, row, r, nil, style)
		}
	}
	// Label cell width = w - brackets - 2*dividechars (one blank each side).
	labelW := w - len([]rune(b.leftBracket)) - len([]rune(b.rightBracket)) - 2*urwidButtonDivideChars
	if labelW < 0 {
		labelW = 0
	}
	lines := urwidSpaceWrap(b.label, labelW)
	if len(lines) == 0 {
		lines = []string{""}
	}
	for r := 0; r < h; r++ {
		var line string
		if r < len(lines) {
			line = lines[r]
		}
		if sw := stringWidth(line); sw < labelW {
			line += spaces(labelW - sw)
		}
		px := x
		// Left bracket on row 0 only; blank below.
		if r == 0 {
			for _, br := range b.leftBracket {
				setRune(px, y+r, br)
				px++
			}
		} else {
			for range b.leftBracket {
				setRune(px, y+r, ' ')
				px++
			}
		}
		for i := 0; i < urwidButtonDivideChars; i++ {
			setRune(px, y+r, ' ')
			px++
		}
		// Label line (left-justified, padded to labelW).
		for i, ch := range line {
			if i >= labelW {
				break
			}
			setRune(px, y+r, ch)
			px++
		}
		for i := stringWidth(line); i < labelW; i++ {
			setRune(px, y+r, ' ')
			px++
		}
		// Right dividechars blank.
		for i := 0; i < urwidButtonDivideChars; i++ {
			setRune(px, y+r, ' ')
			px++
		}
		// Right bracket on row 0 only; blank below.
		if r == 0 {
			for _, br := range b.rightBracket {
				setRune(px, y+r, br)
				px++
			}
		} else {
			for range b.rightBracket {
				setRune(px, y+r, ' ')
				px++
			}
		}
	}
	// urwid's SelectableIcon places the cursor at label position 0; show it
	// when focused so the hardware cursor matches the original (Phase 0).
	if b.HasFocus() {
		screen.ShowCursor(x+len([]rune(b.leftBracket))+urwidButtonDivideChars, y)
	}
}

// RequiredHeight returns the number of rows the wrapped label needs at width w,
// matching urwid Columns render height (the max child height) for a button
// placed in a column of that width. Used by urwidColumns to size the tab bar.
func (b *UrwidButton) RequiredHeight(w int) int {
	labelW := w - len([]rune(b.leftBracket)) - len([]rune(b.rightBracket)) - 2*urwidButtonDivideChars
	if labelW < 1 {
		return 1
	}
	return len(urwidSpaceWrap(b.label, labelW))
}

// stringWidth returns the display width of s using rune widths (min 1 per
// rune), matching urwid's text width accounting.
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += cellWidth(r)
	}
	return w
}

// spaces returns a string of n space characters.
func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
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
