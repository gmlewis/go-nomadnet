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

// TestGuideReaderBaseColor pins the Guide reader's base (fallback) foreground
// to the micron "plain" style default. Python MicronParser renders every body
// part with a style (make_style); the plain default fg is DEFAULT_FG_DARK="ddd"
// / DEFAULT_FG_LIGHT="222" (MicronParser.py:13-14). make_style's high_color
// nibble-doubles the 3-hex to a 6-hex "#rrggbb" (MicronParser.py ~555
// `return "#"+r+r+g+g+b+b`), and urwid _parse_color_true parses 7-char
// "#rrggbb" EXACT — so Python renders the Guide body as #dddddd (dark) /
// #222222 (light), and NEVER #bbbbbb.
//
// The Go styled spans carry that same #dddddd/#222222 tag, but tview's
// TextView base color (SetTextColor — the fallback for `[-:-:-]` resets and
// the plain indent/padding spaces StyledLinesToTviewText writes BEFORE the
// first span) must ALSO be #dddddd/#222222, else those untagged runs diverge
// to a third color. The port previously set 0xbbbbbb, which Python never
// emits and which surfaced on boot/Guide captures as an extra #bbbbbb
// foreground. We render topic 0 and probe BLANK cells (indent/padding), whose
// foreground is the base color (styled spans only color non-blank glyphs).
func TestGuideReaderBaseColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		want  uint32
	}{
		{"dark", ThemeDark, 0xdddddd},
		{"light", ThemeLight, 0x222222},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			gd := NewGuideDisplay(app)
			gd.showTopic(0)

			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(100, 24)
			gd.reader.SetRect(0, 0, 100, 24)
			gd.reader.Draw(screen)

			want := tcell.NewHexColor(int32(tc.want))
			wrong := tcell.NewHexColor(0xbbbbbb)
			var foundBase, foundWrong bool
			for y := range 24 {
				for x := range 100 {
					c, _, style, _ := cellContent(screen, x, y)
					if c != ' ' && c != 0 {
						continue
					}
					fg, _, _ := style.Decompose()
					if fg == want {
						foundBase = true
					}
					if fg == wrong {
						foundWrong = true
					}
				}
			}
			if foundWrong {
				t.Errorf("Guide reader has a blank cell with the wrong base fg #bbbbbb (Python never emits #bbbbbb in the Guide body)")
			}
			if !foundBase {
				t.Errorf("Guide reader has no blank cell at the base fg #%06x (micron plain DEFAULT_FG nibble-doubled to 6-hex, urwid-exact)",
					tc.want)
			}
		})
	}
}
