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

// cursor-coords.go ports urwid's line-wrapping layout + calc_coords so the Go
// port can position the terminal hardware cursor (screen.ShowCursor) on focused
// micron LinkableText pages identically to the Python original.
//
// urwid's LinkableText.render sets canvas.cursor = get_cursor_coords(size),
// which maps the model cursor offset (a char position into the line's text) to
// an (x, y) screen coordinate via urwid.text_layout.calc_coords over the line's
// wrap layout (MicronParser.py:982-992). tview's Application.Draw hides the
// cursor each frame, so the focused widget must re-show it on every draw.
//
// The wrapping reproduced here is urwid's default StandardTextLayout "space"
// mode (break at spaces, break_long_words=True) from urwid/text_layout.py
// (StandardTextLayout._calculate_segments), operating on runes with a
// best-effort East-Asian width model. The golden (x,y) table in
// cursor-coords_test.go is captured from Python urwid calc_coords.

// runeWidth returns the screen-column width of a single rune: 2 for East-Asian
// wide/fullwidth, 0 for combining marks, 1 otherwise. This is a best-effort
// subset of urwid's wcwidth model sufficient for micron page text; emoji ZWJ
// grapheme clusters are not split (treated as their constituent runes).
func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20: // C0 control codes
		return 0
	case isCombining(r):
		return 0
	case isWide(r):
		return 2
	default:
		return 1
	}
}

// isCombining reports whether r is a Unicode combining mark (zero width).
func isCombining(r rune) bool {
	return r >= 0x0300 && r <= 0x036F ||
		r >= 0x1AB0 && r <= 0x1AFF ||
		r >= 0x1DC0 && r <= 0x1DFF ||
		r >= 0x20D0 && r <= 0x20FF ||
		r >= 0xFE20 && r <= 0xFE2F
}

// isWide reports whether r is East-Asian wide or fullwidth (2 columns).
func isWide(r rune) bool {
	return r >= 0x1100 && r <= 0x115F || // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E || // CJK radicals
		r >= 0x3041 && r <= 0x33FF || // Hiragana/Katakana/CJK
		r >= 0x3400 && r <= 0x4DBF || // CJK Ext A
		r >= 0x4E00 && r <= 0x9FFF || // CJK Unified
		r >= 0xA000 && r <= 0xA4CF || // Yi
		r >= 0xAC00 && r <= 0xD7A3 || // Hangul Syllables
		r >= 0xF900 && r <= 0xFAFF || // CJK Compat
		r >= 0xFE30 && r <= 0xFE4F || // CJK Compat Forms
		r >= 0xFF00 && r <= 0xFF60 || // Fullwidth
		r >= 0xFFE0 && r <= 0xFFE6 ||
		r >= 0x20000 && r <= 0x3FFFD // CJK Ext B+
}

// calcWidth returns the screen-column width of text[start:end] (rune offsets).
// Mirrors urwid.str_util.calc_width.
func calcWidth(text []rune, start, end int) int {
	w := 0
	for i := start; i < end; i++ {
		w += runeWidth(text[i])
	}
	return w
}

// isWideCharAt reports whether the rune at offs is wide. Mirrors
// urwid.str_util.is_wide_char (single-rune grapheme model).
func isWideCharAt(text []rune, offs int) bool {
	return runeWidth(text[offs]) == 2
}

// calcTextPos returns the rune offset `pos` whose accumulated display width from
// start is the largest value not exceeding prefCol, plus that width. Mirrors
// urwid.str_util.calc_string_text_pos.
func calcTextPos(text []rune, start, end, prefCol int) (pos, cols int) {
	cols = 0
	pos = start
	for i := start; i < end; i++ {
		w := runeWidth(text[i])
		if w+cols > prefCol {
			return pos, cols
		}
		cols += w
		pos = i + 1
	}
	return end, cols
}

// CalcCoords maps a cursor rune offset `pos` within `text` to an (x, y) screen
// coordinate, given the text is wrapped to `width` columns using urwid's
// "space"-mode layout. This is the Go port of urwid.text_layout.calc_coords,
// used to position the terminal hardware cursor on focused LinkableText pages
// (matching Python LinkableText.get_cursor_coords, MicronParser.py:982-992).
//
// x is the screen column within the wrapped line, y is the wrapped-line index.
// A position past the end clamps to the closest coordinate, matching urwid.
func CalcCoords(text string, width, pos int) (x, y int) {
	runes := []rune(text)
	if width <= 0 {
		return 0, 0
	}
	layout := layoutSegments(runes, width)
	if pos < 0 {
		pos = 0
	}
	var closestX, closestY int
	var closestDist *int
	curY := 0
	for _, line := range layout {
		curX := 0
		for _, s := range line {
			if s.offs == pos {
				return curX, curY
			}
			if s.end >= 0 && s.offs <= pos && pos < s.end {
				return curX + calcWidth(runes, s.offs, pos), curY
			}
			distance := abs(s.offs - pos)
			if s.end >= 0 && s.end < pos {
				distance = pos - (s.end - 1)
			}
			if closestDist == nil || distance < *closestDist {
				d := distance
				closestDist = &d
				closestX, closestY = curX, curY
			}
			curX += s.sc
		}
		curY++
	}
	if closestDist != nil {
		return closestX, closestY
	}
	return 0, 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// layoutSeg is one segment of a wrapped line: a text run (end >= 0) or a
// zero-width "removed character" break hint (end < 0). Mirrors urwid's
// (sc, offs, end) / (sc, offs) layout tuples.
type layoutSeg struct {
	sc   int
	offs int
	end  int // -1 for a zero-width break hint
}

// layoutSegments computes the urwid "space"-mode wrap layout of text into
// screen `width` columns. Mirrors StandardTextLayout._calculate_segments.
func layoutSegments(text []rune, width int) [][]layoutSeg {
	var segments [][]layoutSeg
	idx := 0
	n := len(text)
	for idx <= n {
		nlPos := -1
		for i := idx; i < n; i++ {
			if text[i] == '\n' {
				nlPos = i
				break
			}
		}
		if nlPos == -1 {
			nlPos = n
		}
		screenCols := calcWidth(text, idx, nlPos)
		if screenCols == 0 {
			// removed character hint (blank line / consumed newline)
			segments = append(segments, []layoutSeg{{sc: 0, offs: nlPos, end: -1}})
			idx = nlPos + 1
			continue
		}
		if screenCols <= width {
			segments = append(segments, []layoutSeg{
				{sc: screenCols, offs: idx, end: nlPos},
				{sc: 0, offs: nlPos, end: -1},
			})
			idx = nlPos + 1
			continue
		}
		pos, sc := calcTextPos(text, idx, nlPos, width)
		if pos == idx {
			// pathological width=1 wide-char case; force a one-rune advance
			pos = idx + 1
			sc = runeWidth(text[idx])
		}
		if text[pos] == ' ' {
			// perfect space wrap
			segments = append(segments, []layoutSeg{
				{sc: sc, offs: idx, end: pos},
				{sc: 0, offs: pos, end: -1},
			})
			idx = pos + 1
			continue
		}
		if isWideCharAt(text, pos) {
			// perfect next wide
			segments = append(segments, []layoutSeg{{sc: sc, offs: idx, end: pos}})
			idx = pos
			continue
		}
		// search back for a space or wide boundary to break at
		prev := pos
		broke := false
		for prev > idx {
			prev--
			if text[prev] == ' ' {
				sc2 := calcWidth(text, idx, prev)
				line := []layoutSeg{{sc: 0, offs: prev, end: -1}}
				if idx != prev {
					line = append([]layoutSeg{{sc: sc2, offs: idx, end: prev}}, line...)
				}
				segments = append(segments, line)
				idx = prev + 1
				broke = true
				break
			}
			if isWideCharAt(text, prev) {
				nextChar := prev + 1
				sc2 := calcWidth(text, idx, nextChar)
				segments = append(segments, []layoutSeg{{sc: sc2, offs: idx, end: nextChar}})
				idx = nextChar
				broke = true
				break
			}
		}
		if broke {
			continue
		}
		// no space found: unwrap previous line's removed space if possible
		if next, ok := unwrapPrevSpace(text, &segments, width, nlPos); ok {
			idx = next
			continue
		}
		// force any-char wrap
		segments = append(segments, []layoutSeg{{sc: sc, offs: idx, end: pos}})
		idx = pos
	}
	return segments
}

// unwrapPrevSpace implements the "unwrap previous line space if possible to fit
// more text (we're breaking a word anyway)" branch of _calculate_segments. On
// success it returns the new idx to resume from; otherwise (-1, false).
func unwrapPrevSpace(text []rune, segments *[][]layoutSeg, width, nlPos int) (int, bool) {
	last := len(*segments) - 1
	if last < 0 {
		return -1, false
	}
	line := (*segments)[last]
	var pSc, pOff, hSc, hOff int
	hasPrev := false
	if len(line) == 2 {
		// [(p_sc, p_off, p_end), (h_sc, h_off)]
		pSc, pOff = line[0].sc, line[0].offs
		hSc, hOff = line[1].sc, line[1].offs
		hasPrev = true
	} else if len(line) == 1 && line[0].end < 0 {
		// [(h_sc, h_off)]  (single removed-char hint)
		hSc, hOff = line[0].sc, line[0].offs
		pSc = 0
		pOff = hOff
		hasPrev = true
	}
	if !hasPrev {
		return -1, false
	}
	if pSc < width && hSc == 0 && hOff < len(text) && text[hOff] == ' ' {
		// combine with the previous line
		*segments = (*segments)[:last]
		idx := pOff
		pos, sc := calcTextPos(text, idx, nlPos, width)
		newLine := []layoutSeg{{sc: sc, offs: idx, end: pos}}
		if idx < len(text) && (text[idx] == ' ' || text[idx] == '\n') {
			newLine = append(newLine, layoutSeg{sc: 0, offs: idx, end: -1})
			idx++
		}
		*segments = append(*segments, newLine)
		idx = pos
		return idx, true
	}
	return -1, false
}
