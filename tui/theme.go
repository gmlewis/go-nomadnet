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

// GetThemeColors returns the color map for the given theme.
func GetThemeColors(theme int) map[string]tcell.Color {
	if theme == ThemeLight {
		return lightColors
	}
	return darkColors
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
