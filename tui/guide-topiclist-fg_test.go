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

// TestGuideTopicListUnfocusedFg pins V-TopicList-Fg: unfocused Guide topic-list
// items must render with the theme's topic_list_normal foreground, matching
// Python Guide.py:133-135 where each GuideEntry is
// `AttrMap(widget, "topic_list_normal", "list_focus")`. Python's first palette
// block is THEME_DARK (TextUI.py:19), so dark topic_list_normal = #ddd
// (TextUI.py:52) and light = #222 (TextUI.py:105). Both are 3-hex, so urwid
// cube-quantizes them even in truecolor: #ddd→#d7d7d7, #222→#000000. The Go
// port previously left the unfocused items at tview's terminal-default fg
// (ApplyListFocusStyle only sets the SELECTED colors), so the topic list read
// as default text instead of the themed #ddd/#222.
//
// We draw the Guide and probe a NON-current topic row (its item is not the
// focused/selected one, so it uses the main/unfocused text color) and assert
// its cell foreground is topic_list_normal.
func TestGuideTopicListUnfocusedFg(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		theme   int
		wantHex uint32
	}{
		{"dark", ThemeDark, 0xd7d7d7},
		{"light", ThemeLight, 0x000000},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			gd := NewGuideDisplay(app)
			app.Main.SetDisplay("guide", gd.Widget())

			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(135, 32)
			app.Main.Root().SetRect(0, 0, 135, 32)
			app.Main.SelectPage("guide")
			app.Main.Root().Draw(screen)

			// The topic list is the left pane. An UNFOCUSED topic-list row
			// carries topic_list_normal as its foreground and a DEFAULT
			// background; the focused (selected) row instead carries the
			// list_focus background (#aaa cube-quantized to #afafaf).
			// Identify unfocused rows by their default background so the
			// probe works in the light theme too, where topic_list_normal
			// #222 and list_focus fg #111 BOTH cube-quantize to #000000
			// (the same foreground).
			want := tcell.NewHexColor(int32(tc.wantHex))
			focusBG := tcell.NewHexColor(0xafafaf) // cube-quantized #aaa
			found := false
			for y := 2; y < 30 && !found; y++ {
				for x := 2; x < 5; x++ {
					c, combc, style, width := screen.GetContent(x, y)
					_ = combc
					_ = width
					if c == ' ' || c == 0 {
						continue
					}
					fg, bg, _ := style.Decompose()
					// skip the focused (selected) row — it carries list_focus bg.
					if bg == focusBG {
						continue
					}
					if fg == want {
						found = true
						break
					}
					// record the first divergent cell for a helpful message
					if !found {
						t.Errorf("topic-list unfocused cell at (%v,%v) char %q fg=#%06x, want topic_list_normal #%06x",
							x, y, string(c), uint32(fg&0xffffff), tc.wantHex)
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("no topic-list unfocused cell matched topic_list_normal #%06x (theme %v)", tc.wantHex, tc.name)
			}
		})
	}
}
