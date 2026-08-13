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

// TestRoomWidgetPaletteColors pins the chat room widget's message-body base
// color and editor field colors to the Python palette.
//
// Messages: Go renders each message as `[#66cc55]<nick>[-] <text>` (room-
// widget.go renderMessages), so the message body text carries NO color tag and
// inherits the TextView's SetTextColor. Python colors the message body with the
// `body_text` palette attr (Channels.py:1333 `_body_markup(body,
// body_attr="body_text")`); body_text is 3-hex #ddd (dark) / #222 (light)
// (ui/TextUI.py:26,80), cube-quantized to #d7d7d7 / #000000. The Go port
// previously used 0xbbbbbb.
//
// Editor: Python wraps the room message editor in `AttrMap(editor,
// "msg_editor")` (Channels.py:609). msg_editor is #111/#0bb (both themes,
// ui/TextUI.py:32,85), cube-quantized to #000000/#00afaf. The Go port
// previously used 0x222222/0xdddddd.
func TestRoomWidgetPaletteColors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		theme   int
		msgWant uint32
	}{
		{"dark", ThemeDark, 0xd7d7d7},
		{"light", ThemeLight, 0x000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			rw := NewRoomWidget(app, "hub", "room")

			// Message-body base color: probe a painted untagged glyph, which
			// inherits SetTextColor (tview TextView has no GetTextColor).
			rw.messages.SetText("X")
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 3)
			rw.messages.SetRect(0, 0, 40, 3)
			rw.messages.Draw(screen)
			if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
				t.Fatalf("messages cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if got := uint32(fg.Hex()) & 0xffffff; got != tc.msgWant {
					t.Errorf("messages base fg = #%06x, want #%06x (body_text cube-quantized)", got, tc.msgWant)
				}
			}

			// Editor field colors: msg_editor fg #111→#000000, bg #0bb→#00afaf
			// (both themes). tview InputField.SetField* update the style
			// GetFieldStyle returns.
			fg, bg, _ := rw.editor.GetFieldStyle().Decompose()
			if got := uint32(fg.Hex()) & 0xffffff; got != 0x000000 {
				t.Errorf("editor field fg = #%06x, want #000000 (msg_editor fg #111 cube-quantized)", got)
			}
			if got := uint32(bg.Hex()) & 0xffffff; got != 0x00afaf {
				t.Errorf("editor field bg = #%06x, want #00afaf (msg_editor bg #0bb cube-quantized)", got)
			}
		})
	}
}
