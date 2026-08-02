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
// You should have receive a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// drawUrwidButtonAt renders b on a fresh simulation screen of width w and
// returns the joined cell text of its single row.
func drawUrwidButtonAt(t *testing.T, b *UrwidButton, w int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, 1)
	b.SetRect(0, 0, w, 1)
	b.Draw(screen)
	screen.Sync()
	var bu strings.Builder
	for x := 0; x < w; x++ {
		c, _, _, _ := screen.GetContent(x, 0)
		bu.WriteRune(c)
	}
	return bu.String()
}

// TestUrwidButtonRenderFlat pins urwid's flat button rendering: "< label >"
// with the brackets at the edges and the label left-justified, padded to fill
// the given box width (urwid Button = button_left "<" + dividechars space +
// SelectableIcon label + dividechars space + button_right ">", the label
// absorbing the remaining Columns width). There is NO box border.
func TestUrwidButtonRenderFlat(t *testing.T) {
	t.Parallel()
	wantButton := func(label string, w int) string {
		labelW := w - 2 - 2*urwidButtonDivideChars // brackets + dividechars blanks
		padded := label
		if len([]rune(padded)) < labelW {
			padded += strings.Repeat(" ", labelW-len([]rune(padded)))
		}
		return urwidButtonLeft + " " + padded + " " + urwidButtonRight
	}
	b := NewUrwidButton("Create")
	// Width 20: "<" + " " + label padded to 16 + " " + ">" (brackets at edges).
	got := drawUrwidButtonAt(t, b, 20)
	want := wantButton("Create", 20)
	if got != want {
		t.Errorf("render width 20 = %q, want %q", got, want)
	}
	// Minimum-ish width: "< Back >" needs 8 cells (len("Back")+4).
	b2 := NewUrwidButton("Back")
	if got := drawUrwidButtonAt(t, b2, 8); got != "< Back >" {
		t.Errorf("render width 8 = %q, want %q", got, "< Back >")
	}
}

// TestUrwidButtonActivate verifies Space/Enter fires the selected callback and
// that other runes are ignored (urwid Button maps " " and "enter" to activate).
func TestUrwidButtonActivate(t *testing.T) {
	t.Parallel()
	fired := 0
	b := NewUrwidButton("OK").SetSelectedFunc(func() { fired++ })
	h := b.InputHandler()
	h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	h(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) {})
	if fired != 2 {
		t.Errorf("fired %d times after Enter+Space, want 2", fired)
	}
	// A non-space rune is ignored.
	before := fired
	h(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone), func(p tview.Primitive) {})
	if fired != before {
		t.Errorf("rune 'x' fired the button, want ignored")
	}
}

// TestUrwidButtonClick verifies a left mouse click fires the selected callback.
func TestUrwidButtonClick(t *testing.T) {
	t.Parallel()
	fired := false
	b := NewUrwidButton("Create").SetSelectedFunc(func() { fired = true })
	b.SetRect(0, 0, 20, 1)
	mh := b.MouseHandler()
	consumed, _ := mh(tview.MouseLeftClick,
		tcell.NewEventMouse(2, 0, tcell.Button1, 0), // click on "C" of "Create"
		func(p tview.Primitive) {})
	if !consumed || !fired {
		t.Errorf("left click: consumed=%v fired=%v, want true/true", consumed, fired)
	}
}

// TestTabButtonRenderFlat pins the TabButton rendering: "[ label ]" — identical
// to UrwidButton but with "[" / "]" brackets (Python TabButton, Conversations.py
// :82-84, is a urwid.Button subclass overriding button_left="[", button_right=
// "]"). The label is left-justified and padded so "]" sits at the right edge,
// matching the original's Columns layout (each tab is weight 1 in the tab_bar
// Columns with dividechars=1, Conversations.py:395-398).
func TestTabButtonRenderFlat(t *testing.T) {
	t.Parallel()
	wantTab := func(label string, w int) string {
		labelW := w - 2 - 2*urwidButtonDivideChars
		padded := label
		if len([]rune(padded)) < labelW {
			padded += strings.Repeat(" ", labelW-len([]rune(padded)))
		}
		return "[" + " " + padded + " " + "]"
	}
	// "Trusted (0)" (11 chars) in a 25-wide tab → "[ Trusted (0)           ]".
	b := NewTabButton("Trusted (0)")
	if got := drawUrwidButtonAt(t, b, 25); got != wantTab("Trusted (0)", 25) {
		t.Errorf("tab render width 25 = %q, want %q", got, wantTab("Trusted (0)", 25))
	}
	// "Untrusted (0)" (13 chars) in a 24-wide tab → "[ Untrusted (0)        ]".
	b2 := NewTabButton("Untrusted (0)")
	if got := drawUrwidButtonAt(t, b2, 24); got != wantTab("Untrusted (0)", 24) {
		t.Errorf("tab render width 24 = %q, want %q", got, wantTab("Untrusted (0)", 24))
	}
}

// TestTabButtonActivate verifies the TabButton fires on Enter/Space/click like
// the flat button (it drives the Conversations tab filter switch).
func TestTabButtonActivate(t *testing.T) {
	t.Parallel()
	fired := 0
	b := NewTabButton("Untrusted (0)").SetSelectedFunc(func() { fired++ })
	h := b.InputHandler()
	h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	h(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) {})
	if fired != 2 {
		t.Errorf("tab fired %d times after Enter+Space, want 2", fired)
	}
}

func TestUrwidButtonFocusedStyle(t *testing.T) {
	t.Parallel()
	b := NewUrwidButton("Create")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(12, 1)
	b.SetRect(0, 0, 12, 1)
	b.Focus(func(p tview.Primitive) {})
	b.Draw(screen)
	screen.Sync()

	_, _, style, _ := screen.GetContent(0, 0)
	fg, bg, _ := style.Decompose()
	if bg != tcell.ColorGreen || fg != tcell.ColorBlack {
		t.Errorf("focused button style = fg:%v bg:%v, want fg:Black bg:Green", fg, bg)
	}
}

