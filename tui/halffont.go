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
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// halffont_glyph is one parsed entry of urwid's HalfBlock5x4Font: a glyph that
// is W columns wide and 4 character rows tall (each row exactly W runes,
// trailing spaces included so glyphs concatenate without misalignment).
type halffontGlyph struct {
	W    int      `json:"w"`
	Rows []string `json:"rows"`
}

//go:embed halffont_data.json
var halffontDataJSON []byte

var (
	halffontOnce   sync.Once
	halffontGlyphs map[rune]halffontGlyph
	halffontHeight = 4
)

// loadHalffont parses the embedded glyph table (captured from urwid's
// HalfBlock5x4Font — the read-only Python source of truth) on first use.
func loadHalffont() {
	var raw map[string]halffontGlyph
	if err := json.Unmarshal(halffontDataJSON, &raw); err != nil {
		panic("tui: invalid halffont_data.json: " + err.Error())
	}
	halffontGlyphs = make(map[rune]halffontGlyph, len(raw))
	for ch, g := range raw {
		if len(g.Rows) != halffontHeight {
			continue
		}
		halffontGlyphs[[]rune(ch)[0]] = g
	}
}

// halfBlock5x4Render renders text as 4-row "big text" using urwid's
// HalfBlock5x4Font, matching urwid.BigText.render (big_text.py:60-86): each
// character with a non-zero-width glyph is concatenated horizontally; unknown
// characters contribute nothing. The returned slice has exactly 4 strings
// (one per font row), each the concatenation of the per-glyph rows (so trailing
// spaces are preserved). Callers may rstrip each line for display.
func halfBlock5x4Render(s string) []string {
	halffontOnce.Do(loadHalffont)

	rows := make([]string, halffontHeight)
	for _, ch := range s {
		g, ok := halffontGlyphs[ch]
		if !ok || g.W == 0 {
			continue
		}
		for r := range halffontHeight {
			rows[r] += g.Rows[r]
		}
	}
	return rows
}

// halfBlock5x4Width returns the rendered width of s in columns (the sum of each
// character's glyph width, ignoring unknown characters), matching
// urwid.BigText.pack (big_text.py:42-49).
func halfBlock5x4Width(s string) int {
	halffontOnce.Do(loadHalffont)
	w := 0
	for _, ch := range s {
		if g, ok := halffontGlyphs[ch]; ok {
			w += g.W
		}
	}
	return w
}

// halfBlock5x4RenderTrimmed is halfBlock5x4Render with each line's trailing
// spaces removed (how the splash is displayed: the big text is left-aligned in
// its box and trailing blank columns are not visually meaningful).
func halfBlock5x4RenderTrimmed(s string) []string {
	rows := halfBlock5x4Render(s)
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return rows
}
