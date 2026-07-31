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
// along with this program. If not, see <https://www.gnu.org/licenses>.

package tui

import (
	"os"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
)

// Color-depth constants matching Python TextUI.py:12-16 (COLORMODE_*). The
// shipped default is 24-bit (true color); below 256 the original resets the
// terminal palette (set_colormode, TextUI.py:254-260).
const (
	ColorModeMono = 1
	ColorMode16   = 16
	ColorMode88   = 88
	ColorMode256  = 256
	ColorModeTrue = 1 << 24
)

// ParseColorMode maps a config "colormode" string to its numeric depth,
// matching the values accepted by Python's set_colormode. Unknown or empty
// values fall back to the shipped default, 24-bit true color.
func ParseColorMode(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "monochrome", "mono", "1":
		return ColorModeMono
	case "16":
		return ColorMode16
	case "88":
		return ColorMode88
	case "256":
		return ColorMode256
	case "24bit", "24", "truecolor", "true":
		return ColorModeTrue
	case "":
		return ColorModeTrue
	}
	return ColorModeTrue
}

// PaletteEntry is one urwid palette row as a 5-tuple: the 16-color
// foreground/background, the monochrome spec, and the 88/256/24-bit
// foreground/background. It mirrors the columns of THEMES in Python
// TextUI.py:18-125. The mono/low/high values are urwid display-attribute
// spec strings (e.g. "light gray,underline", "#111", "g93", "standout").
//
// The micron color-depth helpers in nomadnet/micron/color-depth.go
// (monoColor/lowColor/highColor) synthesize equivalent 5-tuples at render
// time for *arbitrary* micron colors; the static palette below holds the
// hand-authored entries the Python original ships.
type PaletteEntry struct {
	Name   string
	LowFG  string
	LowBG  string
	Mono   string
	HighFG string
	HighBG string
}

// darkPalette is the complete dark-theme urwid palette, transcribed verbatim
// from TextUI.py:22-69. Each value is the 5-tuple (LowFG, LowBG, Mono,
// HighFG, HighBG).
var darkPalette = map[string]PaletteEntry{
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

// lightPalette is the complete light-theme urwid palette, transcribed verbatim
// from TextUI.py:76-122. The light theme has no browser_inactive entry, so
// its key set differs from the dark theme's by exactly that one name.
var lightPalette = map[string]PaletteEntry{
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

// paletteFor returns the named-style map for the given theme (dark default).
func paletteFor(theme int) map[string]PaletteEntry {
	if theme == ThemeLight {
		return lightPalette
	}
	return darkPalette
}

// paletteLookup returns the entry for a named style in the given theme.
func paletteLookup(theme int, name string) (PaletteEntry, bool) {
	e, ok := paletteFor(theme)[name]
	return e, ok
}

// ResolveStyle selects the (foreground, background) spec strings for a style
// at the given color depth, matching urwid's per-depth palette selection:
// monochrome uses the single mono spec (returned as the foreground with a
// "default" background, since urwid's mono column carries no background);
// 16-color uses the low-color columns; 88/256/24-bit use the high-color
// columns.
func ResolveStyle(e PaletteEntry, colorMode int) (fg, bg string) {
	switch {
	case colorMode <= ColorModeMono:
		return e.Mono, "default"
	case colorMode == ColorMode16:
		return e.LowFG, e.LowBG
	default: // 88, 256, true color
		return e.HighFG, e.HighBG
	}
}

// namedColors maps urwid 16-color names to tcell color constants.
var namedColors = map[string]tcell.Color{
	"default":       tcell.ColorDefault,
	"black":         tcell.ColorBlack,
	"white":         tcell.ColorWhite,
	"dark gray":     tcell.ColorGray,
	"light gray":    tcell.ColorSilver,
	"dark red":      tcell.ColorMaroon,
	"light red":     tcell.ColorRed,
	"dark green":    tcell.ColorGreen,
	"light green":   tcell.ColorLime,
	"dark blue":     tcell.ColorNavy,
	"light blue":    tcell.ColorBlue,
	"dark cyan":     tcell.ColorTeal,
	"light cyan":    tcell.ColorAqua,
	"dark magenta":  tcell.ColorPurple,
	"light magenta": tcell.ColorFuchsia,
	"yellow":        tcell.ColorYellow,
	"brown":         tcell.ColorOlive,
}

// parseColor converts a urwid color spec string (named color, "#rgb",
// "#rrggbb", "gNN" grayscale, "default", or empty) to a tcell.Color.
func parseColor(spec string) tcell.Color {
	s := strings.TrimSpace(spec)
	if s == "" || s == "default" {
		return tcell.ColorDefault
	}
	if c, ok := namedColors[s]; ok {
		return c
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		switch len(hex) {
		case 3:
			r, ok1 := hexNibble(hex[0])
			g, ok2 := hexNibble(hex[1])
			b, ok3 := hexNibble(hex[2])
			if !ok1 || !ok2 || !ok3 {
				return tcell.ColorDefault
			}
			rgb := doubleNibble(r)<<16 | doubleNibble(g)<<8 | doubleNibble(b)
			return tcell.NewHexColor(int32(rgb))
		case 6:
			r, ok1 := hexNibble(hex[0])
			r2, ok2 := hexNibble(hex[1])
			g, ok3 := hexNibble(hex[2])
			g2, ok4 := hexNibble(hex[3])
			b, ok5 := hexNibble(hex[4])
			b2, ok6 := hexNibble(hex[5])
			if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
				return tcell.ColorDefault
			}
			rgb := (r<<4 | r2) | (g<<4|g2)<<8 | (b<<4|b2)<<16
			return tcell.NewHexColor(int32(rgb))
		}
		return tcell.ColorDefault
	}
	if len(s) == 3 && s[0] == 'g' {
		d1, ok1 := decDigit(s[1])
		d2, ok2 := decDigit(s[2])
		if !ok1 || !ok2 {
			return tcell.ColorDefault
		}
		v := (d1*10 + d2) * 255 / 99
		return tcell.NewHexColor(int32(v<<16 | v<<8 | v))
	}
	return tcell.ColorDefault
}

// hexNibble parses a single hex digit to its value 0-15.
func hexNibble(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// decDigit parses a single decimal digit to its value 0-9.
func decDigit(c byte) (int, bool) {
	if c >= '0' && c <= '9' {
		return int(c - '0'), true
	}
	return 0, false
}

// doubleNibble doubles a 4-bit value (0x6 -> 0x66), expanding a 3-digit
// "#rgb" hex color to its 6-digit "#rrggbb" form.
func doubleNibble(v int) int { return v<<4 | v }

// parseAttr maps a urwid monochrome/attribute name to a tcell attribute mask.
// urwid "standout" is reverse video.
func parseAttr(s string) tcell.AttrMask {
	switch strings.TrimSpace(s) {
	case "bold":
		return tcell.AttrBold
	case "underline":
		return tcell.AttrUnderline
	case "blink":
		return tcell.AttrBlink
	case "standout", "reverse":
		return tcell.AttrReverse
	}
	return 0
}

// parseSpec splits a urwid display-attribute spec ("color,attr,attr") into a
// tcell color and the combined attribute mask.
func parseSpec(spec string) (tcell.Color, tcell.AttrMask) {
	parts := strings.Split(spec, ",")
	color := parseColor(parts[0])
	var attr tcell.AttrMask
	for _, p := range parts[1:] {
		attr |= parseAttr(p)
	}
	return color, attr
}

// resolveTcellStyle builds a tcell.Style for a palette entry at the given
// depth. In monochrome the single mono spec is an attribute, not a color.
func resolveTcellStyle(e PaletteEntry, colorMode int) tcell.Style {
	if colorMode <= ColorModeMono {
		return tcell.StyleDefault.Attributes(parseAttr(e.Mono))
	}
	fg, bg := ResolveStyle(e, colorMode)
	fgColor, fgAttr := parseSpec(fg)
	bgColor, _ := parseSpec(bg)
	return tcell.StyleDefault.Foreground(fgColor).Background(bgColor).Attributes(fgAttr)
}

// DetectColorMode queries tcell's terminfo database for the current $TERM
// and returns the closest supported depth. If the terminal cannot be
// identified it assumes 24-bit true color (the shipped default).
func DetectColorMode() int {
	if ti, err := tcell.LookupTerminfo(os.Getenv("TERM")); err == nil && ti != nil {
		switch n := ti.Colors; {
		case n <= 1:
			return ColorModeMono
		case n <= 16:
			return ColorMode16
		case n <= 256:
			return ColorMode256
		default:
			return ColorModeTrue
		}
	}
	return ColorModeTrue
}

// StyleRegistry is the name -> tcell.Style registry mirroring urwid's named
// palette. It is owned by App (App.Styles) and populated by Register; widgets
// look up styles via Style. Holding the registry on App (rather than a
// package global) keeps the mutable registry off the package level so
// parallel tests can each use an isolated instance.
type StyleRegistry struct {
	mu     sync.RWMutex
	styles map[string]tcell.Style
}

// newStyleRegistry returns an empty StyleRegistry.
func newStyleRegistry() *StyleRegistry {
	return &StyleRegistry{styles: map[string]tcell.Style{}}
}

// Register resolves every named palette entry for the given theme at the
// given color depth and records it in the registry. If colorMode is
// non-positive, the depth is auto-detected from the terminal via tcell
// (DetectColorMode).
func (r *StyleRegistry) Register(theme, colorMode int) {
	if colorMode <= 0 {
		colorMode = DetectColorMode()
	}
	pal := paletteFor(theme)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.styles = make(map[string]tcell.Style, len(pal))
	for name, e := range pal {
		r.styles[name] = resolveTcellStyle(e, colorMode)
	}
}

// Style returns the registered tcell.Style for a named style, or
// tcell.StyleDefault if the name is unknown.
func (r *StyleRegistry) Style(name string) tcell.Style {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.styles[name]; ok {
		return s
	}
	return tcell.StyleDefault
}
