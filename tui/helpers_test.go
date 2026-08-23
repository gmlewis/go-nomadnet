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

	"github.com/gdamore/tcell/v2"
)

// cellContent returns the primary rune, combining runes, style, and display
// width of the cell at (x, y) on s. It is a test-only stand-in for the
// deprecated tcell Screen.GetContent: upstream now implements GetContent in
// terms of Screen.Get, splitting Get's result string into a leading primary
// rune and the remaining combining runes. Mirroring that here keeps the suite
// free of the SA1019 deprecation warning without altering any rune-level
// assertion (the return signature is identical to GetContent).
func cellContent(s tcell.Screen, x, y int) (rune, []rune, tcell.Style, int) {
	str, style, width := s.Get(x, y)
	var primary rune
	var combining []rune
	for i, r := range str {
		if i == 0 {
			primary = r
		} else {
			combining = append(combining, r)
		}
	}
	return primary, combining, style, width
}

func TestNewClickableIcon(t *testing.T) {
	t.Parallel()

	ci := NewClickableIcon("📋", nil)
	if ci == nil {
		t.Fatal("NewClickableIcon returned nil")
	}
}

func TestClickableIconClickFiresCallback(t *testing.T) {
	t.Parallel()

	var clicked bool
	ci := NewClickableIcon("📋", func() { clicked = true })

	ci.HandleMouseLeftClick()

	if !clicked {
		t.Error("ClickableIcon should fire callback on left click")
	}
}

func TestClickableIconClickNilCallback(t *testing.T) {
	t.Parallel()

	ci := NewClickableIcon("📋", nil)

	ci.HandleMouseLeftClick()
}

func TestClickableIconClickWithText(t *testing.T) {
	t.Parallel()

	var copied string
	ci := NewClickableIcon("📋", func() { copied = "test data" })

	ci.HandleMouseLeftClick()

	if copied != "test data" {
		t.Errorf("callback result = %q, want %q", copied, "test data")
	}
}

// TestClickableIconGlyphColor pins the ClickableIcon glyph color to the
// browser_controls palette value (#bbb dark, 3-hex cube-quantized to
// #afafaf). Python's ClickableIcon is a bare urwid.Text inside an
// AttrMap("browser_controls") so it inherits #bbb. The Go port previously
// used 0xdddddd which Python never emits.
//
// Python source: Browser.py:516-521 (ClickableIcon inside
// AttrMap(..., "browser_controls")); TextUI.py:53 (browser_controls = #bbb).
func TestClickableIconGlyphColor(t *testing.T) {
	t.Parallel()

	ci := NewClickableIcon("X", nil)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(10, 3)
	ci.SetRect(0, 0, 10, 3)
	ci.Draw(screen)

	if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
		t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if got := uint32(fg.Hex()) & 0xffffff; got != 0xafafaf {
			t.Errorf("ClickableIcon glyph fg = #%06x, want #afafaf "+
				"(browser_controls #bbb cube-quantized)", got)
		}
	}
}

func TestOSC52CopyEmpty(t *testing.T) {
	t.Parallel()

	err := OSC52Copy("")
	if err != nil {
		t.Errorf("OSC52Copy empty string should not error: %v", err)
	}
}
