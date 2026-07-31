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

import "runtime"

// Glyph set name constants, matching Python GLYPHSETS (TextUI.py:127-131).
const (
	GlyphPlain   = "plain"
	GlyphUnicode = "unicode"
	GlyphNerd    = "nerdfont"
)

// GlyphSet maps a glyph name to its rendered string for one glyph set.
type GlyphSet map[string]string

// glyphsPlain is the plain (ASCII) glyph set, transcribed verbatim from
// TextUI.py:142-171 (the second column of each GLYPHS row).
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

// glyphsUnicode is the unicode glyph set, transcribed verbatim from
// TextUI.py:142-171 (the third column of each GLYPHS row).
var glyphsUnicode = GlyphSet{
	"check":           "✓",
	"cross":           "✕",
	"unknown":         "?",
	"encrypted":       "⚿",
	"plaintext":       "!",
	"arrow_r":         "→",
	"arrow_l":         "←",
	"arrow_u":         "↑",
	"arrow_d":         "↓",
	"warning":         "⚠",
	"info":            "ℹ",
	"unread":          "✉",
	"divider1":        "┄",
	"peer":            "Ⓟ ",
	"node":            "Ⓝ ",
	"page":            "▤ ",
	"speed":           "◷ ",
	"decoration_menu": " +",
	"unread_menu":     " ✉",
	"globe":           "",
	"sent":            "↑",
	"papermsg":        "▤",
	"qrcode":          "▤",
	"selected":        "●",
	"unselected":      "○",
	"file":            "▤",
	"image":           "▣",
	"audio":           "♫",
	"pin":             "★",
	"copy":            "⧉",
}

// glyphsNerd is the Nerd Font glyph set, transcribed verbatim from
// TextUI.py:142-171 (the fourth column of each GLYPHS row). The unread and
// unread_menu glyphs are platform-dependent (TextUI.py:133-138): on Darwin
// the original uses the solid envelope U+F0E0, elsewhere the outline
// envelope U+F003. The Darwin values are the map defaults; init flips them
// on non-Darwin platforms.
var glyphsNerd = GlyphSet{
	"check":           "✓",
	"cross":           "✕",
	"unknown":         "?",
	"encrypted":       "",
	"plaintext":       " ",
	"arrow_r":         "→",
	"arrow_l":         "←",
	"arrow_u":         "↑",
	"arrow_d":         "↓",
	"warning":         "",
	"info":            "\U000f064e",
	"unread":          " ",
	"divider1":        "┄",
	"peer":            "",
	"node":            "\U000f0002",
	"page":            " ",
	"speed":           "\U000f04c5 ",
	"decoration_menu": " \U000f043b",
	"unread_menu":     " ",
	"globe":           "",
	"sent":            "\U000f0cd8",
	"papermsg":        "",
	"qrcode":          "",
	"selected":        "●",
	"unselected":      "○",
	"file":            "",
	"image":           "",
	"audio":           "",
	"pin":             "",
	"copy":            "",
}

func init() {
	// Match Python's platform-dependent unread envelope (TextUI.py:133-138).
	// Darwin uses U+F0E0 (the map default); other platforms use U+F003.
	if runtime.GOOS != "darwin" {
		glyphsNerd["unread"] = " "
		glyphsNerd["unread_menu"] = " "
	}
}

// GlyphSets maps glyph set names to their glyph maps, matching Python
// GLYPHSETS (TextUI.py:127-131).
var GlyphSets = map[string]GlyphSet{
	GlyphPlain:   glyphsPlain,
	GlyphUnicode: glyphsUnicode,
	GlyphNerd:    glyphsNerd,
}

// GetGlyphSet returns the glyph set for the given name. An unrecognized name
// falls back to the unicode set, matching Python's default branch
// (TextUI.py:211-212).
func GetGlyphSet(name string) GlyphSet {
	if gs, ok := GlyphSets[name]; ok {
		return gs
	}
	return glyphsUnicode
}

// Glyph returns the rendered string for a single glyph name in the named
// glyph set. If either the set or the glyph is unknown it returns the empty
// string.
func Glyph(setName, glyphName string) string {
	gs, ok := GlyphSets[setName]
	if !ok {
		return ""
	}
	return gs[glyphName]
}
