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
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// longPage returns a page with n selectable text lines, so a reading position
// well down the page can be recorded and restored.
func longPage(n int) string {
	var b strings.Builder
	b.WriteString(">> Long Page\n\n")
	for i := 0; i < n; i++ {
		b.WriteString("line of text\n")
	}
	return b.String()
}

// focusedBrowser returns a browser display whose content pane is focused and
// sized, which is the state drawCursor and the nav model require.
func focusedBrowser(t *testing.T, w, h int) *BrowserDisplay {
	t.Helper()
	bd := NewBrowserDisplay(newTestApp())
	bd.content.SetRect(0, 0, w, h)
	bd.content.Focus(func(p tview.Primitive) {})
	return bd
}

// TestNewPageDoesNotLeaveStaleHardwareCursor covers the reported bug: after
// following a link, the hardware cursor stayed on the cell the OLD page had put
// it on (the clicked link's row, or a blank row of a short new page). Python
// hides the cursor once LinkableText's 2s key-timeout window has closed
// (MicronParser.py:986 — the canvas gets no cursor, and urwid then leaves the
// terminal cursor hidden), but tview only calls HideCursor from SetFocus, so
// skipping ShowCursor is not enough: drawCursor must hide it explicitly.
func TestNewPageDoesNotLeaveStaleHardwareCursor(t *testing.T) {
	t.Parallel()

	bd := focusedBrowser(t, 80, 24)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)

	// Page one: put the cursor deep in the page and open the visibility window,
	// as an arrow key would.
	bd.RenderPage(longPage(40))
	bd.focusLine = 12
	bd.lineCursors[12] = 2
	bd.stampKeypress()
	bd.drawCursor(screen)
	_, y1, vis1 := screen.GetCursor()
	if !vis1 {
		t.Fatalf("page one: hardware cursor not visible after a keypress (y=%v)", y1)
	}

	// Page two — a NEW page. No keypress has happened on it, so the window is
	// closed and the cursor must not still be sitting at page one's row.
	bd.RenderPage(">> Short Page\n\nonly a little text\n")
	if bd.focusLine != 0 {
		t.Errorf("after a new page: focusLine=%v, want 0 (page top)", bd.focusLine)
	}
	bd.drawCursor(screen)
	_, y2, vis2 := screen.GetCursor()
	if vis2 {
		t.Errorf("after a new page: hardware cursor still visible at y=%v, want hidden "+
			"(it was left at page one's y=%v)", y2, y1)
	}
}

// TestGoBackRestoresReadingPosition covers the second reported bug: Back must
// return to the page exactly where the reader left it — same focused line, same
// part cursor, same scroll row — instead of jumping to the top.
//
// This is a deliberate divergence from Python, whose history holds URLs only
// (Browser.py:131-132) and therefore always re-opens a page at the top.
func TestGoBackRestoresReadingPosition(t *testing.T) {
	t.Parallel()

	const urlA = "aaaa1111aaaa1111aaaa1111aaaa1111"
	const urlB = "bbbb2222bbbb2222bbbb2222bbbb2222"

	bd := focusedBrowser(t, 80, 24)
	bd.OnRetrieveURL = func(url string, _ map[string]string) {
		if url == urlA {
			bd.RenderPage(longPage(40))
			return
		}
		bd.RenderPage(">> Page B\n\nshort\n")
	}

	bd.LoadURL(urlA)
	if bd.CurrentURL() != urlA {
		t.Fatalf("CurrentURL=%q, want %q", bd.CurrentURL(), urlA)
	}

	// Read down page A: focus a line, put the part cursor on it, and scroll.
	bd.focusLine = 12
	bd.lineCursors[12] = 2
	bd.content.ScrollTo(7, 0)
	wantFocus := bd.focusLine
	wantCursor := bd.lineCursors[12]
	wantScroll, _ := bd.content.GetScrollOffset()

	// Follow a link to page B, then come back.
	bd.LoadURL(urlB)
	if bd.focusLine != 0 {
		t.Errorf("new page B: focusLine=%v, want 0 (a fresh page starts at the top)", bd.focusLine)
	}
	bd.GoBack()

	if bd.CurrentURL() != urlA {
		t.Fatalf("after GoBack: CurrentURL=%q, want %q", bd.CurrentURL(), urlA)
	}
	if bd.focusLine != wantFocus {
		t.Errorf("after GoBack: focusLine=%v, want %v (the line the reader left)", bd.focusLine, wantFocus)
	}
	if len(bd.lineCursors) <= 12 || bd.lineCursors[12] != wantCursor {
		t.Errorf("after GoBack: lineCursors[12]=%v, want %v", bd.lineCursors[12], wantCursor)
	}
	if got, _ := bd.content.GetScrollOffset(); got != wantScroll {
		t.Errorf("after GoBack: scroll row=%v, want %v", got, wantScroll)
	}
}

// TestGoBackRestoresPositionRecursively walks A → B → C and back twice,
// asserting each Back lands on its own remembered position rather than the top.
func TestGoBackRestoresPositionRecursively(t *testing.T) {
	t.Parallel()

	const urlA = "aaaa1111aaaa1111aaaa1111aaaa1111"
	const urlB = "bbbb2222bbbb2222bbbb2222bbbb2222"
	const urlC = "cccc3333cccc3333cccc3333cccc3333"

	bd := focusedBrowser(t, 80, 24)
	bd.OnRetrieveURL = func(url string, _ map[string]string) {
		switch url {
		case urlA:
			bd.RenderPage(longPage(40))
		case urlB:
			bd.RenderPage(longPage(30))
		default:
			bd.RenderPage(longPage(20))
		}
	}

	bd.LoadURL(urlA)
	bd.focusLine = 20
	bd.content.ScrollTo(15, 0)
	aFocus := bd.focusLine
	aScroll, _ := bd.content.GetScrollOffset()

	bd.LoadURL(urlB)
	bd.focusLine = 9
	bd.content.ScrollTo(4, 0)
	bFocus := bd.focusLine
	bScroll, _ := bd.content.GetScrollOffset()

	bd.LoadURL(urlC)
	if bd.focusLine != 0 {
		t.Errorf("page C: focusLine=%v, want 0", bd.focusLine)
	}

	bd.GoBack()
	if bd.CurrentURL() != urlB {
		t.Fatalf("after first GoBack: CurrentURL=%q, want %q", bd.CurrentURL(), urlB)
	}
	if bd.focusLine != bFocus {
		t.Errorf("after first GoBack: focusLine=%v, want %v", bd.focusLine, bFocus)
	}
	if got, _ := bd.content.GetScrollOffset(); got != bScroll {
		t.Errorf("after first GoBack: scroll row=%v, want %v", got, bScroll)
	}

	bd.GoBack()
	if bd.CurrentURL() != urlA {
		t.Fatalf("after second GoBack: CurrentURL=%q, want %q", bd.CurrentURL(), urlA)
	}
	if bd.focusLine != aFocus {
		t.Errorf("after second GoBack: focusLine=%v, want %v", bd.focusLine, aFocus)
	}
	if got, _ := bd.content.GetScrollOffset(); got != aScroll {
		t.Errorf("after second GoBack: scroll row=%v, want %v", got, aScroll)
	}
}

// TestGoForwardRestoresReadingPosition checks the forward direction restores
// the position recorded when the reader left that page via Back.
func TestGoForwardRestoresReadingPosition(t *testing.T) {
	t.Parallel()

	const urlA = "aaaa1111aaaa1111aaaa1111aaaa1111"
	const urlB = "bbbb2222bbbb2222bbbb2222bbbb2222"

	bd := focusedBrowser(t, 80, 24)
	bd.OnRetrieveURL = func(url string, _ map[string]string) {
		if url == urlA {
			bd.RenderPage(longPage(40))
			return
		}
		bd.RenderPage(longPage(30))
	}

	bd.LoadURL(urlA)
	bd.LoadURL(urlB)
	bd.focusLine = 11
	bd.content.ScrollTo(6, 0)
	bFocus := bd.focusLine
	bScroll, _ := bd.content.GetScrollOffset()

	bd.GoBack()
	if bd.focusLine != 0 && bd.focusLine == bFocus {
		t.Fatalf("after GoBack: focusLine=%v unexpectedly equals page B's %v", bd.focusLine, bFocus)
	}
	bd.GoForward()
	if bd.CurrentURL() != urlB {
		t.Fatalf("after GoForward: CurrentURL=%q, want %q", bd.CurrentURL(), urlB)
	}
	if bd.focusLine != bFocus {
		t.Errorf("after GoForward: focusLine=%v, want %v", bd.focusLine, bFocus)
	}
	if got, _ := bd.content.GetScrollOffset(); got != bScroll {
		t.Errorf("after GoForward: scroll row=%v, want %v", got, bScroll)
	}
}

// TestReloadStartsAtTop guards the other half of the contract: a reload is a
// fresh page load, so it must NOT inherit a Back/Forward restore.
func TestReloadStartsAtTop(t *testing.T) {
	t.Parallel()

	const urlA = "aaaa1111aaaa1111aaaa1111aaaa1111"
	const urlB = "bbbb2222bbbb2222bbbb2222bbbb2222"

	bd := focusedBrowser(t, 80, 24)
	bd.OnRetrieveURL = func(url string, _ map[string]string) {
		bd.RenderPage(longPage(40))
	}

	bd.LoadURL(urlA)
	bd.focusLine = 18
	bd.content.ScrollTo(12, 0)
	bd.LoadURL(urlB)
	bd.GoBack()
	if bd.focusLine != 18 {
		t.Fatalf("after GoBack: focusLine=%v, want 18", bd.focusLine)
	}

	bd.Reload()
	if bd.focusLine != 0 {
		t.Errorf("after Reload: focusLine=%v, want 0 (a reload starts at the top)", bd.focusLine)
	}
	if got, _ := bd.content.GetScrollOffset(); got != 0 {
		t.Errorf("after Reload: scroll row=%v, want 0", got)
	}
}

// TestRestoreClampsToShorterPage guards the re-fetch case: Back re-fetches the
// page, so a page that shrank since the position was recorded must not panic or
// leave the focus on a non-selectable line.
func TestRestoreClampsToShorterPage(t *testing.T) {
	t.Parallel()

	const urlA = "aaaa1111aaaa1111aaaa1111aaaa1111"
	const urlB = "bbbb2222bbbb2222bbbb2222bbbb2222"

	bd := focusedBrowser(t, 80, 24)
	shrink := false
	bd.OnRetrieveURL = func(url string, _ map[string]string) {
		if url == urlA && shrink {
			bd.RenderPage(">> Now Short\n\none line\n")
			return
		}
		bd.RenderPage(longPage(40))
	}

	bd.LoadURL(urlA)
	bd.focusLine = 30
	bd.lineCursors[30] = 4
	bd.content.ScrollTo(25, 0)

	bd.LoadURL(urlB)
	shrink = true
	bd.GoBack()

	if bd.focusLine < 0 || bd.focusLine >= len(bd.currentLines) {
		t.Fatalf("focusLine=%v out of range for %v lines", bd.focusLine, len(bd.currentLines))
	}
	if !bd.selectableLine(bd.focusLine) {
		t.Errorf("focusLine=%v is not a selectable line after clamping", bd.focusLine)
	}
}
