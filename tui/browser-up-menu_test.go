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
)

// TestBrowserDisplayUpAtTopToMenu asserts Up at the top of the browser page
// content moves focus to the menu bar, matching the Go port's unified body
// focus model (Python's BrowserFrame.keypress at Browser.py:41-67 only scrolls
// the page; the Go port collapses focus to the menu on Up-at-top for every body
// page, like the log display). The centralized MainDisplay.bodyListAtTop only
// covers *tview.List, so the browser's scrollable TextView content owns this
// transition. Up on the URL bar is NOT stolen (the input field keeps focus).
func TestBrowserDisplayUpAtTopToMenu(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	bd := NewBrowserDisplay(app)

	// Page content focused and scrolled to the top.
	bd.content.ScrollToBeginning()
	app.SetFocus(bd.content)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	got := bd.handleInput(up)
	if got != nil {
		t.Errorf("handleInput(Up) = %v, want nil (consumed)", got)
	}
	if app.Main.focusRegion != "menu" {
		t.Errorf("focusRegion = %q, want menu", app.Main.focusRegion)
	}
}

// TestBrowserDisplayUpMidScrollForwards asserts Up when the page content is
// scrolled down (not at the top) is forwarded to the view for scrolling, NOT
// stolen to focus the menu.
func TestBrowserDisplayUpMidScrollForwards(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	bd := NewBrowserDisplay(app)

	bd.content.SetText("line\nline\nline\nline\nline")
	bd.content.ScrollTo(3, 0)
	app.SetFocus(bd.content)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	got := bd.handleInput(up)
	if got == nil {
		t.Error("handleInput(Up) consumed mid-scroll; want forwarded for scrolling")
	}
	if app.Main.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body (Up should scroll, not steal focus)", app.Main.focusRegion)
	}
}

// TestBrowserDisplayUpOnURLBarForwards asserts Up while the URL input field is
// focused is forwarded (the cursor stays in the URL bar), NOT stolen to focus
// the menu — the URL bar is an InputField, not the page content.
func TestBrowserDisplayUpOnURLBarForwards(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	bd := NewBrowserDisplay(app)

	app.SetFocus(bd.urlBar)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	got := bd.handleInput(up)
	if got == nil {
		t.Error("handleInput(Up) consumed while URL bar focused; want forwarded")
	}
	if app.Main.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body (URL bar keeps focus)", app.Main.focusRegion)
	}
}
