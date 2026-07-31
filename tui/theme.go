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

// Package tui implements the NomadNet terminal UI using rivo/tview.
//
// It provides dark and light themes, configurable glyph sets
// (plain, unicode, nerdfont), and a top menu bar with content
// area switching for all NomadNet displays.
package tui

import (
	"github.com/gdamore/tcell/v2"
)

// Theme constants matching Python NomadNet.
const (
	ThemeDark  = 1
	ThemeLight = 2
)

// Glyph set constants.
const (
	GlyphPlain   = "plain"
	GlyphUnicode = "unicode"
	GlyphNerd    = "nerdfont"
)

// Color definitions for the dark theme.
var darkColors = map[string]tcell.Color{
	"heading":                    tcell.NewHexColor(0x999999), // g93
	"menubar_fg":                 tcell.NewHexColor(0x111111),
	"menubar_bg":                 tcell.NewHexColor(0xbbbbbb),
	"scrollbar":                  tcell.NewHexColor(0x444444),
	"body_text":                  tcell.NewHexColor(0xdddddd),
	"error_text":                 tcell.NewHexColor(0xaa2222),
	"warning_text":               tcell.NewHexColor(0xbbaa44),
	"inactive_text":              tcell.NewHexColor(0x666666),
	"browser_inactive":           tcell.NewHexColor(0x444444),
	"buttons":                    tcell.NewHexColor(0x00a533),
	"msg_editor_fg":              tcell.NewHexColor(0x111111),
	"msg_editor_bg":              tcell.NewHexColor(0x00bbbb),
	"msg_header_ok_fg":           tcell.NewHexColor(0x111111),
	"msg_header_ok_bg":           tcell.NewHexColor(0x66bb22),
	"msg_header_caution_fg":      tcell.NewHexColor(0x111111),
	"msg_header_caution_bg":      tcell.NewHexColor(0xffdd33),
	"msg_header_sent_fg":         tcell.NewHexColor(0x111111),
	"msg_header_sent_bg":         tcell.NewHexColor(0xdddddd),
	"msg_header_propagated_fg":   tcell.NewHexColor(0x111111),
	"msg_header_propagated_bg":   tcell.NewHexColor(0x2288bb),
	"msg_header_delivered_fg":    tcell.NewHexColor(0x111111),
	"msg_header_delivered_bg":    tcell.NewHexColor(0x2288bb),
	"msg_header_failed_fg":       tcell.NewHexColor(0x000000),
	"msg_header_failed_bg":       tcell.NewHexColor(0x777777),
	"msg_warning_untrusted_fg":   tcell.NewHexColor(0x111111),
	"msg_warning_untrusted_bg":   tcell.ColorRed,
	"msg_notice_unread":          tcell.NewHexColor(0x2288bb),
	"msg_notice_caution":         tcell.NewHexColor(0xffdd33),
	"list_focus_fg":              tcell.NewHexColor(0x111111),
	"list_focus_bg":              tcell.NewHexColor(0xaaaaaa),
	"list_off_focus_fg":          tcell.NewHexColor(0x111111),
	"list_off_focus_bg":          tcell.NewHexColor(0x777777),
	"list_trusted":               tcell.NewHexColor(0x66bb22),
	"list_focus_trusted_fg":      tcell.NewHexColor(0x151500),
	"list_focus_trusted_bg":      tcell.NewHexColor(0xaaaaaa),
	"list_unknown":               tcell.NewHexColor(0xbbbbbb),
	"list_normal":                tcell.NewHexColor(0xbbbbbb),
	"list_untrusted":             tcell.NewHexColor(0xaa2222),
	"list_focus_untrusted_fg":    tcell.NewHexColor(0x881100),
	"list_focus_untrusted_bg":    tcell.NewHexColor(0xaaaaaa),
	"list_unresponsive":          tcell.NewHexColor(0xbb9922),
	"list_focus_unresponsive_fg": tcell.NewHexColor(0x553300),
	"list_focus_unresponsive_bg": tcell.NewHexColor(0xaaaaaa),
	"topic_list_normal":          tcell.NewHexColor(0xdddddd),
	"browser_controls":           tcell.NewHexColor(0xbbbbbb),
	"progress_full_fg":           tcell.NewHexColor(0x111111),
	"progress_full_bg":           tcell.NewHexColor(0xbbbbbb),
	"progress_empty":             tcell.NewHexColor(0xdddddd),
	"placeholder":                tcell.NewHexColor(0x666666),
	"placeholder_text":           tcell.NewHexColor(0x666666),
	"irc_ts":                     tcell.NewHexColor(0x888888),
	"irc_nick_self":              tcell.NewHexColor(0x66cc55),
	"irc_nick_peer":              tcell.NewHexColor(0x33ccdd),
	"irc_notice":                 tcell.NewHexColor(0xffdd33),
	"irc_error":                  tcell.NewHexColor(0xff5555),
	"irc_system":                 tcell.NewHexColor(0x888888),
	"irc_mention_fg":             tcell.NewHexColor(0xffbb44),
	"interface_title":            tcell.NewHexColor(0xdddddd),
	"interface_title_selected":   tcell.NewHexColor(0xaaaaaa),
	"connected_status":           tcell.NewHexColor(0x66bb22),
	"disconnected_status":        tcell.NewHexColor(0xaa2222),
	"shortcutbar":                tcell.NewHexColor(0xdddddd),
}

// Color definitions for the light theme.
var lightColors = map[string]tcell.Color{
	"heading":                    tcell.NewHexColor(0x999999), // g93
	"menubar_fg":                 tcell.NewHexColor(0x111111),
	"menubar_bg":                 tcell.NewHexColor(0xbbbbbb),
	"scrollbar":                  tcell.NewHexColor(0x444444),
	"body_text":                  tcell.NewHexColor(0x222222),
	"error_text":                 tcell.NewHexColor(0xaa2222),
	"warning_text":               tcell.NewHexColor(0xbbaa44),
	"inactive_text":              tcell.NewHexColor(0x666666),
	"buttons":                    tcell.NewHexColor(0x00a533),
	"msg_editor_fg":              tcell.NewHexColor(0x111111),
	"msg_editor_bg":              tcell.NewHexColor(0x00bbbb),
	"msg_header_ok_fg":           tcell.NewHexColor(0x111111),
	"msg_header_ok_bg":           tcell.NewHexColor(0x66bb22),
	"msg_header_caution_fg":      tcell.NewHexColor(0x111111),
	"msg_header_caution_bg":      tcell.NewHexColor(0xffdd33),
	"msg_header_sent_fg":         tcell.NewHexColor(0x111111),
	"msg_header_sent_bg":         tcell.NewHexColor(0xdddddd),
	"msg_header_propagated_fg":   tcell.NewHexColor(0x111111),
	"msg_header_propagated_bg":   tcell.NewHexColor(0x2288bb),
	"msg_header_delivered_fg":    tcell.NewHexColor(0x111111),
	"msg_header_delivered_bg":    tcell.NewHexColor(0x2288bb),
	"msg_header_failed_fg":       tcell.NewHexColor(0x000000),
	"msg_header_failed_bg":       tcell.NewHexColor(0x777777),
	"msg_warning_untrusted_fg":   tcell.NewHexColor(0x111111),
	"msg_warning_untrusted_bg":   tcell.ColorRed,
	"msg_notice_unread":          tcell.NewHexColor(0x006699),
	"msg_notice_caution":         tcell.NewHexColor(0xffdd33),
	"list_focus_fg":              tcell.NewHexColor(0x111111),
	"list_focus_bg":              tcell.NewHexColor(0xaaaaaa),
	"list_off_focus_fg":          tcell.NewHexColor(0x111111),
	"list_off_focus_bg":          tcell.NewHexColor(0x777777),
	"list_trusted":               tcell.NewHexColor(0x44aa00),
	"list_focus_trusted_fg":      tcell.NewHexColor(0x151500),
	"list_focus_trusted_bg":      tcell.NewHexColor(0xaaaaaa),
	"list_unknown":               tcell.NewHexColor(0x444444),
	"list_normal":                tcell.NewHexColor(0x444444),
	"list_untrusted":             tcell.NewHexColor(0xaa2222),
	"list_focus_untrusted_fg":    tcell.NewHexColor(0x881100),
	"list_focus_untrusted_bg":    tcell.NewHexColor(0xaaaaaa),
	"list_unresponsive":          tcell.NewHexColor(0xbb9922),
	"list_focus_unresponsive_fg": tcell.NewHexColor(0x553300),
	"list_focus_unresponsive_bg": tcell.NewHexColor(0xaaaaaa),
	"topic_list_normal":          tcell.NewHexColor(0x222222),
	"browser_controls":           tcell.NewHexColor(0x444444),
	"progress_full_fg":           tcell.NewHexColor(0x111111),
	"progress_full_bg":           tcell.NewHexColor(0xbbbbbb),
	"progress_empty":             tcell.NewHexColor(0xdddddd),
	"placeholder":                tcell.NewHexColor(0x999999),
	"placeholder_text":           tcell.NewHexColor(0x999999),
	"irc_ts":                     tcell.NewHexColor(0x888888),
	"irc_nick_self":              tcell.NewHexColor(0x33aa00),
	"irc_nick_peer":              tcell.NewHexColor(0x007777),
	"irc_notice":                 tcell.NewHexColor(0xaa7700),
	"irc_error":                  tcell.NewHexColor(0xaa2222),
	"irc_system":                 tcell.NewHexColor(0x888888),
	"irc_mention_fg":             tcell.NewHexColor(0xffbb44),
	"interface_title":            tcell.NewHexColor(0x444444),
	"interface_title_selected":   tcell.NewHexColor(0x444444),
	"connected_status":           tcell.NewHexColor(0x44aa00),
	"disconnected_status":        tcell.NewHexColor(0xaa2222),
	"shortcutbar":                tcell.NewHexColor(0x111111),
}

// Glyph definitions: name → (plain, unicode, nerd)
type GlyphSet map[string]string

var glyphsPlain = GlyphSet{
	"check":           "=",
	"cross":           "X",
	"unknown":         "?",
	"encrypted":       "",
	"plaintext":       "!",
	"arrow_r":         "->",
	"arrow_l":         "<-",
	"arrow_u":         "/\\",
	"arrow_d":         "\\/",
	"warning":         "!",
	"info":            "i",
	"unread":          "[!]",
	"divider1":        "-",
	"peer":            "[P]",
	"node":            "[N]",
	"page":            "",
	"speed":           "",
	"decoration_menu": " +",
	"unread_menu":     " !",
	"globe":           "",
	"sent":            "/\\",
	"papermsg":        "P",
	"qrcode":          "QR",
	"selected":        "[*] ",
	"unselected":      "[ ] ",
	"file":            "[F]",
	"image":           "[I]",
	"audio":           "[~]",
	"pin":             "*",
	"copy":            "[C]",
}

var glyphsUnicode = GlyphSet{
	"check":           "\u2713",
	"cross":           "\u2715",
	"unknown":         "?",
	"encrypted":       "\u26BF",
	"plaintext":       "!",
	"arrow_r":         "\u2192",
	"arrow_l":         "\u2190",
	"arrow_u":         "\u2191",
	"arrow_d":         "\u2193",
	"warning":         "\u26a0",
	"info":            "\u2139",
	"unread":          "\u2709",
	"divider1":        "\u2504",
	"peer":            "\u24c5 ",
	"node":            "\u24c3 ",
	"page":            "\u25a4 ",
	"speed":           "\u25F7 ",
	"decoration_menu": " +",
	"unread_menu":     " \u2709",
	"globe":           "",
	"sent":            "\u2191",
	"papermsg":        "\u25a4",
	"qrcode":          "\u25a4",
	"selected":        "\u25CF ",
	"unselected":      "\u25CB ",
	"file":            "\u25a4",
	"image":           "\u25a3",
	"audio":           "\u266b",
	"pin":             "\u2605",
	"copy":            "\u29c9",
}

var glyphsNerd = GlyphSet{
	"check":           "\u2713",
	"cross":           "\u2715",
	"unknown":         "?",
	"encrypted":       "\uf023",
	"plaintext":       "\uf06e ",
	"arrow_r":         "\u2192",
	"arrow_l":         "\u2190",
	"arrow_u":         "\u2191",
	"arrow_d":         "\u2193",
	"warning":         "\uf12a",
	"info":            "\U000f064e",
	"unread":          "\uf0e0",
	"divider1":        "\u2504",
	"peer":            "\uf415",
	"node":            "\U000f0002",
	"page":            "\uf719 ",
	"speed":           "\U000f04c5 ",
	"decoration_menu": " \U000f043b",
	"unread_menu":     " \uf0e0",
	"globe":           "\uf484",
	"sent":            "\U000f0cd8",
	"papermsg":        "\uf719",
	"qrcode":          "\uf029",
	"selected":        "\u25CF ",
	"unselected":      "\u25CB ",
	"file":            "\uf15b",
	"image":           "\uf1c5",
	"audio":           "\uf1c7",
	"pin":             "\uf08d",
	"copy":            "\uf0c5",
}

// GlyphSets maps glyph set names to their glyph maps.
var GlyphSets = map[string]GlyphSet{
	GlyphPlain:   glyphsPlain,
	GlyphUnicode: glyphsUnicode,
	GlyphNerd:    glyphsNerd,
}

// GetThemeColors returns the color map for the given theme.
func GetThemeColors(theme int) map[string]tcell.Color {
	if theme == ThemeLight {
		return lightColors
	}
	return darkColors
}

// GetGlyphSet returns the glyph set for the given name.
func GetGlyphSet(name string) GlyphSet {
	if gs, ok := GlyphSets[name]; ok {
		return gs
	}
	return glyphsUnicode
}

// Menu items for the top menu bar.
type MenuItem struct {
	Label string
	Key   string
}

// MenuItems defines the top-level menu bar entries.
var MenuItems = []MenuItem{
	{Label: "Network", Key: "network"},
	{Label: "Conversations", Key: "conversations"},
	{Label: "Channels", Key: "channels"},
	{Label: "Directory", Key: "directory"},
	{Label: "Map", Key: "map"},
	{Label: "Log", Key: "log"},
	{Label: "Config", Key: "config"},
	{Label: "Interfaces", Key: "interfaces"},
	{Label: "Guide", Key: "guide"},
	{Label: "Quit", Key: "quit"},
}
