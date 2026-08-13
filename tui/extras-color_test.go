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

// TestIntroDisplayDefaultColor pins the splash/intro color parity fix.
// Python's Extras.py renders the intro title as urwid.BigText with the palette
// name "intro_title", which is NOT defined in nomadnet's palette, so urwid
// falls back to its default style (terminal-default fg/bg — no forced color).
// The Go port must replicate that with tcell.ColorDefault rather than tview's
// forced PrimaryTextColor (white), otherwise the splash renders bright white
// while Python renders in the terminal default color. All three intro views
// (big text, version, starting) must draw their non-blank cells with
// tcell.ColorDefault as the foreground.
func TestIntroDisplayDefaultColor(t *testing.T) {
	t.Parallel()

	id := NewIntroDisplay("nomadnet", "0.0.0")

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("failed to init simulation screen: %v", err)
	}

	// Draw each intro view across the full width so a non-blank glyph cell is
	// guaranteed to land inside the rect, then assert every non-blank cell it
	// emits uses tcell.ColorDefault as its foreground (terminal default).
	for _, view := range []struct {
		name string
		w    interface {
			SetRect(int, int, int, int)
			Draw(tcell.Screen)
		}
		h int
	}{
		{"bigView", id.bigView, 4},
		{"versionView", id.versionView, 1},
		{"startingView", id.startingView, 1},
	} {
		screen.SetSize(60, view.h)
		view.w.SetRect(0, 0, 60, view.h)
		view.w.Draw(screen)

		found := false
		for y := 0; y < view.h; y++ {
			for x := range 60 {
				mainc, _, style, width := cellContent(screen, x, y)
				if width == 0 || mainc == ' ' {
					continue
				}
				found = true
				fg, _, _ := style.Decompose()
				if fg != tcell.ColorDefault {
					t.Errorf("%v cell (%v,%v) %q foreground = %v, want tcell.ColorDefault",
						view.name, x, y, string(mainc), fg)
				}
			}
		}
		if !found {
			t.Fatalf("%v drew no non-blank cells; test setup is wrong", view.name)
		}
	}
}
