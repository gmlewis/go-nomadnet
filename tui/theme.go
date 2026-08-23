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
	"github.com/gdamore/tcell/v2"
)

// Theme constants matching Python NomadNet.
const (
	ThemeDark  = 1
	ThemeLight = 2
)

// 3-digit "#rgb" palette colors are expressed via cubeHex3, which quantizes
// each nibble to the nearest of urwid's six 256-color-cube steps
// {0,95,135,175,215,255} — EVEN in 24-bit truecolor (urwid's
// _parse_color_true routes 3-hex through _parse_color_256). Nibble-doubling
// (#bbb→#bbbbbb=187) would diverge from Python, which renders #bbb as
// #afafaf (175). 6-hex "#rrggbb" entries (buttons) and named-color entries
// (dark red, dark gray, …) are left as literal tcell.Color values; the
// named-color literals are a separate, pre-existing parity gap, not part of
// the 3-hex cube-quantization fix.

// Color definitions for the dark theme.
var darkColors = map[string]tcell.Color{
	"heading":                    parseColor("g93"), // g93 grayscale ramp → #eeeeee
	"menubar_fg":                 cubeHex3("#111"),
	"menubar_bg":                 cubeHex3("#bbb"),
	"scrollbar":                  cubeHex3("#444"),
	"body_text":                  cubeHex3("#ddd"),
	"error_text":                 tcell.ColorMaroon, // Python "dark red" (named; urwid 16-color SGR 31)
	"warning_text":               cubeHex3("#ba4"),
	"inactive_text":              tcell.ColorGray, // Python "dark gray" (named; urwid 16-color 1;30)
	"browser_inactive":           cubeHex3("#444"),
	"buttons":                    tcell.NewHexColor(0x00a533), // 6-hex, exact (not quantized)
	"msg_editor_fg":              cubeHex3("#111"),
	"msg_editor_bg":              cubeHex3("#0bb"),
	"msg_header_ok_fg":           cubeHex3("#111"),
	"msg_header_ok_bg":           cubeHex3("#6b2"),
	"msg_header_caution_fg":      cubeHex3("#111"),
	"msg_header_caution_bg":      cubeHex3("#fd3"),
	"msg_header_sent_fg":         cubeHex3("#111"),
	"msg_header_sent_bg":         cubeHex3("#ddd"),
	"msg_header_propagated_fg":   cubeHex3("#111"),
	"msg_header_propagated_bg":   cubeHex3("#28b"),
	"msg_header_delivered_fg":    cubeHex3("#111"),
	"msg_header_delivered_bg":    cubeHex3("#28b"),
	"msg_header_failed_fg":       cubeHex3("#000"),
	"msg_header_failed_bg":       cubeHex3("#777"),
	"msg_warning_untrusted_fg":   cubeHex3("#111"),
	"msg_warning_untrusted_bg":   tcell.ColorMaroon, // Python "dark red" (named; urwid 16-color SGR 31)
	"msg_notice_unread":          cubeHex3("#28b"),
	"msg_notice_caution":         cubeHex3("#fd3"),
	"list_focus_fg":              cubeHex3("#111"),
	"list_focus_bg":              cubeHex3("#aaa"),
	"list_off_focus_fg":          cubeHex3("#111"),
	"list_off_focus_bg":          cubeHex3("#777"),
	"list_trusted":               cubeHex3("#6b2"),
	"list_focus_trusted_fg":      cubeHex3("#150"),
	"list_focus_trusted_bg":      cubeHex3("#aaa"),
	"list_unknown":               cubeHex3("#bbb"),
	"list_normal":                cubeHex3("#bbb"),
	"list_untrusted":             cubeHex3("#a22"),
	"list_focus_untrusted_fg":    cubeHex3("#810"),
	"list_focus_untrusted_bg":    cubeHex3("#aaa"),
	"list_unresponsive":          cubeHex3("#b92"),
	"list_focus_unresponsive_fg": cubeHex3("#530"),
	"list_focus_unresponsive_bg": cubeHex3("#aaa"),
	"topic_list_normal":          cubeHex3("#ddd"),
	"browser_controls":           cubeHex3("#bbb"),
	"progress_full_fg":           cubeHex3("#111"),
	"progress_full_bg":           cubeHex3("#bbb"),
	"progress_empty":             cubeHex3("#ddd"),
	"placeholder":                tcell.ColorGray, // Python "dark gray" (named; urwid 16-color 1;30)
	"placeholder_text":           tcell.ColorGray, // Python "dark gray" (named; urwid 16-color 1;30)
	"irc_ts":                     cubeHex3("#888"),
	"irc_nick_self":              cubeHex3("#6c5"),
	"irc_nick_peer":              cubeHex3("#3cd"),
	"irc_notice":                 cubeHex3("#fd3"),
	"irc_error":                  cubeHex3("#f55"),
	"irc_system":                 cubeHex3("#888"),
	"irc_mention_fg":             cubeHex3("#fb4"),
	"interface_title":            tcell.ColorDefault,          // dark = "" (default; named-color resolved)
	"interface_title_selected":   tcell.ColorDefault,          // dark = "bold" no color (named-color resolved)
	"connected_status":           tcell.ColorGreen,            // Python "dark green" (named; urwid 16-color SGR 32)
	"disconnected_status":        tcell.ColorMaroon,           // Python "dark red" (named; urwid 16-color SGR 31)
	"shortcutbar":                tcell.NewHexColor(0xdddddd), // unused; left as-is
}

// Color definitions for the light theme.
var lightColors = map[string]tcell.Color{
	"heading":                    parseColor("g93"), // g93 grayscale ramp → #eeeeee
	"menubar_fg":                 cubeHex3("#111"),
	"menubar_bg":                 cubeHex3("#bbb"),
	"scrollbar":                  cubeHex3("#444"),
	"body_text":                  cubeHex3("#222"),
	"error_text":                 tcell.ColorMaroon, // Python "dark red" (named; urwid 16-color SGR 31)
	"warning_text":               cubeHex3("#ba4"),
	"inactive_text":              tcell.ColorGray,             // Python "dark gray" (named; urwid 16-color 1;30)
	"buttons":                    tcell.NewHexColor(0x00a533), // 6-hex, exact (not quantized)
	"msg_editor_fg":              cubeHex3("#111"),
	"msg_editor_bg":              cubeHex3("#0bb"),
	"msg_header_ok_fg":           cubeHex3("#111"),
	"msg_header_ok_bg":           cubeHex3("#6b2"),
	"msg_header_caution_fg":      cubeHex3("#111"),
	"msg_header_caution_bg":      cubeHex3("#fd3"),
	"msg_header_sent_fg":         cubeHex3("#111"),
	"msg_header_sent_bg":         cubeHex3("#ddd"),
	"msg_header_propagated_fg":   cubeHex3("#111"),
	"msg_header_propagated_bg":   cubeHex3("#28b"),
	"msg_header_delivered_fg":    cubeHex3("#111"),
	"msg_header_delivered_bg":    cubeHex3("#28b"),
	"msg_header_failed_fg":       cubeHex3("#000"),
	"msg_header_failed_bg":       cubeHex3("#777"),
	"msg_warning_untrusted_fg":   cubeHex3("#111"),
	"msg_warning_untrusted_bg":   tcell.ColorMaroon, // Python "dark red" (named; urwid 16-color SGR 31)
	"msg_notice_unread":          cubeHex3("#069"),
	"msg_notice_caution":         cubeHex3("#fd3"),
	"list_focus_fg":              cubeHex3("#111"),
	"list_focus_bg":              cubeHex3("#aaa"),
	"list_off_focus_fg":          cubeHex3("#111"),
	"list_off_focus_bg":          cubeHex3("#777"),
	"list_trusted":               cubeHex3("#4a0"),
	"list_focus_trusted_fg":      cubeHex3("#150"),
	"list_focus_trusted_bg":      cubeHex3("#aaa"),
	"list_unknown":               cubeHex3("#444"),
	"list_normal":                cubeHex3("#444"),
	"list_untrusted":             cubeHex3("#a22"),
	"list_focus_untrusted_fg":    cubeHex3("#810"),
	"list_focus_untrusted_bg":    cubeHex3("#aaa"),
	"list_unresponsive":          cubeHex3("#b92"),
	"list_focus_unresponsive_fg": cubeHex3("#530"),
	"list_focus_unresponsive_bg": cubeHex3("#aaa"),
	"topic_list_normal":          cubeHex3("#222"),
	"browser_controls":           cubeHex3("#444"),
	"progress_full_fg":           cubeHex3("#111"),
	"progress_full_bg":           cubeHex3("#bbb"),
	"progress_empty":             cubeHex3("#ddd"),
	"placeholder":                cubeHex3("#999"),
	"placeholder_text":           cubeHex3("#999"),
	"irc_ts":                     cubeHex3("#888"),
	"irc_nick_self":              cubeHex3("#3a0"),
	"irc_nick_peer":              cubeHex3("#077"),
	"irc_notice":                 cubeHex3("#a70"),
	"irc_error":                  cubeHex3("#a22"),
	"irc_system":                 cubeHex3("#888"),
	"irc_mention_fg":             cubeHex3("#c50"),
	"interface_title":            cubeHex3("#444"),
	"interface_title_selected":   cubeHex3("#444"),
	"connected_status":           cubeHex3("#4a0"),
	"disconnected_status":        cubeHex3("#a22"),
	"shortcutbar":                tcell.NewHexColor(0x111111), // unused; left as-is
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

// MenuItems defines the top-level menu bar entries, transcribed from Python
// nomadnet/ui/textui/Main.py:201-204. Directory and Map are deliberately absent
// — they are SubDisplays but not top-level menu buttons. On-screen order is
// Conversations, Network, Channels, Log, Interfaces, Config, Guide, Quit (the
// Guide button is dropped by SetHideGuide when the config hides it).
var MenuItems = []MenuItem{
	{Label: "Conversations", Key: "conversations"},
	{Label: "Network", Key: "network"},
	{Label: "Channels", Key: "channels"},
	{Label: "Log", Key: "log"},
	{Label: "Interfaces", Key: "interfaces"},
	{Label: "Config", Key: "config"},
	{Label: "Guide", Key: "guide"},
	{Label: "Quit", Key: "quit"},
}
