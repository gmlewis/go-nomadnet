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

// darkThemeGolden is the complete dark-theme urwid palette, transcribed
// verbatim from TextUI.py:22-69. Each entry is the 5-tuple
// (LowFG, LowBG, Mono, HighFG, HighBG).
var darkThemeGolden = map[string]PaletteEntry{
	"heading":                  {"heading", "light gray,underline", "default", "underline", "g93,underline", "default"},
	"menubar":                  {"menubar", "black", "light gray", "standout", "#111", "#bbb"},
	"scrollbar":                {"scrollbar", "light gray", "default", "default", "#444", "default"},
	"shortcutbar":              {"shortcutbar", "black", "light gray", "standout", "#111", "#bbb"},
	"body_text":                {"body_text", "light gray", "default", "default", "#ddd", "default"},
	"error_text":               {"error_text", "dark red", "default", "default", "dark red", "default"},
	"warning_text":             {"warning_text", "yellow", "default", "default", "#ba4", "default"},
	"inactive_text":            {"inactive_text", "dark gray", "default", "default", "dark gray", "default"},
	"browser_inactive":         {"browser_inactive", "dark gray", "default", "default", "#444", "default"},
	"buttons":                  {"buttons", "light green,bold", "default", "default", "#00a533", "default"},
	"msg_editor":               {"msg_editor", "black", "light cyan", "standout", "#111", "#0bb"},
	"msg_header_ok":            {"msg_header_ok", "black", "light green", "standout", "#111", "#6b2"},
	"msg_header_caution":       {"msg_header_caution", "black", "yellow", "standout", "#111", "#fd3"},
	"msg_header_sent":          {"msg_header_sent", "black", "light gray", "standout", "#111", "#ddd"},
	"msg_header_propagated":    {"msg_header_propagated", "black", "light blue", "standout", "#111", "#28b"},
	"msg_header_delivered":     {"msg_header_delivered", "black", "light blue", "standout", "#111", "#28b"},
	"msg_header_failed":        {"msg_header_failed", "black", "dark gray", "standout", "#000", "#777"},
	"msg_warning_untrusted":    {"msg_warning_untrusted", "black", "dark red", "standout", "#111", "dark red"},
	"msg_notice_unread":        {"msg_notice_unread", "light blue", "default", "standout", "#28b", "default"},
	"msg_notice_caution":       {"msg_notice_caution", "yellow", "default", "standout", "#fd3", "default"},
	"list_focus":               {"list_focus", "black", "light gray", "standout", "#111", "#aaa"},
	"list_off_focus":           {"list_off_focus", "black", "dark gray", "standout", "#111", "#777"},
	"list_trusted":             {"list_trusted", "dark green", "default", "default", "#6b2", "default"},
	"list_focus_trusted":       {"list_focus_trusted", "black", "light gray", "standout", "#150", "#aaa"},
	"list_unknown":             {"list_unknown", "dark gray", "default", "default", "#bbb", "default"},
	"list_normal":              {"list_normal", "dark gray", "default", "default", "#bbb", "default"},
	"list_untrusted":           {"list_untrusted", "dark red", "default", "default", "#a22", "default"},
	"list_focus_untrusted":     {"list_focus_untrusted", "black", "light gray", "standout", "#810", "#aaa"},
	"list_unresponsive":        {"list_unresponsive", "yellow", "default", "default", "#b92", "default"},
	"list_focus_unresponsive":  {"list_focus_unresponsive", "black", "light gray", "standout", "#530", "#aaa"},
	"topic_list_normal":        {"topic_list_normal", "light gray", "default", "default", "#ddd", "default"},
	"browser_controls":         {"browser_controls", "light gray", "default", "default", "#bbb", "default"},
	"progress_full":            {"progress_full", "black", "light gray", "standout", "#111", "#bbb"},
	"progress_empty":           {"progress_empty", "light gray", "default", "default", "#ddd", "default"},
	"interface_title":          {"interface_title", "", "", "default", "", ""},
	"interface_title_selected": {"interface_title_selected", "bold", "", "bold", "", ""},
	"connected_status":         {"connected_status", "dark green", "default", "default", "dark green", "default"},
	"disconnected_status":      {"disconnected_status", "dark red", "default", "default", "dark red", "default"},
	"placeholder":              {"placeholder", "dark gray", "default", "default", "dark gray", "default"},
	"placeholder_text":         {"placeholder_text", "dark gray", "default", "default", "dark gray", "default"},
	"error":                    {"error", "light red,blink", "default", "blink", "#f44,blink", "default"},
	"irc_ts":                   {"irc_ts", "dark gray", "default", "default", "#888", "default"},
	"irc_nick_self":            {"irc_nick_self", "light green", "default", "default", "#6c5", "default"},
	"irc_nick_peer":            {"irc_nick_peer", "light cyan", "default", "default", "#3cd", "default"},
	"irc_notice":               {"irc_notice", "yellow", "default", "default", "#fd3", "default"},
	"irc_error":                {"irc_error", "light red", "default", "default", "#f55", "default"},
	"irc_system":               {"irc_system", "dark gray", "default", "default", "#888", "default"},
	"irc_mention":              {"irc_mention", "light red,bold", "default", "bold", "#fb4,bold", "default"},
}

// lightThemeGolden is the complete light-theme urwid palette, transcribed
// verbatim from TextUI.py:76-122. The light theme has no browser_inactive
// entry, so its key set differs from the dark theme's by exactly that name.
var lightThemeGolden = map[string]PaletteEntry{
	"heading":                  {"heading", "dark gray,underline", "default", "underline", "g93,underline", "default"},
	"menubar":                  {"menubar", "black", "dark gray", "standout", "#111", "#bbb"},
	"scrollbar":                {"scrollbar", "dark gray", "default", "default", "#444", "default"},
	"shortcutbar":              {"shortcutbar", "black", "dark gray", "standout", "#111", "#bbb"},
	"body_text":                {"body_text", "dark gray", "default", "default", "#222", "default"},
	"error_text":               {"error_text", "dark red", "default", "default", "dark red", "default"},
	"warning_text":             {"warning_text", "yellow", "default", "default", "#ba4", "default"},
	"inactive_text":            {"inactive_text", "light gray", "default", "default", "dark gray", "default"},
	"buttons":                  {"buttons", "light green,bold", "default", "default", "#00a533", "default"},
	"msg_editor":               {"msg_editor", "black", "dark cyan", "standout", "#111", "#0bb"},
	"msg_header_ok":            {"msg_header_ok", "black", "dark green", "standout", "#111", "#6b2"},
	"msg_header_caution":       {"msg_header_caution", "black", "yellow", "standout", "#111", "#fd3"},
	"msg_header_sent":          {"msg_header_sent", "black", "dark gray", "standout", "#111", "#ddd"},
	"msg_header_propagated":    {"msg_header_propagated", "black", "light blue", "standout", "#111", "#28b"},
	"msg_header_delivered":     {"msg_header_delivered", "black", "light blue", "standout", "#111", "#28b"},
	"msg_header_failed":        {"msg_header_failed", "black", "dark gray", "standout", "#000", "#777"},
	"msg_warning_untrusted":    {"msg_warning_untrusted", "black", "dark red", "standout", "#111", "dark red"},
	"msg_notice_unread":        {"msg_notice_unread", "dark blue", "default", "standout", "#069", "default"},
	"msg_notice_caution":       {"msg_notice_caution", "yellow", "default", "standout", "#fd3", "default"},
	"list_focus":               {"list_focus", "black", "dark gray", "standout", "#111", "#aaa"},
	"list_off_focus":           {"list_off_focus", "black", "dark gray", "standout", "#111", "#777"},
	"list_trusted":             {"list_trusted", "dark green", "default", "default", "#4a0", "default"},
	"list_focus_trusted":       {"list_focus_trusted", "black", "dark gray", "standout", "#150", "#aaa"},
	"list_unknown":             {"list_unknown", "dark gray", "default", "default", "#444", "default"},
	"list_normal":              {"list_normal", "dark gray", "default", "default", "#444", "default"},
	"list_untrusted":           {"list_untrusted", "dark red", "default", "default", "#a22", "default"},
	"list_focus_untrusted":     {"list_focus_untrusted", "black", "dark gray", "standout", "#810", "#aaa"},
	"list_unresponsive":        {"list_unresponsive", "yellow", "default", "default", "#b92", "default"},
	"list_focus_unresponsive":  {"list_focus_unresponsive", "black", "light gray", "standout", "#530", "#aaa"},
	"topic_list_normal":        {"topic_list_normal", "dark gray", "default", "default", "#222", "default"},
	"browser_controls":         {"browser_controls", "dark gray", "default", "default", "#444", "default"},
	"progress_full":            {"progress_full", "black", "dark gray", "standout", "#111", "#bbb"},
	"progress_empty":           {"progress_empty", "dark gray", "default", "default", "#ddd", "default"},
	"interface_title":          {"interface_title", "dark gray", "default", "default", "#444", "default"},
	"interface_title_selected": {"interface_title_selected", "dark gray,bold", "default", "bold", "#444,bold", "default"},
	"connected_status":         {"connected_status", "dark green", "default", "default", "#4a0", "default"},
	"disconnected_status":      {"disconnected_status", "dark red", "default", "default", "#a22", "default"},
	"placeholder":              {"placeholder", "light gray", "default", "default", "#999", "default"},
	"placeholder_text":         {"placeholder_text", "light gray", "default", "default", "#999", "default"},
	"error":                    {"error", "dark red,blink", "default", "blink", "#a22,blink", "default"},
	"irc_ts":                   {"irc_ts", "dark gray", "default", "default", "#888", "default"},
	"irc_nick_self":            {"irc_nick_self", "dark green", "default", "default", "#3a0", "default"},
	"irc_nick_peer":            {"irc_nick_peer", "dark cyan", "default", "default", "#077", "default"},
	"irc_notice":               {"irc_notice", "brown", "default", "default", "#a70", "default"},
	"irc_error":                {"irc_error", "dark red", "default", "default", "#a22", "default"},
	"irc_system":               {"irc_system", "dark gray", "default", "default", "#888", "default"},
	"irc_mention":              {"irc_mention", "dark red,bold", "default", "bold", "#c50,bold", "default"},
}

func TestPaletteMatchesPython(t *testing.T) {
	t.Parallel()

	themes := []struct {
		name       string
		theme      int
		golden     map[string]PaletteEntry
		implements map[string]PaletteEntry
	}{
		{"dark", ThemeDark, darkThemeGolden, darkPalette},
		{"light", ThemeLight, lightThemeGolden, lightPalette},
	}
	for _, th := range themes {
		// Every golden entry must be present and match verbatim.
		for name, want := range th.golden {
			got, ok := paletteLookup(th.theme, name)
			if !ok {
				t.Errorf("%s palette missing %q", th.name, name)
				continue
			}
			if got != want {
				t.Errorf("%s %q = %+v, want %+v", th.name, name, got, want)
			}
		}
		// The implementation must not carry entries absent from the Python
		// original (catches typos / inventing styles).
		if len(th.implements) != len(th.golden) {
			t.Errorf("%s palette has %d entries, want %d", th.name, len(th.implements), len(th.golden))
		}
		for name := range th.implements {
			if _, ok := th.golden[name]; !ok {
				t.Errorf("%s palette has extra entry %q not in Python THEMES", th.name, name)
			}
		}
	}
}

func TestResolveStyleDepthSelection(t *testing.T) {
	t.Parallel()

	modes := []int{ColorModeMono, ColorMode16, ColorMode88, ColorMode256, ColorModeTrue}

	for name, e := range darkThemeGolden {
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
