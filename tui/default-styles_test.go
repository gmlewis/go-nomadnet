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
	"github.com/rivo/tview"
)

// TestTerminalDefaultBackgroundAndBorders asserts the port overrides tview's
// library-global Styles so that pane backgrounds, border cells, and border
// titles use the TERMINAL DEFAULT color (transparent / `ColorDefault`), matching
// the Python original. Golden (python_session.cast): every border line and
// undrawn pane cell is emitted with `\x1b[0;39;49m` (default fg, default bg) —
// urwid never forces a black background or a white border. tview's stock
// defaults (`PrimitiveBackgroundColor=ColorBlack`, `BorderColor=ColorWhite`,
// box.go borderStyle = white-on-black) produce `\x1b[38;2;255;255;255;
// 48;2;0;0;0` borders — a hard black box that is wrong on a light terminal.
//
// The fix mutates the library-global `tview.Styles` (like ApplySingleLineBorders
// mutates `tview.Borders`); it is idempotent and safe to call repeatedly.
func TestTerminalDefaultBackgroundAndBorders(t *testing.T) {
	t.Parallel()

	newTestApp() // invokes NewApp -> ApplyDefaultStyles

	if tview.Styles.PrimitiveBackgroundColor != tcell.ColorDefault {
		t.Errorf("tview.Styles.PrimitiveBackgroundColor = %v, want ColorDefault (transparent, Python default bg)",
			tview.Styles.PrimitiveBackgroundColor)
	}
	if tview.Styles.BorderColor != tcell.ColorDefault {
		t.Errorf("tview.Styles.BorderColor = %v, want ColorDefault (Python border fg = default)",
			tview.Styles.BorderColor)
	}
	if tview.Styles.TitleColor != tcell.ColorDefault {
		t.Errorf("tview.Styles.TitleColor = %v, want ColorDefault (Python border title fg = default)",
			tview.Styles.TitleColor)
	}
	if tview.Styles.GraphicsColor != tcell.ColorDefault {
		t.Errorf("tview.Styles.GraphicsColor = %v, want ColorDefault", tview.Styles.GraphicsColor)
	}

	// Render a plain bordered box and confirm its border cell carries the
	// terminal-default background (transparent), not black.
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	screen.SetSize(20, 5)

	box := tview.NewBox().SetBorder(true)
	box.SetRect(0, 0, 20, 5)
	box.Draw(screen)

	_, _, style, _ := cellContent(screen, 0, 0) // top-left border corner
	fg, bg, _ := style.Decompose()
	if bg != tcell.ColorDefault {
		t.Errorf("border cell bg = %v, want ColorDefault (Python \\x1b[...49m)", bg)
	}
	if fg != tcell.ColorDefault {
		t.Errorf("border cell fg = %v, want ColorDefault (Python \\x1b[...39m)", fg)
	}
}
