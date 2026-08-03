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
	"strings"
	"testing"
)

// TestMenuButtonsUniformMenubarStyle asserts that every menu button renders
// with the uniform `menubar` style, regardless of which button is the active
// page / focused button. Golden (Python nomadnet/ui/textui/Main.py:211):
//
//	self.widget = urwid.AttrMap(columns, "menubar")
//
// AttrMap is applied to the whole MenuColumns with NO focus_map, so each
// `[ Name ]` button — focused or not, active page or not — renders the
// `menubar` style (`#111`/`#bbb`, no bold). The active/focused button is
// indicated to the user ONLY by the hardware cursor (screen.ShowCursor),
// never by a per-button color or bold change.
//
// Regression: the Go port had been wrapping the active button in
// `[#111111:aaaaaa:b]…[-:-:-]` (bold `#111`/`#aaa` = list_focus), which Python
// does not emit (confirmed in python_session.cast: every menu button carries
// the same SGR, including the active page's).
func TestMenuButtonsUniformMenubarStyle(t *testing.T) {
	t.Parallel()

	// Both themes define menubar_bg=#bbbbbb and list_focus_bg=#aaaaaa
	// (tui/theme.go). The menubar fg is #111111 in both themes.
	const menubarBg = "bbbbbb"
	const listFocusBg = "aaaaaa"

	for _, theme := range []int{ThemeDark, ThemeLight} {
		app := newTestApp()
		md := NewMainDisplay(app, theme, GlyphUnicode)

		for active := 0; active < len(md.menuItems); active++ {
			md.activeMenu = active
			md.activePage = md.menuItems[active].Key
			md.redrawMenuBar()

			styled := md.menuBar.GetText(false) // with color tags

			// No button may be bold. A bold button tag looks like ":b]".
			if strings.Contains(styled, ":b]") {
				t.Errorf("theme %v active %v: menu bar contains a bold button tag ':b]' (Python never bolds menu buttons):\n%v",
					theme, active, styled)
			}

			// No button may use the list_focus background.
			if strings.Contains(styled, ":"+listFocusBg) {
				t.Errorf("theme %v active %v: menu bar uses list_focus_bg %q (no button should):\n%v",
					theme, active, listFocusBg, styled)
			}

			// Every button's preceding color tag must use the menubar background.
			for _, item := range md.menuItems {
				btn := "[ " + item.Label + " ]"
				idx := strings.Index(styled, btn)
				if idx < 0 {
					t.Errorf("theme %v active %v: menu bar missing button %q", theme, active, btn)
					continue
				}
				tagStart := strings.LastIndex(styled[:idx], "[")
				if tagStart < 0 {
					t.Errorf("theme %v active %v: button %q has no preceding color tag", theme, active, btn)
					continue
				}
				tag := styled[tagStart:idx]
				if !strings.Contains(tag, menubarBg) {
					t.Errorf("theme %v active %v: button %q color tag %q does not use menubar_bg %q:\n%v",
						theme, active, btn, tag, menubarBg, styled)
				}
			}
		}
	}
}
