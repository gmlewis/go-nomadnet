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
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// cursorKeyTimeout is the window after the last keypress during which the
// hardware cursor stays visible on the focused browser line, mirroring Python
// LinkableText.key_timeout = 2 (MicronParser.py:876,986).
const cursorKeyTimeout = 2 * time.Second

// browserPageView wraps the page content TextView so it can re-show the
// terminal hardware cursor on every draw. tview's Application.Draw hides the
// cursor at the start of each frame, so the focused primitive must reposition
// it on each Draw — the same pattern ReadlineEdit.Draw uses (readline.go:152).
// This mirrors Python LinkableText.render setting canvas.cursor =
// get_cursor_coords(size) (MicronParser.py:982-992). All TextView behavior
// (scrolling, color tags, region tags, #!bg/#!fg defaults) is unchanged; the
// nav model in BrowserDisplay drives scrolling via ScrollTo and link dispatch
// via HandleLink, so the TextView's own InputHandler never scrolls the page.
type browserPageView struct {
	*tview.TextView
	bd *BrowserDisplay
}

func newBrowserPageView(bd *BrowserDisplay) *browserPageView {
	return &browserPageView{
		TextView: tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetRegions(true),
		bd: bd,
	}
}

// Draw renders the page text and, when the page body is focused and the cursor
// is within its key-timeout visibility window, repositions the terminal
// hardware cursor at the focused line's part cursor.
//
// It also reflows the page when the content is first drawn at a width different
// from the one renderPage laid out for. The fetch callback (OnRetrieveURL) can
// fire before the browser display's first Draw, so contentWidth() returns a
// stale/zero value and horizontal dividers render too short; once the real
// width is known at Draw time, a re-render is queued (Python's urwid.Divider
// fills width at draw time, so this restores parity).
func (v *browserPageView) Draw(screen tcell.Screen) {
	v.TextView.Draw(screen)
	v.bd.drawCursor(screen)
	v.bd.reflowIfWidthChanged()
}

// reflowIfWidthChanged queues a re-render of the current page when the content's
// real inner width differs from the width it was laid out for (renderedWidth).
// No-op when there is no current page, the width already matches, or no app
// loop is available (unit tests drive rendering directly). The re-render runs
// on the UI loop via QueueUpdateDraw so it does not recurse into this Draw.
func (bd *BrowserDisplay) reflowIfWidthChanged() {
	if bd.currentMarkup == "" || bd.app == nil || bd.app.Application == nil {
		return
	}
	_, _, w, _ := bd.content.GetInnerRect()
	if w <= 0 || w == bd.renderedWidth {
		return
	}
	bd.app.QueueUpdateDraw(func() {
		// re-render at the now-known width; renderPage updates renderedWidth so
		// the next Draw does not re-trigger.
		bd.renderPage()
	})
}

// MouseHandler makes a left-click on a rendered link follow it, matching
// Python LinkableText.mouse_event (MicronParser.py:1005-1044): on a left-click
// Python maps the click to a position, finds the item there, and if it is a
// LinkSpec calls handle_link(item.link_target, item.link_fields).
//
// The embedded TextView already resolves a click to one of the numbered region
// tags ["N"]...[""] emitted by StyledLinesToTviewText (SetRegions(true), the
// region index N ↔ bd.links[N]) and highlights it. This override delegates to
// that handler so the region is resolved, then reads the highlighted region's
// ID, maps it to bd.links[N], and dispatches HandleLink with the link's URL —
// the same call the keyboard Enter/Space path makes. The highlight is cleared
// after dispatch so the clicked link is not left visually inverted (urwid
// does not invert a clicked link; tview inverts highlighted regions on draw).
//
// It does not reposition the keyboard part-cursor to the click (Python sets
// _cursor_position); a click typically navigates to a new page whose
// initNavState resets the cursor, and for in-page anchors JumpToAnchor
// scrolls instead. That finer cursor-follow is a deferred refinement.
func (v *browserPageView) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	base := v.TextView.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		consumed, capture = base(action, event, setFocus)
		if v.bd == nil || v.bd.content == nil {
			return
		}
		switch action {
		case tview.MouseLeftDown:
			// Focus the browserPageView itself (not the embedded TextView) so
			// the layout's keyboard-nav input capture sees bd.content.HasFocus()
			// exactly as the keyboard focus path does. Both share the same Box,
			// so HasFocus is identical either way, but keeping the focused
			// primitive the page view keeps input dispatch consistent.
			setFocus(v.bd.content)
		case tview.MouseLeftClick:
			ids := v.bd.content.GetHighlights()
			if len(ids) == 0 {
				return
			}
			// Clear the highlight tview just set so the link is not left
			// inverted on the next draw; urwid does not invert on click.
			v.bd.content.Highlight()
			idx, err := strconv.Atoi(ids[0])
			if err != nil || idx < 0 || idx >= len(v.bd.links) {
				return
			}
			link := v.bd.links[idx] // capture before dispatch (HandleLink may re-render)
			v.bd.HandleLink(link.URL)
		}
		return
	}
}

// initNavState resets the per-line focus + cursor model for the freshly
// rendered page (mirrors Python update_page_display building a fresh Pile of
// LinkableText, whose focus defaults to the first item and whose per-line
// cursor starts at 0). Called from renderPage after bd.currentLines is set.
func (bd *BrowserDisplay) initNavState() {
	n := len(bd.currentLines)
	bd.lineCursors = make([]int, n)
	bd.focusLine = bd.firstSelectableLine()
	bd.cursorHasKeypress = false
	bd.stopCursorHideTimer()
	// A freshly loaded page starts at the top (Python Scrollable trim_top = 0).
	// tview's TextView initializes lineOffset = -1 (an "unset" sentinel), which
	// would make the first scroll math off by one; reset to 0 so the scroll
	// model matches Python from the first keypress.
	bd.content.ScrollToBeginning()
}

// selectableLine reports whether line idx is focusable in the Pile sense: a
// non-empty line (blank lines are urwid.Text("") and non-selectable, so Pile
// up/down skips them, MicronParser.py:122). Every non-empty rendered line is a
// LinkableText in Python (selectable); partial placeholders and tables are
// flattened to ordinary styled lines in the Go renderer, so they are selectable
// here too. (Python's partial Pile is non-selectable, a minor edge divergence.)
func (bd *BrowserDisplay) selectableLine(idx int) bool {
	if idx < 0 || idx >= len(bd.currentLines) {
		return false
	}
	return utf8.RuneCountInString(bd.linePlainText(idx)) > 0
}

func (bd *BrowserDisplay) firstSelectableLine() int {
	for i := 0; i < len(bd.currentLines); i++ {
		if bd.selectableLine(i) {
			return i
		}
	}
	return -1
}

func (bd *BrowserDisplay) prevSelectableLine(idx int) int {
	for i := idx - 1; i >= 0; i-- {
		if bd.selectableLine(i) {
			return i
		}
	}
	return -1
}

func (bd *BrowserDisplay) nextSelectableLine(idx int) int {
	for i := idx + 1; i < len(bd.currentLines); i++ {
		if bd.selectableLine(i) {
			return i
		}
	}
	return -1
}

// linePlainText returns the concatenated visible text of line idx (the span
// texts), the plain-text component of urwid Text.get_text
// (MicronParser.py:921-929).
func (bd *BrowserDisplay) linePlainText(idx int) string {
	if idx < 0 || idx >= len(bd.currentLines) {
		return ""
	}
	sl := bd.currentLines[idx]
	if sl == nil {
		return ""
	}
	n := 0
	for _, s := range sl.Spans {
		n += len(s.Text)
	}
	b := make([]byte, 0, n)
	for _, s := range sl.Spans {
		b = append(b, s.Text...)
	}
	return string(b)
}

// linePartPositions returns the cumulative rune-offset table of the line's
// parts: [0] followed by each span's rune length + running total. Mirrors
// LinkableText.keypress's part_positions build (MicronParser.py:921-929),
// using rune (codepoint) offsets to match Python's str indexing.
func (bd *BrowserDisplay) linePartPositions(idx int) []int {
	pos := []int{0}
	total := 0
	if idx < 0 || idx >= len(bd.currentLines) {
		return pos
	}
	for _, s := range bd.currentLines[idx].Spans {
		total += utf8.RuneCountInString(s.Text)
		pos = append(pos, total)
	}
	return pos
}

// lineLinkAtCursor returns the link spec of the part (span) containing the
// line's cursor offset, or nil if the cursor is on a plain (non-link) part.
// Mirrors find_item_at_pos (MicronParser.py:898-908) + the LinkSpec check.
func (bd *BrowserDisplay) lineLinkAtCursor(idx int) *micron.LinkSpec {
	if idx < 0 || idx >= len(bd.currentLines) || idx >= len(bd.lineCursors) {
		return nil
	}
	cursor := bd.lineCursors[idx]
	total := 0
	for _, s := range bd.currentLines[idx].Spans {
		rlen := utf8.RuneCountInString(s.Text)
		if total <= cursor && cursor < total+rlen {
			return s.Link
		}
		total += rlen
	}
	return nil
}

// cursorVisibleAt reports whether the hardware cursor should be drawn at the
// given instant, mirroring LinkableText.render's focus condition
// (MicronParser.py:986): visible only when focused, and (with a delegate)
// only within key_timeout of the last keypress.
func (bd *BrowserDisplay) cursorVisibleAt(now time.Time, focused bool) bool {
	if !focused {
		return false
	}
	if !bd.cursorHasKeypress {
		return false
	}
	return now.Before(bd.cursorLastKeypress.Add(cursorKeyTimeout))
}

// stampKeypress records that a nav key was pressed, (re)starting the 2s
// cursor-visibility window, and schedules a redraw at its expiry to hide the
// cursor (Python sets delegate.last_keypress + set_alarm_in(key_timeout, kt)
// on every keypress, MicronParser.py:932-935).
func (bd *BrowserDisplay) stampKeypress() {
	bd.cursorLastKeypress = time.Now()
	bd.cursorHasKeypress = true
	bd.scheduleCursorHide()
}

func (bd *BrowserDisplay) stopCursorHideTimer() {
	if bd.cursorHideTimer != nil {
		bd.cursorHideTimer.Stop()
		bd.cursorHideTimer = nil
	}
}

// scheduleCursorHide queues a no-op redraw 2s out so the cursor disappears once
// the visibility window closes. tview's event loop redraws on
// QueueUpdateDraw; the redrawn browserPageView.Draw sees cursorVisibleAt ==
// false and skips ShowCursor (tview already hid it for the frame). No-op
// without a running app (tests drive cursor visibility via cursorVisibleAt).
func (bd *BrowserDisplay) scheduleCursorHide() {
	if bd.app == nil || bd.app.Application == nil {
		return
	}
	bd.stopCursorHideTimer()
	bd.cursorHideTimer = time.AfterFunc(cursorKeyTimeout+50*time.Millisecond, func() {
		bd.app.QueueUpdateDraw(func() {})
	})
}

// peekLink reports the focused line's link to the footer (or clears the peek
// on a plain part), mirroring LinkableText.peek_link (MicronParser.py:910-918).
func (bd *BrowserDisplay) peekLink() {
	link := bd.lineLinkAtCursor(bd.focusLine)
	if link != nil {
		bd.MarkedLink(link.URL, link.Fields)
	} else {
		bd.MarkedLink("", "")
	}
}

// cursorScreenXY returns the screen cell of the focused line's part cursor,
// relative to the content's inner rect origin. x is the display column within
// the wrapped line; y is the wrapped-row offset from the focused line's top.
// Uses tview.WordWrap (the same wrap the TextView uses to draw) so the cursor
// lands on the rendered glyph, then runeWidth for the column.
func (bd *BrowserDisplay) cursorScreenXY() (x, y int, ok bool) {
	if bd.focusLine < 0 || bd.focusLine >= len(bd.currentLines) {
		return 0, 0, false
	}
	if bd.focusLine >= len(bd.lineCursors) {
		return 0, 0, false
	}
	plain := bd.linePlainText(bd.focusLine)
	_, _, width, _ := bd.content.GetInnerRect()
	if width <= 0 {
		width = bd.contentWidth()
	}
	pos := bd.lineCursors[bd.focusLine]
	x, y, _ = wrapCursorXY(plain, width, pos)
	return x, y, true
}

// wrapCursorXY maps a rune offset pos within plain text to a (column, row)
// within its tview.WordWrap layout. Row 0 is the first wrapped line; column is
// the display width from the wrapped line's start to pos. WordWrap returns
// substrings that concatenate back to the input, so accumulating each line's
// rune length tiles the original offsets exactly.
func wrapCursorXY(plain string, width, pos int) (x, y int, ok bool) {
	if width <= 0 {
		return 0, 0, false
	}
	runes := []rune(plain)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	lines := tview.WordWrap(plain, width)
	off := 0
	for yi, ln := range lines {
		rl := utf8.RuneCountInString(ln)
		if pos <= off+rl {
			w := 0
			for i := off; i < pos && i < len(runes); i++ {
				w += runeWidth(runes[i])
			}
			return w, yi, true
		}
		off += rl
	}
	if len(lines) > 0 {
		return 0, len(lines) - 1, true
	}
	return 0, 0, false
}

// cursorAbsRow is the focused cursor's absolute wrapped-row index in the page
// (rowsAbove(focusLine) + its row within the line), used by ensureVisible's
// Scrollable cursor-follow (Scrollable.py:266-274).
func (bd *BrowserDisplay) cursorAbsRow() int {
	if bd.focusLine < 0 {
		return 0
	}
	_, cy, ok := bd.cursorScreenXY()
	if !ok {
		return bd.rowsAbove(bd.focusLine)
	}
	return bd.rowsAbove(bd.focusLine) + cy
}

// ensureVisible scrolls the content so the focused cursor row is within the
// viewport, mirroring Scrollable._adjust_trim_top's cursor-follow
// (Scrollable.py:266-274): if the cursor is above the top, scroll to it; if it
// is at/below the bottom, scroll so it sits on the last row.
func (bd *BrowserDisplay) ensureVisible() {
	if bd.focusLine < 0 {
		return
	}
	_, _, _, h := bd.content.GetInnerRect()
	if h <= 0 {
		return
	}
	cursRow := bd.cursorAbsRow()
	scrollRow, _ := bd.content.GetScrollOffset()
	if cursRow < scrollRow {
		bd.content.ScrollTo(cursRow, 0)
	} else if cursRow >= scrollRow+h {
		bd.content.ScrollTo(cursRow-h+1, 0)
	}
}

// totalWrappedRows is the page's total wrapped-row count (the Scrollable
// canvas height), used to clamp End.
func (bd *BrowserDisplay) totalWrappedRows() int {
	return bd.rowsAbove(len(bd.currentLines))
}

// scrollUpOne / scrollDownOne mirror Scrollable SCROLL_LINE_UP/DOWN
// (Scrollable.py:248-251): trim_top ∓ 1.
func (bd *BrowserDisplay) scrollUpOne() {
	row, _ := bd.content.GetScrollOffset()
	if row > 0 {
		bd.content.ScrollTo(row-1, 0)
	}
}

func (bd *BrowserDisplay) scrollDownOne() {
	row, _ := bd.content.GetScrollOffset()
	bd.content.ScrollTo(row+1, 0)
}

// scrollByPage mirrors Scrollable SCROLL_PAGE_UP/DOWN (Scrollable.py:253-256):
// trim_top ∓ (maxrow-1). dir = -1 up, +1 down.
func (bd *BrowserDisplay) scrollByPage(dir int) {
	_, _, _, h := bd.content.GetInnerRect()
	if h <= 0 {
		return
	}
	row, _ := bd.content.GetScrollOffset()
	bd.content.ScrollTo(row+dir*(h-1), 0)
}

// firstVisibleSelectableAfterScroll returns the first selectable line at or
// below the current scroll top, mirroring Scrollable's automove_cursor_on_scroll
// (Scrollable.py:133-158): after a page/home/end scroll, focus moves to the
// first selectable line that becomes visible. Falls back to the last
// selectable line if all are above the viewport.
func (bd *BrowserDisplay) firstVisibleSelectableAfterScroll() int {
	row, _ := bd.content.GetScrollOffset()
	for i := 0; i < len(bd.currentLines); i++ {
		if bd.selectableLine(i) && bd.rowsAbove(i)+bd.lineRowCount(i) > row {
			return i
		}
	}
	// All selectable lines are above the viewport top: pin to the last.
	last := -1
	for i := 0; i < len(bd.currentLines); i++ {
		if bd.selectableLine(i) {
			last = i
		}
	}
	return last
}

// lineRowCount is the wrapped-row count of one line (its contribution to
// rowsAbove).
func (bd *BrowserDisplay) lineRowCount(idx int) int {
	if idx < 0 || idx >= len(bd.lineTexts) {
		return 1
	}
	_, _, innerW, _ := bd.content.GetInnerRect()
	if innerW <= 0 {
		innerW = bd.contentWidth()
	}
	if innerW <= 0 {
		return 1
	}
	if w := len(tview.WordWrap(bd.lineTexts[idx], innerW)); w > 0 {
		return w
	}
	return 1
}

// automoveFocus is the post-scroll focus reset (Home/End/PgUp/PgDn): move focus
// to the first visible selectable line and reset its cursor, matching
// Scrollable automove_cursor_on_scroll.
func (bd *BrowserDisplay) automoveFocus() {
	idx := bd.firstVisibleSelectableAfterScroll()
	if idx < 0 {
		return
	}
	bd.focusLine = idx
	bd.lineCursors[idx] = 0
	bd.peekLink()
}

// drawCursor repositions the terminal hardware cursor at the focused line's
// part cursor when the page body is focused and the cursor is visible.
func (bd *BrowserDisplay) drawCursor(screen tcell.Screen) {
	if screen == nil {
		return
	}
	if bd.content == nil || !bd.content.HasFocus() {
		return
	}
	if !bd.cursorVisibleAt(time.Now(), true) {
		return
	}
	cx, cy, ok := bd.cursorScreenXY()
	if !ok {
		return
	}
	x0, y0, _, _ := bd.content.GetInnerRect()
	scrollRow, _ := bd.content.GetScrollOffset()
	screenX := x0 + cx
	screenY := y0 + (bd.rowsAbove(bd.focusLine) - scrollRow) + cy
	screen.ShowCursor(screenX, screenY)
}

// handleNavKey dispatches one key through the Python page-key model when the
// page body (bd.content) is focused. Returns true if the key was consumed
// (caller returns nil to stop propagation). Mirrors the dispatch chain
// BrowserFrame → Scrollable → Pile → LinkableText (Browser.py:21,
// Scrollable.py:183, pile.py:978, MicronParser.py:921).
func (bd *BrowserDisplay) handleNavKey(event *tcell.EventKey) bool {
	if bd.focusLine < 0 {
		// No selectable line (empty/disconnected page): let Home/End/PgUp/PgDn
		// still scroll the TextView, but consume the arrows so tview does not
		// horizontal-scroll on Left/Right (Python no-ops them on a blank page).
		switch event.Key() {
		case tcell.KeyLeft, tcell.KeyRight, tcell.KeyUp, tcell.KeyDown,
			tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn:
			return true
		}
		return false
	}

	switch event.Key() {
	case tcell.KeyEnter:
		bd.stampKeypress()
		if link := bd.lineLinkAtCursor(bd.focusLine); link != nil {
			bd.HandleLink(link.URL)
		}
		return true

	case tcell.KeyUp:
		bd.stampKeypress()
		bd.lineCursors[bd.focusLine] = 0
		if p := bd.prevSelectableLine(bd.focusLine); p >= 0 {
			bd.focusLine = p
			bd.ensureVisible()
			bd.peekLink()
		} else {
			bd.scrollUpOne()
		}
		return true

	case tcell.KeyDown:
		bd.stampKeypress()
		bd.lineCursors[bd.focusLine] = 0
		if n := bd.nextSelectableLine(bd.focusLine); n >= 0 {
			bd.focusLine = n
			bd.ensureVisible()
			bd.peekLink()
		} else {
			bd.scrollDownOne()
		}
		return true

	case tcell.KeyRight:
		bd.stampKeypress()
		positions := bd.linePartPositions(bd.focusLine)
		old := bd.lineCursors[bd.focusLine]
		nxt := findNextPartPos(old, positions)
		if nxt == old {
			// At the last part: right wraps to down (in_columns is always false
			// in the Go renderer — tables are flattened, not a Columns layout).
			bd.lineCursors[bd.focusLine] = 0
			if n := bd.nextSelectableLine(bd.focusLine); n >= 0 {
				bd.focusLine = n
				bd.ensureVisible()
				bd.peekLink()
			} else {
				bd.scrollDownOne()
			}
		} else {
			bd.lineCursors[bd.focusLine] = nxt
			bd.ensureVisible()
			bd.peekLink()
		}
		return true

	case tcell.KeyLeft:
		bd.stampKeypress()
		if bd.lineCursors[bd.focusLine] > 0 {
			bd.lineCursors[bd.focusLine] = findPrevPartPos(bd.lineCursors[bd.focusLine], bd.linePartPositions(bd.focusLine))
			bd.ensureVisible()
			bd.peekLink()
		} else {
			// Left at the line's start releases focus to the owning view
			// (Python delegate.micron_released_focus → focus_lists,
			// MicronParser.py:972-974). Go maps it to OnReleaseFocus (the
			// Network left list, or the menu for the standalone browser).
			bd.MicronReleasedFocus()
		}
		return true

	case tcell.KeyHome:
		bd.stampKeypress()
		bd.content.ScrollTo(0, 0)
		bd.automoveFocus()
		return true

	case tcell.KeyEnd:
		bd.stampKeypress()
		_, _, _, h := bd.content.GetInnerRect()
		if h <= 0 {
			h = 1
		}
		endRow := max(0, bd.totalWrappedRows()-h)
		bd.content.ScrollTo(endRow, 0)
		bd.automoveFocus()
		return true

	case tcell.KeyPgUp:
		bd.stampKeypress()
		bd.scrollByPage(-1)
		bd.automoveFocus()
		return true

	case tcell.KeyPgDn:
		bd.stampKeypress()
		bd.scrollByPage(1)
		bd.automoveFocus()
		return true
	}

	// Suppress tview.TextView's vim-style scroll bindings (g/G/j/k/h/l) when the
	// page body is focused — in Python these are unhandled (no-op) in the
	// browser body, so they must not scroll the Go page either.
	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'g', 'G', 'j', 'k', 'h', 'l':
			return true
		}
	}

	// Space activates the link at the cursor (Python ACTIVATE, command_map
	// " " → ACTIVATE, MicronParser.py:937-941).
	if event.Key() == tcell.KeyRune && event.Rune() == ' ' {
		bd.stampKeypress()
		if link := bd.lineLinkAtCursor(bd.focusLine); link != nil {
			bd.HandleLink(link.URL)
		}
		return true
	}

	return false
}
