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
)

// TestComposeDisplayPaletteColors pins the compose (new-message) title + body
// editor field colors to the cube-quantized msg_editor palette. Python wraps
// BOTH the title editor and the message editor in `AttrMap(..., "msg_editor")`
// (Conversations.py:1921, 1926). msg_editor is 3-hex #111/#0bb (ui/TextUI.py:32
// /85), cube-quantized to #000000/#00afaf. The Go port previously used
// 0x222222/0xdddddd, which is not Python's msg_editor.
func TestComposeDisplayPaletteColors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
	}{
		{"dark", ThemeDark},
		{"light", ThemeLight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			cd := NewComposeDisplay(app)

			for _, ed := range []*ReadlineEdit{cd.title, cd.editor} {
				fg, bg, _ := ed.GetFieldStyle().Decompose()
				if uint32(fg.Hex())&0xffffff != 0x000000 {
					t.Errorf("compose editor fg = #%06x, want #000000 (msg_editor fg #111 cube-quantized)", uint32(fg.Hex())&0xffffff)
				}
				if uint32(bg.Hex())&0xffffff != 0x00afaf {
					t.Errorf("compose editor bg = #%06x, want #00afaf (msg_editor bg #0bb cube-quantized)", uint32(bg.Hex())&0xffffff)
				}
			}
		})
	}
}
