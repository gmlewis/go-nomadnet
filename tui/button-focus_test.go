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
	"testing"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// TestUrwidButtonFocusIsACursorNotAColor pins how a focused button looks.
// Python's paper_message builds plain urwid.Button values
// (Conversations.py:2542-2548) and a plain urwid Button applies no attribute —
// the nomadnet "button_normal"/"button_focus" palette entries reach buttons only
// where they are wrapped in an AttrMap, which these are not. urwid shows focus as
// a HARDWARE CURSOR on the label's first cell (SelectableIcon cursor position 0).
//
// So there is no color difference between focused and unfocused: the same cells
// carry the same style in both states, and the focus indicator is the cursor.
func TestUrwidButtonFocusIsACursorNotAColor(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 5)

	button := NewUrwidButton("Save QR")
	button.SetRect(2, 1, 20, 1)

	// Unfocused: painted, and no cursor on the label.
	button.Blur()
	button.Draw(screen)
	if _, _, visible := screen.GetCursor(); visible {
		t.Error("an unfocused button left the hardware cursor visible")
	}
	_, unfocusedStyle, _ := screen.Get(3, 1) // first label cell

	// Focused: the same cells, the same style, and the cursor on the label's first
	// cell — x + len("<") + dividechars.
	button.Focus(func(p tview.Primitive) {})
	button.Draw(screen)
	x, y, visible := screen.GetCursor()
	if !visible {
		t.Fatal("a focused button did not show the hardware cursor")
	}
	if wantX, wantY := 2+len("<")+urwidButtonDivideChars, 1; x != wantX || y != wantY {
		t.Errorf("focus cursor at (%v,%v), want (%v,%v) — urwid's SelectableIcon cursor position 0",
			x, y, wantX, wantY)
	}
	_, focusedStyle, _ := screen.Get(3, 1)
	if focusedStyle != unfocusedStyle {
		t.Errorf("the label cell's style changed with focus (%v vs %v); urwid's plain Button applies no attribute",
			focusedStyle, unfocusedStyle)
	}
}

// TestUrwidButtonRendersTheUrwidForm pins the row itself: "< label >" with one blank
// either side of the label, padded to the box width — urwid's Button with its default
// dividechars (1) and no border, which tview.Button would not produce (it draws a
// bordered box).
func TestUrwidButtonRendersTheUrwidForm(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(40, 5)

	button := NewUrwidButton("Save QR")
	button.SetRect(0, 0, 12, 1)
	button.Draw(screen)

	var row string
	for x := range 12 {
		str, _, _ := screen.Get(x, 0)
		row += str
	}
	if got, want := row, "< Save QR  >"; got != want {
		t.Errorf("button row = %q, want %q", got, want)
	}
}
