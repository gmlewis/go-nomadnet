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

// TestChannelsDisplayPaletteColors pins the Channels compose editor + message
// view base colors to the Python palette. Python wraps the room editor in
// `AttrMap(editor, "msg_editor")` (Channels.py:609) and builds the message list
// as a bare `_StickyMessageListBox` with NO AttrMap (Channels.py:784), so its
// base is the terminal default. msg_editor is 3-hex #111/#0bb (ui/TextUI.py:32
// /85), cube-quantized to #000000/#00afaf. The Go port previously used
// 0x222222/0xdddddd for the editor (not Python's msg_editor) and 0xbbbbbb for
// the message view base (Python uses default).
func TestChannelsDisplayPaletteColors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
	}{
		{"dark", ThemeDark},
		{"light", ThemeLight},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			cd := NewChannelsDisplay(app, nil)

			// Compose editor field colors = cube-quantized msg_editor:
			// fg #111→#000000, bg #0bb→#00afaf.
			fg, bg, _ := cd.input.GetFieldStyle().Decompose()
			if uint32(fg.Hex())&0xffffff != 0x000000 {
				t.Errorf("channels input fg = #%06x, want #000000 (msg_editor fg #111 cube-quantized)", uint32(fg.Hex())&0xffffff)
			}
			if uint32(bg.Hex())&0xffffff != 0x00afaf {
				t.Errorf("channels input bg = #%06x, want #00afaf (msg_editor bg #0bb cube-quantized)", uint32(bg.Hex())&0xffffff)
			}

			// Message view base = terminal default (Python's _StickyMessageListBox
			// has no AttrMap).
			mlScreen := tcell.NewSimulationScreen("UTF-8")
			if err := mlScreen.Init(); err != nil {
				t.Fatalf("messages screen.Init: %v", err)
			}
			defer mlScreen.Fini()
			mlScreen.SetSize(10, 1)
			cd.messages.SetText("X")
			cd.messages.SetRect(0, 0, 10, 1)
			cd.messages.Draw(mlScreen)
			if c, _, style, _ := mlScreen.GetContent(0, 0); c != 'X' {
				t.Fatalf("messages cell (0,0) = %q, want 'X'", string(c))
			} else {
				mfg, _, _ := style.Decompose()
				if mfg != tcell.ColorDefault {
					t.Errorf("messages base fg = %v, want ColorDefault (Python _StickyMessageListBox has no AttrMap)", mfg)
				}
			}
		})
	}
}
