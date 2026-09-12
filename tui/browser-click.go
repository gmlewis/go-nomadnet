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
	"unicode/utf8"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// Click focus model: a left click inside the page body moves the page focus to
// the line (and part) the click landed on, and — when the click lands inside a
// Micron text field — puts the caret in that field so the visitor can type
// straight into it.
//
// Python gets this from urwid's widget tree: Pile.mouse_event focuses the row
// under the press (pile.py:1059-1084, "May change focus on button 1 press"),
// Columns focuses the column under it, LinkableText.mouse_event records the
// clicked part cursor (MicronParser.py:1005-1044, `self._cursor_position = pos`)
// and Edit.mouse_event moves the edit cursor to the clicked cell (edit.py:547-
// 568, `move_cursor_to_coords`). The Go page body is a single tview.TextView
// whose form fields are drawn as off-tree overlays (see renderedField), so no
// widget sits under the mouse and the click had no effect at all on a field:
// the field could only be reached with Down/Up. These helpers restore the
// urwid behavior by hit-testing the rendered geometry.

// lineAtScreenY maps a screen row to the rendered page line drawn there plus
// the wrapped row within that line. It inverts the row math the nav model uses
// to place the hardware cursor and the field overlays (rowsAbove +
// lineRowCount for the line, plus the content's scroll offset), so a click
// resolves to exactly the line the glyphs under it belong to.
func (bd *BrowserDisplay) lineAtScreenY(y int) (line, wrappedRow int, ok bool) {
	_, y0, _, h := bd.content.GetInnerRect()
	if h <= 0 {
		return 0, 0, false
	}
	abs := y - y0
	if abs < 0 || abs >= h {
		return 0, 0, false
	}
	scrollRow, _ := bd.content.GetScrollOffset()
	abs += scrollRow
	for i := 0; i < len(bd.currentLines); i++ {
		rows := max(bd.lineRowCount(i), 1)
		if abs < rows {
			return i, abs, true
		}
		abs -= rows
	}
	return 0, 0, false
}

// linePosAtScreenX maps a screen column to the rune offset within the line's
// plain text at the given wrapped row. It inverts cursorScreenXY: the row's
// glyphs start after the line indent and the per-row alignment pad, and the
// text wraps at width-2*indent (the same tview.WordWrap both models use).
func (bd *BrowserDisplay) linePosAtScreenX(line, wrappedRow, x int) int {
	plain := bd.linePlainText(line)
	x0, _, width, _ := bd.content.GetInnerRect()
	if width <= 0 {
		width = bd.contentWidth()
	}
	if width <= 0 {
		return 0
	}
	indent, align := 0, micron.AlignLeft
	if line >= 0 && line < len(bd.currentLines) && bd.currentLines[line] != nil {
		indent = bd.currentLines[line].Indent
		align = bd.currentLines[line].Align
	}
	wrapW := width - 2*indent
	if wrapW <= 0 {
		wrapW = width
	}
	col := x - x0 - indent
	if col < 0 {
		return 0
	}
	runes := []rune(plain)
	rows := tview.WordWrap(plain, wrapW)
	off := 0
	for yi, row := range rows {
		rowLen := utf8.RuneCountInString(row)
		if yi == wrappedRow || yi == len(rows)-1 {
			col -= alignPad(align, wrapW, rowContentWidth(row, yi == len(rows)-1))
			if col < 0 {
				return off
			}
			w := 0
			for i := off; i < off+rowLen && i < len(runes); i++ {
				if w >= col {
					return i
				}
				w += runeWidth(runes[i])
			}
			return min(off+rowLen, len(runes))
		}
		off += rowLen
	}
	return len(runes)
}

// fieldAtScreen returns the text field whose drawn rect contains the screen
// cell (x, y), or nil when the click misses every field. The rect is the one
// drawFieldEditor paints: the field's start column plus its declared width on
// the field line's first wrapped row, clipped to the content pane.
func (bd *BrowserDisplay) fieldAtScreen(x, y int) *renderedField {
	line, wrappedRow, ok := bd.lineAtScreenY(y)
	if !ok || line >= len(bd.lineFields) {
		return nil
	}
	x0, _, innerW, _ := bd.content.GetInnerRect()
	if innerW <= 0 {
		return nil
	}
	col := x - x0
	for _, rf := range bd.lineFields[line] {
		if rf.editor == nil {
			// Checkbox/radio fields hold no text caret: they toggle with
			// Enter/Space on their line (toggleFieldAtCursor).
			continue
		}
		// A field's box lives on its own wrapped row, so a click only lands in it
		// when that is the row under the mouse.
		if rf.rowOffset != wrappedRow {
			continue
		}
		if col >= rf.startCol && col < rf.startCol+rf.width {
			return rf
		}
	}
	return nil
}

// clickFocus moves the page focus (and the caret) to the line under a left
// click, mirroring urwid's Pile/Columns/LinkableText/Edit mouse handling. A
// click inside a text field's rect mounts that field's editor as the focused
// one and places the caret on the clicked cell (clamped to the text, as
// urwid's Edit.move_cursor_to_coords does); a click on any other line moves the
// line focus and the part cursor there. Returns true when the click changed
// focus, i.e. when it landed on a selectable line inside the page body.
func (bd *BrowserDisplay) clickFocus(x, y int) bool {
	line, wrappedRow, ok := bd.lineAtScreenY(y)
	if !ok || line >= len(bd.lineCursors) {
		return false
	}
	rf := bd.fieldAtScreen(x, y)
	if rf == nil && !bd.selectableLine(line) {
		// Python's Pile moves focus only to a selectable row (pile.py:1083-
		// 1084 `w.selectable()`), and urwid.Text — a blank line — is not
		// selectable, so clicking blank space changes nothing.
		return false
	}
	pos := bd.linePosAtScreenX(line, wrappedRow, x)
	focusChanged := line != bd.focusLine || pos != bd.lineCursors[line]

	bd.focusLine = line
	bd.lineCursors[line] = pos
	// The click is a focus move for the page body: stamp it so the hardware
	// cursor is drawn at the clicked cell (same visibility window every nav key
	// starts).
	bd.stampKeypress()
	bd.ensureVisible()
	bd.peekLink()
	bd.syncFieldFocus()

	if rf != nil {
		bd.focusFieldCaret(rf, x)
	}
	if bd.app != nil {
		bd.app.SetFocus(bd.content)
		// The click moved the focus and/or the field caret, so ask for a frame:
		// a field editor is off-tree and does not invalidate the screen itself,
		// and without this frame the terminal cursor would stay where the
		// previous frame left it.
		bd.app.QueueUpdateDraw(func() {})
	}
	return focusChanged
}

// focusFieldCaret places a clicked field's editor caret on the clicked cell.
// rf is the field under the click and x the click's screen column; the caret is
// clamped to the text length, so clicking the blank area right of a short value
// puts the caret at its end (urwid Edit.mouse_event → move_cursor_to_coords).
func (bd *BrowserDisplay) focusFieldCaret(rf *renderedField, x int) {
	if rf.editor == nil {
		return
	}
	x0, _, _, _ := bd.content.GetInnerRect()
	cell := max(x-x0-rf.startCol, 0)
	limit := len([]rune(rf.editor.GetText()))
	rf.editor.SetCursorPos(min(cell, limit))
	bd.lineCursors[bd.focusLine] = rf.runeStart + rf.editor.CursorPos()
}
