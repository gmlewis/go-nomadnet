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
)

// TestGrayscaleRamp pins the parseColor gNN handling to the urwid 256-color
// grayscale ramp. urwid maps gNN through _GRAY_256_LOOKUP_101 to a 256-color
// index, then to RGB via _COLOR_VALUES_256.
// Python source: urwid display/common.py _parse_color_256, _GRAY_256_LOOKUP_101,
// _COLOR_VALUES_256.
func TestGrayscaleRamp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		spec string
		want uint32
	}{
		{"g0", 0x000000},   // black (idx 16) — tcell ColorBlack
		{"g93", 0xeeeeee},  // ramp step 23 → idx 255 → (238,238,238)
		{"g100", 0xffffff}, // white (idx 231)
		{"g50", 0x808080},  // ramp step 12 → idx 244 → (128,128,128)
		{"g10", 0x1c1c1c},  // ramp step 2 → idx 234 → (28,28,28)
		{"g90", 0xe4e4e4},  // ramp step 22 → idx 254 → (228,228,228)
	} {
		got := parseColor(tc.spec)
		if uint32(got.Hex())&0xffffff != tc.want {
			t.Errorf("parseColor(%q) = #%06x, want #%06x",
				tc.spec, uint32(got.Hex())&0xffffff, tc.want)
		}
	}
}

// TestHeadingColor pins the theme.go heading color to the urwid g93 grayscale
// ramp value (#eeeeee). The Python palette uses "g93,underline" for the heading
// high-color (TextUI.py:20). urwid's grayscale ramp maps g93 to 256-color
// index 255 → RGB (238,238,238) = #eeeeee.
func TestHeadingColor(t *testing.T) {
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
			got := colors["heading"]
			if uint32(got.Hex())&0xffffff != 0xeeeeee {
				t.Errorf("heading = #%06x, want #eeeeee (g93 grayscale ramp)",
					uint32(got.Hex())&0xffffff)
			}
		})
	}
}

// TestListsHeadingColor pins the heading text color to the g93 value
// (#eeeeee): parseColor is the source of truth for gNN, and the theme's
// heading entry must agree.
func TestListsHeadingColor(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	_ = app

	// parseColor("g93") is the source of truth for the gNN value.
	got := parseColor("g93")
	if uint32(got.Hex())&0xffffff != 0xeeeeee {
		t.Errorf("parseColor(g93) = #%06x, want #eeeeee", uint32(got.Hex())&0xffffff)
	}

	// Also verify GetThemeColors heading matches.
	colors := GetThemeColors(ThemeDark)
	if uint32(colors["heading"].Hex())&0xffffff != 0xeeeeee {
		t.Errorf("theme heading = #%06x, want #eeeeee",
			uint32(colors["heading"].Hex())&0xffffff)
	}
}
