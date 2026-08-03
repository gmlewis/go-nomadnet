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
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// mouseScreen draws bd.content at a size big enough to show every line of
// navTestPage so each link's region is populated for click resolution.
func mouseScreen(t *testing.T, bd *BrowserDisplay) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	screen.SetSize(80, 40)
	bd.content.SetRect(0, 0, 80, 40)
	bd.content.Draw(screen)
	return screen
}

// clickAt sends a MouseLeftDown + MouseLeftClick at (x, y) on bd.content,
// driving the browserPageView.MouseHandler override the same way tview would.
// The setFocus callback mimics the real app by calling Focus on the primitive
// so HasFocus() reflects the click.
func clickAt(bd *BrowserDisplay, x, y int) {
	setFocus := func(p tview.Primitive) {
		if p != nil {
			p.Focus(func(tview.Primitive) {})
		}
	}
	ev := tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)
	mh := bd.content.MouseHandler()
	mh(tview.MouseLeftDown, ev, setFocus)
	mh(tview.MouseLeftClick, ev, setFocus)
}

// linkScreenY returns the wrapped display row of the first rendered line whose
// plain text contains sub — i.e. the y to click to hit a link on that line.
func linkScreenY(bd *BrowserDisplay, sub string) int {
	return bd.rowsAbove(findLine(bd, sub))
}

// TestBrowserMouseClickFollowsLink pins that a left-click on a rendered link
// dispatches HandleLink → OnRetrieveURL with the link's target, mirroring
// Python LinkableText.mouse_event (MicronParser.py:1005-1044) which on a click
// finds the LinkSpec at the position and calls handle_link.
func TestBrowserMouseClickFollowsLink(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	mouseScreen(t, bd)

	var got string
	bd.OnRetrieveURL = func(url string) { got = url }

	// "Go" is the first link (region "0") at the start of its line.
	clickAt(bd, 0, linkScreenY(bd, "Go trailing"))
	if got != "/page/a.mu" {
		t.Errorf("click on Go link: OnRetrieveURL = %q, want %q", got, "/page/a.mu")
	}

	// "Link2" is the second link (region "1") at the start of its line.
	got = ""
	clickAt(bd, 0, linkScreenY(bd, "Link2"))
	if got != "/page/b.mu" {
		t.Errorf("click on Link2: OnRetrieveURL = %q, want %q", got, "/page/b.mu")
	}
}

// TestBrowserMouseClickOnPlainTextNoDispatch asserts a click on a plain
// (non-link) line does not dispatch a link and leaves no highlight behind
// (tview would otherwise visually invert a leftover highlighted region).
func TestBrowserMouseClickOnPlainTextNoDispatch(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	mouseScreen(t, bd)

	called := false
	bd.OnRetrieveURL = func(url string) { called = true }

	clickAt(bd, 0, linkScreenY(bd, "intro line one"))
	if called {
		t.Error("OnRetrieveURL fired on a plain-text click; want no dispatch")
	}
	if ids := bd.content.GetHighlights(); len(ids) != 0 {
		t.Errorf("plain-text click left highlight %v; want it cleared", ids)
	}
}

// TestBrowserMouseReclickSameLinkDispatches pins that clicking the same link
// twice dispatches both times. tview's TextView with toggleHighlights=false
// (the default) leaves a clicked region highlighted, so a naive
// SetHighlightedFunc approach would miss the second click (added==[]).
// The override clears the highlight after each dispatch, so every click
// re-resolves and re-dispatches.
func TestBrowserMouseReclickSameLinkDispatches(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	mouseScreen(t, bd)

	count := 0
	bd.OnRetrieveURL = func(url string) {
		if url != "/page/a.mu" {
			t.Errorf("OnRetrieveURL = %q, want /page/a.mu", url)
		}
		count++
	}

	y := linkScreenY(bd, "Go trailing")
	clickAt(bd, 0, y)
	clickAt(bd, 0, y)
	if count != 2 {
		t.Errorf("OnRetrieveURL fired %d times after two clicks on the same link, want 2", count)
	}
}

// TestBrowserMouseClickFocussesContent asserts a click focuses the page body
// (bd.content.HasFocus()), so the keyboard-nav model takes over immediately
// after a click — matching Python, where a click sets the LinkableText focus.
func TestBrowserMouseClickFocussesContent(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	mouseScreen(t, bd)

	bd.OnRetrieveURL = func(string) {}

	var focused tview.Primitive
	setFocus := func(p tview.Primitive) {
		focused = p
		if p != nil {
			p.Focus(func(tview.Primitive) {})
		}
	}
	ev := tcell.NewEventMouse(0, linkScreenY(bd, "Go trailing"), tcell.Button1, tcell.ModNone)
	mh := bd.content.MouseHandler()
	mh(tview.MouseLeftDown, ev, setFocus)
	mh(tview.MouseLeftClick, ev, setFocus)

	if focused != (tview.Primitive)(bd.content) {
		t.Errorf("click focused %T, want bd.content (browserPageView)", focused)
	}
	if !bd.content.HasFocus() {
		t.Error("bd.content.HasFocus() false after click; want true")
	}
}