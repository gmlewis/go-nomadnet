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

// Multiline editing for ReadlineEdit, mirroring the source-of-truth urwid
// Edit(wrap="space") behavior that Python nomadnet's multiline editors
// inherit (RoomMessageEdit Channels.py:413, MessageEdit Conversations.py).
// The goldens in editor-multiline-parity_test.go were captured live from
// urwid 4.0.3, the version the installed Python nomadnet runs on.

// editorRow is one wrapped display row of a multiline editor: the rune range
// [start, end) of its rendered content in the edit buffer, plus tail — the
// zero-width break offset urwid appends to the row's layout (the dropped
// break space, the newline, or the end of the text). A buffer position equal
// to tail renders at that row's end column. tail < 0 marks a hard-broken row
// (cut at the fill column), which carries no zero-width segment and whose
// break position resolves into the following row instead.
type editorRow struct {
	start int
	end   int
	tail  int
}

// editorRows wraps the edit buffer exactly like urwid's space layout
// (urwid text_layout.py:240-352, the algorithm behind Edit(wrap="space")):
// each line splits on newline (the newline is dropped), fills to width
// columns, breaks at a space (dropping that one space), walks back to the
// previous space otherwise, and hard-breaks mid-word when a line has no
// space. Trailing spaces stay inside the row they fit; an empty line or
// buffer yields one empty row. This is urwidWrapSegment without the
// trailing-space trim that the styled shortcut bar needs.
func editorRows(text []rune, width int) []editorRow {
	if width <= 0 {
		return []editorRow{{0, len(text), len(text)}}
	}
	var rows []editorRow
	segStart := 0
	for segStart <= len(text) {
		segEnd := segStart
		for segEnd < len(text) && text[segEnd] != '\n' {
			segEnd++
		}
		if segStart == segEnd {
			// An empty line (or empty buffer) is one empty display row
			// (urwid: [(0, offs)] — a zero-width segment).
			rows = append(rows, editorRow{segStart, segStart, segStart})
		} else {
			rows = append(rows, editorWrapSegmentRows(text, segStart, segEnd, width)...)
		}
		if segEnd >= len(text) {
			break
		}
		segStart = segEnd + 1 // skip the newline
	}
	return rows
}

// editorWrapSegmentRows wraps one newline-free segment [start, end) of text,
// returning its display rows. Mirrors urwidWrapSegment but keeps trailing
// spaces in the row content and records the break offset as the tail.
func editorWrapSegmentRows(text []rune, idx, segEnd, width int) []editorRow {
	var rows []editorRow
	for idx < segEnd {
		remW := 0
		for _, r := range text[idx:segEnd] {
			remW += cellWidth(r)
		}
		if remW <= width {
			rows = append(rows, editorRow{idx, segEnd, segEnd})
			return rows
		}
		pos, _ := urwidCalcTextPosBounded(text, idx, segEnd, width)
		if pos == idx {
			pos = idx + 1 // pathological: a rune wider than width
		}
		if text[pos] == ' ' {
			// Perfect space wrap: break here, drop the break space.
			rows = append(rows, editorRow{idx, pos, pos})
			idx = pos + 1
			continue
		}
		prev := -1
		for p := pos - 1; p > idx; p-- {
			if text[p] == ' ' {
				prev = p
				break
			}
		}
		if prev >= 0 {
			// Break at the previous space; the break space itself is
			// dropped and earlier spaces of the run stay in the row.
			rows = append(rows, editorRow{idx, prev, prev})
			idx = prev + 1
			continue
		}
		// No space on this line: hard break at the fill column (any-wrap).
		rows = append(rows, editorRow{idx, pos, -1})
		idx = pos
	}
	if idx == segEnd && len(rows) > 0 {
		// A space break that consumed the whole segment still leaves an
		// empty display row for the exhausted remainder (urwid's layout of
		// "abc  " @4: [(4,0,4),(0,4)],[(0,5)]).
		rows = append(rows, editorRow{segEnd, segEnd, segEnd})
	}
	return rows
}

// editorRowWidth returns the screen-column width of a row's content.
func editorRowWidth(text []rune, row editorRow) int {
	w := 0
	for _, r := range text[row.start:row.end] {
		w += cellWidth(r)
	}
	return w
}

// editorCalcCoords mirrors urwid text_layout.calc_coords: the (x, y) display
// coordinates of buffer position pos. A position inside a row's content maps
// to its column; a position at a row's break offset (tail — the dropped
// space, a newline, or the end of the text) renders at that row's end;
// positions inside a gap resolve to the closer neighbor row (urwid's
// closest-segment fallback, earlier rows winning ties).
func editorCalcCoords(text []rune, rows []editorRow, pos int) (int, int) {
	for y, row := range rows {
		if row.start <= pos && pos < row.end {
			x := 0
			for _, r := range text[row.start:pos] {
				x += cellWidth(r)
			}
			return x, y
		}
	}
	for y, row := range rows {
		if row.tail >= 0 && pos == row.tail {
			return editorRowWidth(text, row), y
		}
	}
	found := false
	bestD, bx, by := 0, 0, 0
	for y, row := range rows {
		if pos < row.start {
			if d := row.start - pos; !found || d < bestD {
				found, bestD, bx, by = true, d, 0, y
			}
		} else if pos > row.end {
			if d := pos - (row.end - 1); !found || d < bestD {
				found, bestD, bx, by = true, d, editorRowWidth(text, row), y
			}
		}
	}
	if found {
		return bx, by
	}
	return 0, 0
}

// urwidCalcTextPosBounded returns the position at which width screen columns
// have been consumed scanning text[start:end), mirroring urwid
// calc_text_pos: a wide rune that would cross the boundary stops before it.
func urwidCalcTextPosBounded(text []rune, start, end, width int) (pos, cols int) {
	w := 0
	pos = start
	for p := start; p < end; p++ {
		cw := cellWidth(text[p])
		if w+cw > width {
			return p, w
		}
		w += cw
		pos = p + 1
		cols = w
	}
	return pos, cols
}

// editorCalcPos mirrors urwid text_layout.calc_pos for an integer preferred
// column: the exact position at/after pref columns within the row, the row's
// break offset when pref passes its width (urwid's zero-width segment wins),
// or — on a hard-broken row, which carries no such segment — the position of
// the row's last character.
func editorCalcPos(text []rune, rows []editorRow, pref int, row int) int {
	if row < 0 || row >= len(rows) {
		return 0
	}
	r := rows[row]
	w := editorRowWidth(text, r)
	if pref >= 0 && pref < w {
		pos, _ := urwidCalcTextPosBounded(text, r.start, r.end, pref)
		return min(pos, r.end)
	}
	if r.tail >= 0 {
		return r.tail
	}
	pos, _ := urwidCalcTextPosBounded(text, r.start, r.end, w-1)
	return pos
}

// editorCalcPosLeft mirrors urwid _calc_literal_line_pos "left" (Home /
// Align.LEFT): the row's first content offset.
func editorCalcPosLeft(text []rune, rows []editorRow, row int) int {
	if row < 0 || row >= len(rows) {
		return 0
	}
	return rows[row].start
}

// editorCalcPosRight mirrors urwid _calc_literal_line_pos "right" (End /
// Align.RIGHT): the row's zero-width break offset (its tail), or the last
// character position on a hard-broken row.
func editorCalcPosRight(text []rune, rows []editorRow, row int) int {
	if row < 0 || row >= len(rows) {
		return 0
	}
	r := rows[row]
	if r.tail >= 0 {
		return r.tail
	}
	pos, _ := urwidCalcTextPosBounded(text, r.start, r.end, editorRowWidth(text, r)-1)
	return pos
}

// Preferred-column sentinels for urwid's Align.LEFT / Align.RIGHT literals
// (Home/End store them as the preferred column for the next vertical move,
// urwid Edit.keypress MAX_LEFT/MAX_RIGHT → pref_col_maxcol).
const (
	prefColLeft  = -2
	prefColRight = -3
)

// prefColFor mirrors urwid Edit.get_pref_col: the stored preferred column
// when it was set at this width, else the current cursor column.
func (re *ReadlineEdit) prefColFor(w, x int) int {
	if re.prefColW != w {
		return x
	}
	return re.prefCol
}

// SetMultiline switches the editor between tview's single-line behavior and
// the urwid-parity multiline editor (wrapping rows, Enter inserting a
// newline, wrapped-row cursor navigation). Single-line editors are
// unaffected; multiline is only for the editors Python declares
// multiline=True.
func (re *ReadlineEdit) SetMultiline(v bool) {
	re.multiline = v
}

// MultilineRows reports the wrapped row count the multiline editor needs at
// the given width (minimum 1) — the urwid Edit flow footer's rows(). The
// room composer's layout sizes the footer to this count.
func (re *ReadlineEdit) MultilineRows(width int) int {
	if !re.multiline {
		return 1
	}
	rows := editorRows([]rune(re.GetText()), width)
	return max(len(rows), 1)
}

// multilineCoords returns the caret's (x, y) display coordinates at the
// current width, as urwid get_cursor_coords sees them: the caret column is
// clamped to the last column of the row's shifted window when it lands past
// the row's fill column.
func (re *ReadlineEdit) multilineCoords(runes []rune, width int) (int, int) {
	rows := editorRows(runes, width)
	pos := min(max(re.cursorPos, 0), len(runes))
	x, y := editorCalcCoords(runes, rows, pos)
	if width > 0 && x >= width {
		x = width - 1
	}
	return x, y
}

// multilineVertical moves the cursor one wrapped row up or down, mirroring
// urwid Edit.keypress UP/DOWN: the preferred column targets the new row, the
// achieved column is stored for the next vertical move, and an out-of-range
// target returns the key to the parent (which handles the focus jump).
func (re *ReadlineEdit) multilineVertical(event *tcell.EventKey) *tcell.EventKey {
	_, _, w, _ := re.GetInnerRect()
	runes := []rune(re.GetText())
	rows := editorRows(runes, w)
	x, y := re.multilineCoords(runes, w)
	down := event.Key() == tcell.KeyDown
	if !down {
		if y == 0 {
			// Python RoomMessageEdit.keypress (Channels.py:429-434): Up on
			// the top wrapped row jumps focus to the message body.
			if re.OnFocusTopRow != nil {
				re.killRing.resetChain()
				re.OnFocusTopRow()
			}
			return nil
		}
		y--
	} else {
		y++
		if y >= len(rows) {
			// urwid returns the key to the parent at the last row.
			re.killRing.resetChain()
			return event
		}
	}
	pref := re.prefColFor(w, x)
	var pos int
	switch pref {
	case prefColLeft:
		pos = editorCalcPosLeft(runes, rows, y)
		re.prefCol, re.prefColW = prefColLeft, w
	case prefColRight:
		pos = editorCalcPosRight(runes, rows, y)
		re.prefCol, re.prefColW = prefColRight, w
	default:
		pos = editorCalcPos(runes, rows, pref, y)
		re.prefCol, re.prefColW = pref, w
	}
	re.cursorPos = pos
	return nil
}

// multilineRowKeys handles Home/End (urwid MAX_LEFT/MAX_RIGHT): the bounds
// of the current WRAPPED row, storing the left/right literal as the
// preferred column exactly like urwid's move_cursor_to_coords.
func (re *ReadlineEdit) multilineRowKeys(event *tcell.EventKey) bool {
	_, _, w, _ := re.GetInnerRect()
	runes := []rune(re.GetText())
	rows := editorRows(runes, w)
	_, y := re.multilineCoords(runes, w)
	switch event.Key() {
	case tcell.KeyHome:
		re.cursorPos = editorCalcPosLeft(runes, rows, y)
		re.prefCol, re.prefColW = prefColLeft, w
	case tcell.KeyEnd:
		re.cursorPos = editorCalcPosRight(runes, rows, y)
		re.prefCol, re.prefColW = prefColRight, w
	default:
		return false
	}
	return true
}

// drawMultiline renders the multiline editor like urwid Edit(wrap="space")
// inside its AttrMap: the field background fills the whole area, each
// wrapped row draws its content in the field foreground, and the hardware
// caret sits on its wrapped row/column — shifted into view together with
// that row's window when the caret lands past the row's fill column (urwid
// get_line_translation's shift_line).
func (re *ReadlineEdit) drawMultiline(screen tcell.Screen) {
	x, y, w, h := re.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	fg, bg, _ := re.GetFieldStyle().Decompose()
	style := tcell.StyleDefault.Background(bg).Foreground(fg)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, style)
		}
	}
	runes := []rune(re.GetText())
	rows := editorRows(runes, w)
	pos := min(max(re.cursorPos, 0), len(runes))
	cx, cy := editorCalcCoords(runes, rows, pos)
	shift := 0
	if cx >= w {
		shift = cx - w + 1
	}
	for rowIdx, row := range rows {
		if rowIdx >= h {
			break
		}
		s := 0
		if rowIdx == cy && shift > 0 {
			s = shift
		}
		for col, r := range runes[row.start+s : row.end] {
			screen.SetContent(x+col, y+rowIdx, r, nil, style)
		}
	}
	if re.HasFocus() {
		caretCol := cx
		if shift > 0 {
			caretCol = w - 1
		}
		if caretCol >= 0 && caretCol < w && cy < h {
			screen.ShowCursor(x+caretCol, y+cy)
		}
	}
}
