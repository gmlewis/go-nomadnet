// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// guideCursorKeyTimeout mirrors Python LinkableText.key_timeout (MicronParser.py
// :910): the hardware cursor is shown only within this long of a keypress.
const guideCursorKeyTimeout = 2 * time.Second

// computeFocusLayout rebuilds the focus-model tables from the current rendered
// lines: selectable lists the StyledLine indices that are focusable (every
// text line when a url_delegate is set, i.e. here in the Guide; headings,
// dividers and blank lines are non-selectable, matching urwid's
// markup_to_attrmaps which wraps them in plain urwid.Text/Divider), and
// lineRows[i] is the wrapped display-row offset where line i begins
// (lineRows[len] is the total row count). It mirrors Python's Pile layout:
// Down advances focus through `selectable` one entry per press, so the number
// of Downs to reach the bottom equals the selectable count (B2 root cause).
//
// It is a no-op when no topic is rendered (currentIdx < 0).
func (gd *GuideDisplay) computeFocusLayout() {
	gd.selectable = gd.selectable[:0]
	gd.lineRows = gd.lineRows[:0]
	if len(gd.currentLines) == 0 {
		gd.focusSel = -1
		return
	}
	w := gd.readerWidth()
	row := 0
	for i, line := range gd.currentLines {
		gd.lineRows = append(gd.lineRows, row)
		if gd.isSelectableLine(line) {
			gd.selectable = append(gd.selectable, i)
		}
		rows := 1
		if w > 0 && i < len(gd.lineTexts) {
			if n := len(tview.WordWrap(gd.lineTexts[i], w)); n > 0 {
				rows = n
			}
		}
		row += rows
	}
	gd.lineRows = append(gd.lineRows, row) // sentinel: total rows
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		gd.focusSel = -1
	}
}

// isSelectableLine reports whether a rendered line is focusable. Headings
// (urwid.Text in a heading AttrMap), dividers (urwid.Divider) and blank lines
// (urwid.Text("")) are non-selectable; everything else — text lines with or
// without links, and field/checkbox/radio lines — is selectable. This matches
// Python markup_to_attrmaps with a url_delegate set (MicronParser.py:404-407),
// which builds a LinkableText (selectable) for every text line.
func (gd *GuideDisplay) isSelectableLine(line *micron.StyledLine) bool {
	if line == nil {
		return false
	}
	if line.Divider || line.HeadingLevel > 0 {
		return false
	}
	// A blank line renders as urwid.Text("") (non-selectable).
	blank := true
	for _, s := range line.Spans {
		if s.Text != "" {
			blank = false
			break
		}
	}
	return !blank
}

// resetFocus restarts the focus cursor at the first selectable line, mirroring
// Python's set_content_widgets rebuilding a fresh Pile (focus at the top) on
// every topic switch. Called from showTopic.
func (gd *GuideDisplay) resetFocus() {
	gd.computeFocusLayout()
	if len(gd.selectable) > 0 {
		gd.focusSel = 0
	} else {
		gd.focusSel = -1
	}
	gd.focusCol = 0
}

// setFocusAtOrAfter moves focus to the first selectable line whose StyledLine
// index is >= lineIdx (the anchor's line), mirroring the Pile focus following
// a jump_to_anchor scroll. If none, focus is left unchanged.
func (gd *GuideDisplay) setFocusAtOrAfter(lineIdx int) {
	gd.computeFocusLayout()
	for i, li := range gd.selectable {
		if li >= lineIdx {
			gd.focusSel = i
			gd.focusCol = 0
			return
		}
	}
}

// noteKey records a keypress timestamp for the cursor key-timeout (B5).
func (gd *GuideDisplay) noteKey() {
	gd.lastKey = time.Now()
	gd.hasKey = true
}

// focusDown advances focus to the next selectable line (skipping non-selectable
// headings/dividers/blanks) and scrolls the reader to keep it visible,
// mirroring urwid Pile+Scrollable: focus moves within the visible area first
// (no scroll), and only scrolls once the focused line would leave the viewport.
func (gd *GuideDisplay) focusDown() {
	gd.noteKey()
	gd.computeFocusLayout()
	if len(gd.selectable) == 0 {
		return
	}
	if gd.focusSel < 0 {
		gd.focusSel = 0
	} else if gd.focusSel+1 < len(gd.selectable) {
		gd.focusSel++
	} else {
		return // at the last selectable: a no-op (Python Pile returns "down" unhandled)
	}
	gd.focusCol = 0
	gd.scrollFocusVisible()
}

// focusUp moves focus to the previous selectable line. Up does NOT escape to
// the menu (the Guide reader releases focus via Left, not Up); at the first
// selectable it is a no-op.
func (gd *GuideDisplay) focusUp() {
	gd.noteKey()
	gd.computeFocusLayout()
	if len(gd.selectable) == 0 {
		return
	}
	if gd.focusSel < 0 {
		gd.focusSel = 0
	} else if gd.focusSel > 0 {
		gd.focusSel--
	} else {
		return // at the first selectable: clamp, no menu escape
	}
	gd.focusCol = 0
	gd.scrollFocusVisible()
}

// focusRight moves the within-line cursor to the next part boundary. At the
// last part it advances to the next selectable line (mirroring LinkableText
// right → "down" when not in_columns, MicronParser.py:944-947). Returns true if
// the key was consumed (cursor moved within the line or to the next line).
func (gd *GuideDisplay) focusRight() bool {
	gd.noteKey()
	gd.computeFocusLayout()
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return false
	}
	positions := gd.focusedPartPositions()
	for _, p := range positions {
		if p > gd.focusCol {
			gd.focusCol = p
			return true
		}
	}
	// At the last part: wrap to the next selectable line (cursor at 0).
	if gd.focusSel+1 < len(gd.selectable) {
		gd.focusSel++
		gd.focusCol = 0
		gd.scrollFocusVisible()
		return true
	}
	return false
}

// focusLeft moves the within-line cursor to the previous part boundary. At
// position 0 it returns false so the caller releases focus back to the topic
// list (LinkableText left at pos 0 → micron_released_focus, MicronParser.py
// :958-961). Returns true if the cursor moved (key consumed).
func (gd *GuideDisplay) focusLeft() bool {
	gd.noteKey()
	gd.computeFocusLayout()
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return false
	}
	if gd.focusCol > 0 {
		prev := 0
		for _, p := range gd.focusedPartPositions() {
			if p < gd.focusCol {
				prev = p
			}
		}
		gd.focusCol = prev
		return true
	}
	return false // at position 0: release focus to the topic list
}

// focusActivate dispatches the link at the within-line cursor to handleLink,
// mirroring LinkableText ACTIVATE (MicronParser.py:937-941). A plain (non-link)
// part is a no-op.
func (gd *GuideDisplay) focusActivate() {
	gd.noteKey()
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return
	}
	line := gd.focusedLine()
	if line == nil {
		return
	}
	total := 0
	for _, s := range line.Spans {
		if total <= gd.focusCol && gd.focusCol < total+len(s.Text) {
			if s.Link != nil {
				gd.handleLink(s.Link.URL, s.Link.Fields)
			}
			return
		}
		total += len(s.Text)
	}
}

// focusedLine returns the StyledLine of the currently focused selectable line,
// or nil.
func (gd *GuideDisplay) focusedLine() *micron.StyledLine {
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return nil
	}
	idx := gd.selectable[gd.focusSel]
	if idx < 0 || idx >= len(gd.currentLines) {
		return nil
	}
	return gd.currentLines[idx]
}

// focusedPartPositions returns the cumulative part-position table of the focused
// line's spans (0 followed by each span's length + running total), mirroring
// LinkableText.keypress's part_positions (MicronParser.py:921-929).
func (gd *GuideDisplay) focusedPartPositions() []int {
	line := gd.focusedLine()
	if line == nil {
		return []int{0}
	}
	pos := []int{0}
	total := 0
	for _, s := range line.Spans {
		total += len(s.Text)
		pos = append(pos, total)
	}
	return pos
}

// scrollFocusVisible adjusts the reader scroll so the focused line is visible,
// mirroring urwid Scrollable: when the focused line is within the viewport the
// offset is unchanged (focus moves within the visible area); when it would be
// above or below the viewport the offset shifts just enough to show it (below:
// the focused line lands on the bottom row).
func (gd *GuideDisplay) scrollFocusVisible() {
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return
	}
	idx := gd.selectable[gd.focusSel]
	if idx < 0 || idx >= len(gd.lineRows) {
		return
	}
	row := gd.lineRows[idx]
	scrollOff, _ := gd.reader.GetScrollOffset()
	_, _, _, h := gd.reader.GetInnerRect()
	if h <= 0 {
		return
	}
	switch {
	case row < scrollOff:
		gd.reader.ScrollTo(row, 0)
	case row >= scrollOff+h:
		gd.reader.ScrollTo(row-h+1, 0)
	}
}

// cursorRow returns the focused line's display row relative to the current
// scroll offset (the row the hardware cursor sits on within the reader). Used
// by tests to assert the cursor advances per Down (B5 precondition).
func (gd *GuideDisplay) cursorRow() int {
	gd.computeFocusLayout()
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return -1
	}
	idx := gd.selectable[gd.focusSel]
	if idx < 0 || idx >= len(gd.lineRows) {
		return -1
	}
	scrollOff, _ := gd.reader.GetScrollOffset()
	return gd.lineRows[idx] - scrollOff
}

// cursorScreenXY returns the terminal (x, y) for the hardware cursor on the
// focused line and within-line cursor column, mirroring Python LinkableText
// .render setting canvas.cursor = get_cursor_coords (MicronParser.py:982-992).
// x = inner x + left indent + display width of the span text before the cursor
// (the indent is the Padding the LinkableText sits in, not part of its text);
// y = inner y + (focused display row − scroll offset). Returns ok=false when no
// line is focused or the cursor is off-screen.
func (gd *GuideDisplay) cursorScreenXY() (x, y int, ok bool) {
	gd.computeFocusLayout()
	line := gd.focusedLine()
	if line == nil {
		return 0, 0, false
	}
	idx := gd.selectable[gd.focusSel]
	if idx < 0 || idx >= len(gd.lineRows) {
		return 0, 0, false
	}
	ix, iy, _, h := gd.reader.GetInnerRect()
	if h <= 0 {
		return 0, 0, false
	}
	scrollOff, _ := gd.reader.GetScrollOffset()
	relY := gd.lineRows[idx] - scrollOff
	if relY < 0 || relY >= h {
		return 0, 0, false
	}
	// Display width of the span text preceding the cursor (the indent is the
	// Padding the line sits in, added separately to match Python's model).
	col := 0
	total := 0
	for _, s := range line.Spans {
		if total >= gd.focusCol {
			break
		}
		col += runewidth.StringWidth(s.Text)
		total += len(s.Text)
	}
	return ix + line.Indent + col, iy + relY, true
}

// cursorVisible reports whether the hardware cursor should be shown, mirroring
// LinkableText's render condition (MicronParser.py:982-992): visible only when
// focused, and only within key_timeout of the last keypress.
func (gd *GuideDisplay) cursorVisible(now time.Time, focused bool) bool {
	if !focused {
		return false
	}
	if !gd.hasKey {
		return false
	}
	return now.Before(gd.lastKey.Add(guideCursorKeyTimeout))
}

// handleReaderKey processes a key while the reader is focused, applying the
// focus model (B2/B5). It returns nil when the key is consumed and the event
// to let tview handle otherwise (PageUp/Down/Home/End scroll the underlying
// TextView, which Python's Guide reader also leaves to the Scrollable — the
// known "scroll then arrow jumps back" behavior noted in GuideColumns.keypress).
func (gd *GuideDisplay) handleReaderKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev == nil {
		return nil
	}
	switch ev.Key() {
	case tcell.KeyDown:
		gd.focusDown()
		return nil
	case tcell.KeyUp:
		gd.focusUp()
		return nil
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		gd.focusActivate()
		return nil
	case tcell.KeyRight:
		gd.focusRight()
		return nil
	case tcell.KeyLeft:
		if gd.focusLeft() {
			return nil
		}
		return ev // at position 0: caller releases focus to the topic list
	}
	return ev
}

// plainLineText returns the concatenated span text of a styled line (no tview
// tags), used for cursor-column width math. Unused outside tests/diagnostics
// but kept for completeness of the model.
func plainLineText(line *micron.StyledLine) string {
	if line == nil {
		return ""
	}
	var b strings.Builder
	for _, s := range line.Spans {
		b.WriteString(s.Text)
	}
	return b.String()
}
