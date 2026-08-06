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

// TestLogDisplayBaseColor pins the Log page base text color to the terminal
// default. Python's LogTerminal embeds `urwid.Terminal` running `tail -f`,
// wrapped only in a `urwid.LineBox` with NO AttrMap (Log.py:44-51), so the
// terminal paints with its own default colors — there is no palette text fg.
// The Go port previously used 0xbbbbbb.
func TestLogDisplayBaseColor(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	ld := NewLogDisplay(app, "/nonexistent-log-path-for-test", 10)

	ld.logView.SetText("X")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(40, 3)
	ld.logView.SetRect(0, 0, 40, 3)
	ld.logView.Draw(screen)
	if c, _, style, _ := screen.GetContent(0, 0); c != 'X' {
		t.Fatalf("logView cell (0,0) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if fg != tcell.ColorDefault {
			t.Errorf("logView base fg = %v, want ColorDefault (Python LogTerminal has no AttrMap)", fg)
		}
	}
}
