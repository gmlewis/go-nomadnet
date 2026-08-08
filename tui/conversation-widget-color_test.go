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

// TestConversationWidgetPaletteColors pins the conversation widget's
// editor + peer-info-bar + message-list base colors to the Python palette.
// Python wraps the editor in `AttrMap(editor, "msg_editor")` and the peer-info
// bar in `AttrMap(Text, "msg_header_sent")` (Conversations.py:602,609); the
// messagelist is a bare `IndicativeListBox` with NO AttrMap (Conversations.py:
// 2287) so its base is the terminal default. The palette (ui/TextUI.py):
//
//	msg_header_sent  = #111 / #ddd   (both themes, lines 35 & 88)
//	msg_editor       = #111 / #0bb   (both themes, lines 32 & 85)
//
// Both are 3-hex, so urwid cube-quantizes them even in truecolor: #111→#000000,
// #ddd→#d7d7d7, #0bb→#00afaf. The Go port previously used nibble-doubled / wrong
// literals: the editor 0x222222/0xdddddd (Python never uses #222/#ddd for the
// editor — it is #0bb/#111) and the peer-info bar 0x111111/0xdddddd (exact, not
// the cube-quantized #000000/#d7d7d7 Python emits). The message-list base was
// 0xbbbbbb where Python uses default.
func TestConversationWidgetPaletteColors(t *testing.T) {
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
			cw := NewConversationWidget(app, "") // plain "No conversation selected"

			// Editor field colors must be the cube-quantized msg_editor palette:
			// fg #111→#000000, bg #0bb→#00afaf. (tview InputField.SetField*
			// update the same style GetFieldStyle returns.)
			for _, ed := range []*ReadlineEdit{cw.editor, cw.titleEditor} {
				fg, bg, _ := ed.GetFieldStyle().Decompose()
				if uint32(fg.Hex())&0xffffff != 0x000000 {
					t.Errorf("editor field fg = #%06x, want #000000 (msg_editor fg #111 cube-quantized)", uint32(fg.Hex())&0xffffff)
				}
				if uint32(bg.Hex())&0xffffff != 0x00afaf {
					t.Errorf("editor field bg = #%06x, want #00afaf (msg_editor bg #0bb cube-quantized)", uint32(bg.Hex())&0xffffff)
				}
			}

			// Peer-info bar base must be the cube-quantized msg_header_sent
			// palette: fg #111→#000000, bg #ddd→#d7d7d7. Draw the plain-text
			// header (source=="" → "No conversation selected", no color tags)
			// and probe a non-blank glyph cell.
			piScreen := tcell.NewSimulationScreen("UTF-8")
			if err := piScreen.Init(); err != nil {
				t.Fatalf("peerInfo screen.Init: %v", err)
			}
			defer piScreen.Fini()
			piScreen.SetSize(60, 1)
			cw.peerInfoBar.SetRect(0, 0, 60, 1)
			cw.peerInfoBar.Draw(piScreen)
			if c, _, style, _ := piScreen.GetContent(1, 0); c == ' ' || c == 0 {
				t.Fatalf("peerInfoBar cell (1,0) is blank; cannot probe base color")
			} else {
				fg, bg, _ := style.Decompose()
				if uint32(fg.Hex())&0xffffff != 0x000000 {
					t.Errorf("peerInfoBar fg = #%06x, want #000000 (msg_header_sent fg #111 cube-quantized)", uint32(fg.Hex())&0xffffff)
				}
				if uint32(bg.Hex())&0xffffff != 0xd7d7d7 {
					t.Errorf("peerInfoBar bg = #%06x, want #d7d7d7 (msg_header_sent bg #ddd cube-quantized)", uint32(bg.Hex())&0xffffff)
				}
			}

			// Message-list base must be the terminal default (Python's
			// messagelist is a bare IndicativeListBox with no AttrMap).
			mlScreen := tcell.NewSimulationScreen("UTF-8")
			if err := mlScreen.Init(); err != nil {
				t.Fatalf("messageList screen.Init: %v", err)
			}
			defer mlScreen.Fini()
			mlScreen.SetSize(10, 1)
			cw.messageList.SetText("X")
			cw.messageList.SetRect(0, 0, 10, 1)
			cw.messageList.Draw(mlScreen)
			if c, _, style, _ := mlScreen.GetContent(0, 0); c != 'X' {
				t.Fatalf("messageList cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if fg != tcell.ColorDefault {
					t.Errorf("messageList base fg = %v, want ColorDefault (Python messagelist has no AttrMap)", fg)
				}
			}
		})
	}
}
