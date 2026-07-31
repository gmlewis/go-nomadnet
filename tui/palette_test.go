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

// Golden values captured verbatim from the Python urwid THEMES palette in
// nomadnet/ui/TextUI.py:18-125. Each urwid palette row is a 5-tuple
// (16-color fg, 16-color bg, monochrome spec, 88/256/true-color fg,
// 88/256/true-color bg); the cases below assert that ResolveStyle selects
// the matching column for each colormode, exactly as urwid does at render
// time.

func TestParseColorMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int
	}{
		{"monochrome", ColorModeMono},
		{"mono", ColorModeMono},
		{"1", ColorModeMono},
		{"Monochrome", ColorModeMono},
		{"16", ColorMode16},
		{"88", ColorMode88},
		{"256", ColorMode256},
		{"24bit", ColorModeTrue},
		{"24", ColorModeTrue},
		{"truecolor", ColorModeTrue},
		{"true", ColorModeTrue},
		{"", ColorModeTrue}, // shipped default is 24-bit
		{"junk", ColorModeTrue},
	}
	for _, c := range cases {
		got := ParseColorMode(c.in)
		if got != c.want {
			t.Errorf("ParseColorMode(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// darkGolden holds the 5-tuples for a representative subset of the dark
// theme, captured from TextUI.py:22-69.
var darkGolden = map[string]PaletteEntry{
	"heading":        {"heading", "light gray,underline", "default", "underline", "g93,underline", "default"},
	"menubar":        {"menubar", "black", "light gray", "standout", "#111", "#bbb"},
	"shortcutbar":    {"shortcutbar", "black", "light gray", "standout", "#111", "#bbb"},
	"body_text":      {"body_text", "light gray", "default", "default", "#ddd", "default"},
	"list_focus":     {"list_focus", "black", "light gray", "standout", "#111", "#aaa"},
	"list_off_focus": {"list_off_focus", "black", "dark gray", "standout", "#111", "#777"},
	"msg_header_ok":  {"msg_header_ok", "black", "light green", "standout", "#111", "#6b2"},
	"error":          {"error", "light red,blink", "default", "blink", "#f44,blink", "default"},
	"irc_mention":    {"irc_mention", "light red,bold", "default", "bold", "#fb4,bold", "default"},
}

// lightGolden holds the same subset from the light theme (TextUI.py:76-122).
var lightGolden = map[string]PaletteEntry{
	"heading":        {"heading", "dark gray,underline", "default", "underline", "g93,underline", "default"},
	"menubar":        {"menubar", "black", "dark gray", "standout", "#111", "#bbb"},
	"shortcutbar":    {"shortcutbar", "black", "dark gray", "standout", "#111", "#bbb"},
	"body_text":      {"body_text", "dark gray", "default", "default", "#222", "default"},
	"list_focus":     {"list_focus", "black", "dark gray", "standout", "#111", "#aaa"},
	"list_off_focus": {"list_off_focus", "black", "dark gray", "standout", "#111", "#777"},
	"msg_header_ok":  {"msg_header_ok", "black", "dark green", "standout", "#111", "#6b2"},
	"error":          {"error", "dark red,blink", "default", "blink", "#a22,blink", "default"},
	"irc_mention":    {"irc_mention", "dark red,bold", "default", "bold", "#c50,bold", "default"},
}

func TestPaletteSubsetMatchesPython(t *testing.T) {
	t.Parallel()

	for name, want := range darkGolden {
		got, ok := paletteLookup(ThemeDark, name)
		if !ok {
			t.Errorf("dark palette missing %q", name)
			continue
		}
		if got != want {
			t.Errorf("dark %q = %+v, want %+v", name, got, want)
		}
	}
	for name, want := range lightGolden {
		got, ok := paletteLookup(ThemeLight, name)
		if !ok {
			t.Errorf("light palette missing %q", name)
			continue
		}
		if got != want {
			t.Errorf("light %q = %+v, want %+v", name, got, want)
		}
	}
}

func TestResolveStyleDepthSelection(t *testing.T) {
	t.Parallel()

	modes := []int{ColorModeMono, ColorMode16, ColorMode88, ColorMode256, ColorModeTrue}

	for name, e := range darkGolden {
		// Monochrome: the single mono spec becomes the foreground; urwid's
		// mono column carries no background, so it resolves to "default".
		fg, bg := ResolveStyle(e, ColorModeMono)
		if fg != e.Mono || bg != "default" {
			t.Errorf("mono %q = (%q,%q), want (%q,%q)", name, fg, bg, e.Mono, "default")
		}
		// 16-color: the low-color fg/bg columns.
		fg, bg = ResolveStyle(e, ColorMode16)
		if fg != e.LowFG || bg != e.LowBG {
			t.Errorf("16 %q = (%q,%q), want (%q,%q)", name, fg, bg, e.LowFG, e.LowBG)
		}
		// 88/256/true all select the high-color columns.
		for _, cm := range modes[2:] {
			fg, bg = ResolveStyle(e, cm)
			if fg != e.HighFG || bg != e.HighBG {
				t.Errorf("depth %d %q = (%q,%q), want (%q,%q)", cm, name, fg, bg, e.HighFG, e.HighBG)
			}
		}
	}
}

func TestRegisterThemeStylesTrueColor(t *testing.T) {
	RegisterThemeStyles(ThemeDark, ColorModeTrue)

	cases := []struct {
		name      string
		wantFG    tcell.Color
		wantBG    tcell.Color
		wantAttr  tcell.AttrMask
		checkAttr bool
	}{
		{"menubar", tcell.NewHexColor(0x111111), tcell.NewHexColor(0xbbbbbb), 0, false},
		{"list_focus", tcell.NewHexColor(0x111111), tcell.NewHexColor(0xaaaaaa), 0, false},
		{"body_text", tcell.NewHexColor(0xdddddd), tcell.ColorDefault, 0, false},
		{"heading", tcell.NewHexColor(0xefefef), tcell.ColorDefault, tcell.AttrUnderline, true},
		{"error", tcell.NewHexColor(0xff4444), tcell.ColorDefault, tcell.AttrBlink, true},
		{"irc_mention", tcell.NewHexColor(0xffbb44), tcell.ColorDefault, tcell.AttrBold, true},
	}
	for _, c := range cases {
		fg, bg, attr := Style(c.name).Decompose()
		if fg != c.wantFG {
			t.Errorf("Style(%q) fg = %v, want %v", c.name, fg, c.wantFG)
		}
		if bg != c.wantBG {
			t.Errorf("Style(%q) bg = %v, want %v", c.name, bg, c.wantBG)
		}
		if c.checkAttr && attr&c.wantAttr == 0 {
			t.Errorf("Style(%q) attrs = %v, want %v set", c.name, attr, c.wantAttr)
		}
	}
}

func TestRegisterThemeStyles16Color(t *testing.T) {
	RegisterThemeStyles(ThemeDark, ColorMode16)

	fg, bg, _ := Style("menubar").Decompose()
	if fg != tcell.ColorBlack {
		t.Errorf("16-color menubar fg = %v, want ColorBlack", fg)
	}
	if bg != tcell.ColorSilver {
		t.Errorf("16-color menubar bg = %v, want ColorSilver (light gray)", bg)
	}
	// list_off_focus 16-color background is "dark gray" -> tcell.ColorGray.
	if _, got, _ := Style("list_off_focus").Decompose(); got != tcell.ColorGray {
		t.Errorf("16-color list_off_focus bg = %v, want ColorGray (dark gray)", got)
	}
}

func TestRegisterThemeStylesMono(t *testing.T) {
	RegisterThemeStyles(ThemeDark, ColorModeMono)

	// "standout" (menubar mono) maps to reverse video.
	if _, _, attr := Style("menubar").Decompose(); attr&tcell.AttrReverse == 0 {
		t.Errorf("mono menubar attrs = %v, want AttrReverse set", attr)
	}
	// "default" (body_text mono) carries no attribute.
	if _, _, attr := Style("body_text").Decompose(); attr != 0 {
		t.Errorf("mono body_text attrs = %v, want 0", attr)
	}
}

func TestDetectColorModeValid(t *testing.T) {
	cm := DetectColorMode()
	switch cm {
	case ColorModeMono, ColorMode16, ColorMode88, ColorMode256, ColorModeTrue:
		// ok
	default:
		t.Errorf("DetectColorMode() = %d, want one of the known depth constants", cm)
	}
}
