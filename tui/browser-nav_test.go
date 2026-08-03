// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// navTestPage is a micron page exercising every branch of the page-key model:
//
//	>Heading            (line 0: heading, selectable)
//	intro line one      (line 1: plain, single part)
//	intro line two      (line 2: plain, single part)
//	`[Go`/page/a.mu] trailing  (line 3: link "Go" + plain " trailing"; parts [0,2,11])
//	`[Link2`/page/b.mu]  (line 4: link only; parts [0,5])
//	final line          (line 5: plain, single part)
//
// Line indices are resolved by substring so the test does not hard-depend on
// micron emitting blank separator lines.
const navTestPage = ">Heading\nintro line one\nintro line two\n`[Go`/page/a.mu] trailing\n`[Link2`/page/b.mu]\nfinal line"

// newNavTestBrowser renders navTestPage and focuses the content body, returning
// the display ready for handleInput driving.
func newNavTestBrowser(t *testing.T) (*App, *BrowserDisplay) {
	t.Helper()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	// Give the content a real rect so scroll math (viewport height, wrap width)
	// is non-degenerate. 80×4 viewport: 6 one-row lines, so End scrolls to row 2.
	bd.content.SetRect(0, 0, 80, 4)
	app.SetFocus(bd.content)
	return app, bd
}

// findLine returns the first rendered line index whose plain text contains sub.
func findLine(bd *BrowserDisplay, sub string) int {
	for i := 0; i < len(bd.currentLines); i++ {
		if strings.Contains(bd.linePlainText(i), sub) {
			return i
		}
	}
	return -1
}

func key(k tcell.Key, r rune) *tcell.EventKey {
	return tcell.NewEventKey(k, r, tcell.ModNone)
}

// wantFocus asserts the focused line's plain text contains sub.
func wantFocus(t *testing.T, bd *BrowserDisplay, sub string) {
	t.Helper()
	got := bd.linePlainText(bd.focusLine)
	if !strings.Contains(got, sub) {
		t.Errorf("focusLine=%d text=%q, want a line containing %q", bd.focusLine, got, sub)
	}
}

// TestNavDownUpMovesFocus covers Python Pile.keypress: Down moves focus to the
// next selectable line and resets its cursor; Up moves back. At the last line
// Down scrolls instead of wrapping (Scrollable SCROLL_LINE_DOWN); above the
// first line Up scrolls. MicronParser.py:942-952, Scrollable.py:248-251.
func TestNavDownUpMovesFocus(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	wantFocus(t, bd, "Heading") // firstSelectable = heading line

	if got := bd.handleInput(key(tcell.KeyDown, 0)); got != nil {
		t.Errorf("Down not consumed: %v", got)
	}
	wantFocus(t, bd, "intro line one")

	bd.handleInput(key(tcell.KeyDown, 0))
	wantFocus(t, bd, "intro line two")

	bd.handleInput(key(tcell.KeyDown, 0))
	wantFocus(t, bd, "Go")
	if bd.lineCursors[bd.focusLine] != 0 {
		t.Errorf("cursor after Down = %d, want 0 (Down resets cursor)", bd.lineCursors[bd.focusLine])
	}

	bd.handleInput(key(tcell.KeyUp, 0))
	wantFocus(t, bd, "intro line two")
}

// TestNavRightStepsPartsThenWraps covers LinkableText.keypress Right
// (MicronParser.py:953-961): right advances to the next part; at the last part
// it wraps to Down (in_columns is false for the flattened Go renderer).
func TestNavRightStepsPartsThenWraps(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	linkLine := findLine(bd, "Go")
	// Walk down to the link line.
	for bd.focusLine != linkLine {
		bd.handleInput(key(tcell.KeyDown, 0))
	}
	if bd.lineCursors[linkLine] != 0 {
		t.Fatalf("cursor start = %d, want 0", bd.lineCursors[linkLine])
	}
	// Cursor at 0 is on the "Go" link part (parts are [0,2,11]; offset 0..1 = link).
	if link := bd.lineLinkAtCursor(linkLine); link == nil || link.URL != "/page/a.mu" {
		t.Errorf("cursor at 0: link=%v, want /page/a.mu", link)
	}

	// Right 0 -> 2 (the boundary = start of the " trailing" plain part).
	bd.handleInput(key(tcell.KeyRight, 0))
	if bd.lineCursors[linkLine] != 2 {
		t.Errorf("after 1st Right cursor=%d, want 2", bd.lineCursors[linkLine])
	}
	if link := bd.lineLinkAtCursor(linkLine); link != nil {
		t.Errorf("cursor at part 2: got link %v, want nil (plain part)", link.URL)
	}

	// Right 2 -> 11 (the trailing plain part, last part).
	bd.handleInput(key(tcell.KeyRight, 0))
	if bd.lineCursors[linkLine] != 11 {
		t.Errorf("after 2nd Right cursor=%d, want 11", bd.lineCursors[linkLine])
	}
	if link := bd.lineLinkAtCursor(linkLine); link != nil {
		t.Errorf("cursor at plain part: got link %v, want nil", link.URL)
	}

	// Right at the last part wraps to Down: focus moves to the next line, cursor 0.
	bd.handleInput(key(tcell.KeyRight, 0))
	wantFocus(t, bd, "Link2")
	if bd.lineCursors[bd.focusLine] != 0 {
		t.Errorf("after wrap-to-Down cursor=%d, want 0", bd.lineCursors[bd.focusLine])
	}
}

// TestNavLeftStepsBackThenReleases covers LinkableText.keypress Left
// (MicronParser.py:962-974): Left steps to the previous part when the cursor is
// past the start; Left at the start calls micron_released_focus → OnReleaseFocus.
func TestNavLeftStepsBackThenReleases(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	linkLine := findLine(bd, "Go")
	for bd.focusLine != linkLine {
		bd.handleInput(key(tcell.KeyDown, 0))
	}
	// Step Right to the plain part (cursor 11), then Left back to 2, then 0.
	bd.handleInput(key(tcell.KeyRight, 0)) // -> 2
	bd.handleInput(key(tcell.KeyRight, 0)) // -> 11
	bd.handleInput(key(tcell.KeyLeft, 0))
	if bd.lineCursors[linkLine] != 2 {
		t.Errorf("after Left cursor=%d, want 2", bd.lineCursors[linkLine])
	}
	bd.handleInput(key(tcell.KeyLeft, 0))
	if bd.lineCursors[linkLine] != 0 {
		t.Errorf("after Left cursor=%d, want 0", bd.lineCursors[linkLine])
	}

	// Left at the start releases focus to the owning view.
	released := false
	bd.OnReleaseFocus = func() { released = true }
	bd.handleInput(key(tcell.KeyLeft, 0))
	if !released {
		t.Error("Left at start did not fire OnReleaseFocus (micron_released_focus)")
	}
}

// TestNavEnterSpaceFollowLink covers ACTIVATE (Enter/Space,
// MicronParser.py:937-941): when the cursor is on a link part, HandleLink is
// invoked with the link's URL; on a plain part it is a no-op.
func TestNavEnterSpaceFollowLink(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	var retrieved string
	bd.OnRetrieveURL = func(url string) { retrieved = url }

	linkLine := findLine(bd, "Go")
	for bd.focusLine != linkLine {
		bd.handleInput(key(tcell.KeyDown, 0))
	}
	// Cursor starts at 0 = on the "Go" link part.
	bd.handleInput(key(tcell.KeyEnter, 0))
	if retrieved != "/page/a.mu" {
		t.Errorf("Enter on link: retrieved=%q, want /page/a.mu", retrieved)
	}

	retrieved = ""
	bd.handleInput(key(tcell.KeyRune, ' '))
	if retrieved != "/page/a.mu" {
		t.Errorf("Space on link: retrieved=%q, want /page/a.mu", retrieved)
	}

	// On a plain (non-link) line, Enter does nothing.
	retrieved = ""
	bd.handleInput(key(tcell.KeyUp, 0)) // back to a plain line
	for bd.lineLinkAtCursor(bd.focusLine) != nil {
		bd.handleInput(key(tcell.KeyUp, 0))
	}
	bd.handleInput(key(tcell.KeyEnter, 0))
	if retrieved != "" {
		t.Errorf("Enter on plain line: retrieved=%q, want empty", retrieved)
	}
}

// TestNavHomeEndScroll covers Home/End (Scrollable SCROLL_TO_TOP/TO_END) +
// automove_cursor_on_scroll: focus jumps to the first visible selectable line.
// Scrollable.py:248-256, 133-158.
func TestNavHomeEndScroll(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	// End: scroll to (total - height) = 6 - 4 = 2, automove to first visible
	// selectable line at/below row 2 = "intro line two" (rowsAbove 2, at the
	// viewport top).
	bd.handleInput(key(tcell.KeyEnd, 0))
	row, _ := bd.content.GetScrollOffset()
	if row != 2 {
		t.Errorf("End scroll row=%d, want 2", row)
	}
	wantFocus(t, bd, "intro line two")

	// Home: scroll to 0, automove to the first selectable line.
	bd.handleInput(key(tcell.KeyHome, 0))
	row, _ = bd.content.GetScrollOffset()
	if row != 0 {
		t.Errorf("Home scroll row=%d, want 0", row)
	}
	wantFocus(t, bd, "Heading")
}

// TestNavPageUpDown covers PgUp/PgDn (Scrollable SCROLL_PAGE_UP/DOWN: ±(h-1)).
func TestNavPageUpDown(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	// PgDn from row 0 → row + (h-1) = 3.
	bd.handleInput(key(tcell.KeyPgDn, 0))
	row, _ := bd.content.GetScrollOffset()
	if row != 3 {
		t.Errorf("PgDn scroll row=%d, want 3", row)
	}

	// PgUp from row 3 → 3 - 3 = 0.
	bd.handleInput(key(tcell.KeyPgUp, 0))
	row, _ = bd.content.GetScrollOffset()
	if row != 0 {
		t.Errorf("PgUp scroll row=%d, want 0", row)
	}
}

// TestNavVimLettersSuppressed asserts the tview TextView's vim-style scroll
// bindings (g/G/j/k/h/l) are consumed as no-ops when the page body is focused —
// in Python they are unhandled in the browser body and must not scroll the Go
// page either.
func TestNavVimLettersSuppressed(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	startRow, _ := bd.content.GetScrollOffset()
	startFocus := bd.focusLine
	for _, r := range []rune{'g', 'G', 'j', 'k', 'h', 'l'} {
		if got := bd.handleInput(key(tcell.KeyRune, r)); got != nil {
			t.Errorf("vim %q not consumed: %v", r, got)
		}
	}
	row, _ := bd.content.GetScrollOffset()
	if row != startRow {
		t.Errorf("vim letters scrolled page: row %d -> %d", startRow, row)
	}
	if bd.focusLine != startFocus {
		t.Errorf("vim letters moved focus: %d -> %d", startFocus, bd.focusLine)
	}
}

// TestNavPassesThroughOnURLBar asserts navigation keys are NOT consumed when a
// non-content child (the URL bar) holds focus — they pass through so the input
// field keeps editing. (Python's BrowserFrame only runs the page model when the
// page body is focused.)
func TestNavPassesThroughOnURLBar(t *testing.T) {
	t.Parallel()
	app, bd := newNavTestBrowser(t)
	app.SetFocus(bd.urlBar)

	for _, ev := range []*tcell.EventKey{
		key(tcell.KeyUp, 0), key(tcell.KeyDown, 0), key(tcell.KeyLeft, 0),
		key(tcell.KeyRight, 0), key(tcell.KeyHome, 0), key(tcell.KeyEnd, 0),
		key(tcell.KeyPgUp, 0), key(tcell.KeyPgDn, 0),
	} {
		if got := bd.handleInput(ev); got == nil {
			t.Errorf("key consumed while URL bar focused; want pass-through")
		}
	}
}

// TestNavCursorPlacement asserts the hardware cursor (re-shown by
// browserPageView.Draw) lands on the focused line's part cursor cell, matching
// Python LinkableText.render's canvas.cursor = get_cursor_coords(size)
// (MicronParser.py:982-992).
func TestNavCursorPlacement(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	// Focus the "intro line one" line (rowsAbove = 1, cursor 0).
	intro := findLine(bd, "intro line one")
	for bd.focusLine != intro {
		bd.handleInput(key(tcell.KeyDown, 0))
	}

	// Make the cursor visible (within the 2s key-timeout window) and draw.
	bd.stampKeypress()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	screen.SetSize(80, 4)
	bd.content.SetRect(0, 0, 80, 4)
	bd.content.Draw(screen)

	cx, cy, visible := screen.GetCursor()
	if !visible {
		t.Fatal("cursor not visible after Draw on focused content")
	}
	// rowsAbove(intro)=1, cursor at column 0, scroll 0 → screen (0, 1).
	if cx != 0 || cy != 1 {
		t.Errorf("cursor at (%d,%d), want (0,1)", cx, cy)
	}
}

// TestNavCursorHiddenBeforeKeypress asserts the hardware cursor stays hidden
// until the first nav keypress, matching LinkableText's key_timeout gate
// (MicronParser.py:986).
func TestNavCursorHiddenBeforeKeypress(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	screen.SetSize(80, 4)
	bd.content.SetRect(0, 0, 80, 4)
	bd.content.Draw(screen)

	if _, _, visible := screen.GetCursor(); visible {
		t.Error("cursor visible before any keypress; want hidden (key_timeout gate)")
	}
}

// TestNavReleaseFocusWiredToMenu confirms the standalone browser page's
// OnReleaseFocus hands focus to the menu bar (the Go port's equivalent of
// Python focus_lists for a standalone browser frame). This is wired in
// cmd/gonomadnet; here we assert the delegate plumbing (OnReleaseFocus fires
// MicronReleasedFocus → callback).
func TestNavReleaseFocusDelegate(t *testing.T) {
	t.Parallel()
	_, bd := newNavTestBrowser(t)

	called := false
	bd.OnReleaseFocus = func() { called = true }

	// Left at the start of the first selectable line releases focus.
	for bd.focusLine != bd.firstSelectableLine() {
		bd.handleInput(key(tcell.KeyUp, 0))
	}
	bd.handleInput(key(tcell.KeyLeft, 0))
	if !called {
		t.Error("Left at first line start did not invoke OnReleaseFocus")
	}
}
