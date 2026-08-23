// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
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

// TestHubInfoBaseColor pins the HubInfoArea's base text color to the
// "scrollbar" palette entry. Python wraps the hub info body in
// `urwid.AttrMap(body, "scrollbar")` (Channels.py:1827), so bare label rows
// inherit scrollbar's fg = #444 (dark, 3-hex cube-quantized to #5f5f5f) /
// #444 (light). The Go port previously used a hardcoded 0xbbbbbb which
// Python never emits.
//
// Python source: Channels.py:1827
// `info = HubInfoArea(urwid.AttrMap(body, "scrollbar"), title=hub.name)`.
func TestHubInfoBaseColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		want  uint32
	}{
		{"dark", ThemeDark, 0x5f5f5f},
		{"light", ThemeLight, 0x5f5f5f},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			hia := NewHubInfoArea(app, "TestHub")
			hia.SetMOTD("hello")

			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 5)
			hia.view.SetRect(0, 0, 40, 5)
			hia.view.Draw(screen)

			// Probe the first text glyph in the view (the "M" of "MOTD:").
			// The TextView's SetTextColor applies to untagged text; the
			// "[::b]" tag only toggles bold, not color.
			r, _, style, _ := cellContent(screen, 0, 0)
			if r != 'M' {
				t.Fatalf("cell (0,0) = %q, want 'M'", string(r))
			}
			fg, _, _ := style.Decompose()
			if got := uint32(fg.Hex()) & 0xffffff; got != tc.want {
				t.Errorf("hub-info base fg = #%06x, want #%06x (scrollbar "+
					"#444 cube-quantized)", got, tc.want)
			}
		})
	}
}
