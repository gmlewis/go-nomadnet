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

// TestNamedColorEntries pins the theme.go entries that use urwid named colors
// (not 3-hex) to the correct tcell named-color constants. Python's palette
// (TextUI.py) uses urwid named colors like "dark red", "dark gray", "dark
// green" in the high-color column; urwid renders these as 16-color ANSI codes
// even in truecolor mode (e.g. "dark red" → SGR 31). The tcell equivalents
// (ColorMaroon=0x800000, ColorGray=0x808080, ColorGreen=0x008000) also emit
// 16-color codes. The Go port previously used wrong hex values (0xaa2222,
// 0x666666, 0x66bb22, 0xff0000) that match neither the named color NOR any
// 3-hex palette entry.
//
// Python source: TextUI.py:28-62 (dark), 140-172 (light).
func TestNamedColorEntries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		key   string
		want  tcell.Color
	}{
		// Dark theme named colors
		{"dark/error_text", ThemeDark, "error_text", tcell.ColorMaroon},
		{"dark/inactive_text", ThemeDark, "inactive_text", tcell.ColorGray},
		{"dark/connected_status", ThemeDark, "connected_status", tcell.ColorGreen},
		{"dark/disconnected_status", ThemeDark, "disconnected_status", tcell.ColorMaroon},
		{"dark/placeholder", ThemeDark, "placeholder", tcell.ColorGray},
		{"dark/placeholder_text", ThemeDark, "placeholder_text", tcell.ColorGray},
		{"dark/interface_title", ThemeDark, "interface_title", tcell.ColorDefault},
		{"dark/interface_title_selected", ThemeDark, "interface_title_selected", tcell.ColorDefault},
		// Light theme named colors (some differ from dark)
		{"light/error_text", ThemeLight, "error_text", tcell.ColorMaroon},
		{"light/inactive_text", ThemeLight, "inactive_text", tcell.ColorGray},
		{"light/connected_status", ThemeLight, "connected_status", cubeHex3("#4a0")},
		{"light/disconnected_status", ThemeLight, "disconnected_status", cubeHex3("#a22")},
		{"light/placeholder", ThemeLight, "placeholder", cubeHex3("#999")},
		{"light/placeholder_text", ThemeLight, "placeholder_text", cubeHex3("#999")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			colors := GetThemeColors(tc.theme)
			got := colors[tc.key]
			if got != tc.want {
				t.Errorf("GetThemeColors(%d)[%q] = #%06x, want %v (#%06x)",
					tc.theme, tc.key,
					uint32(got.Hex())&0xffffff, tc.want,
					uint32(tc.want.Hex())&0xffffff)
			}
		})
	}
}

// TestMsgWarningUntrustedBG pins the msg_warning_untrusted bg to ColorMaroon
// (dark red, not ColorRed which is light red). Python's palette has
// msg_warning_untrusted high BG = "dark red" (TextUI.py:39). The Go port's
// theme.go previously used tcell.ColorRed (0xff0000 = light red).
func TestMsgWarningUntrustedBG(t *testing.T) {
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
			colors := GetThemeColors(tc.theme)
			got := colors["msg_warning_untrusted_bg"]
			if got != tcell.ColorMaroon {
				t.Errorf("msg_warning_untrusted_bg = #%06x, want ColorMaroon "+
					"(dark red #800000), not ColorRed (#ff0000)",
					uint32(got.Hex())&0xffffff)
			}
		})
	}
}
