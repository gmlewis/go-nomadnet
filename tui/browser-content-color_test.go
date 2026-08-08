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

// TestBrowserContentBaseColor pins the browser content area's base (fallback)
// text color to the cube-quantized body_text palette entry. Python wraps the
// browser display widget in `AttrMap(..., "body_text")` (Browser.py:562);
// body_text is 3-hex #ddd (dark) / #222 (light) (ui/TextUI.py:26,80), which urwid
// cube-quantizes even in truecolor: #ddd→#d7d7d7, #222→#000000. The base color
// only colors blank/padding cells and the non-micron placeholder/loading text:
// micron plain runs carry an explicit #dddddd tag (Go micron DefaultFG, like
// Python's high_color nibble-doubling), so they do not inherit SetTextColor.
// The Go port previously used a hardcoded 0xbbbbbb, which Python never emits.
func TestBrowserContentBaseColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		want  uint32
	}{
		{"dark", ThemeDark, 0xd7d7d7},
		{"light", ThemeLight, 0x000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			bd := NewBrowserDisplay(app)

			// Probe the base color via a draw: set a single untagged glyph so the
			// cell is painted with the TextView's SetTextColor (tview TextView has
			// no GetTextColor). An empty text leaves cells unpainted (the
			// SimulationScreen default shows through), so a real glyph is needed.
			bd.content.SetText("X")
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 3)
			bd.content.SetRect(0, 0, 40, 3)
			bd.content.Draw(screen)
			if c, _, style, _ := screen.GetContent(0, 0); c != 'X' {
				t.Fatalf("content cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if got := uint32(fg.Hex()) & 0xffffff; got != tc.want {
					t.Errorf("content base fg = #%06x, want #%06x (body_text cube-quantized)", got, tc.want)
				}
			}
		})
	}
}
