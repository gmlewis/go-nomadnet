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

// TestLogDisplayUpAtTopToMenu asserts Up at the top of the log view moves focus
// to the menu bar, matching Python's LogTerminal.keypress (Log.py:55-58) where
// "up" is the escape sequence that returns focus to the header. The log view is
// a scrollable TextView; Up scrolls up through history and, once at the top,
// collapses focus to the menu (the centralized bodyListAtTop dispatcher only
// covers *tview.List, so the log page owns this transition).
func TestLogDisplayUpAtTopToMenu(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	ld := NewLogDisplay(app, "", 50)

	// Simulate the user having scrolled to the top of the log.
	ld.logView.ScrollToBeginning()
	app.SetFocus(ld.logView)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	got := ld.handleInput(up)
	if got != nil {
		t.Errorf("handleInput(Up) = %v, want nil (consumed)", got)
	}
	if app.Main.focusRegion != "menu" {
		t.Errorf("focusRegion = %q, want menu", app.Main.focusRegion)
	}
}

// TestLogDisplayUpMidScrollForwards asserts Up when the log is scrolled down
// (not at the top) is forwarded to the view for scrolling, NOT stolen to focus
// the menu.
func TestLogDisplayUpMidScrollForwards(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	ld := NewLogDisplay(app, "", 50)

	// Put content in the view and force a non-top scroll offset by scrolling to
	// the end (offset becomes large once laid out; without a screen we simulate
	// "not at top" by setting the view's scroll position directly).
	ld.logView.SetText("line\nline\nline\nline\nline")
	ld.logView.ScrollTo(3, 0)

	app.SetFocus(ld.logView)
	app.Main.focusRegion = "body"

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	got := ld.handleInput(up)
	if got == nil {
		t.Error("handleInput(Up) consumed mid-scroll; want forwarded for scrolling")
	}
	if app.Main.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body (Up should scroll, not steal focus)", app.Main.focusRegion)
	}
}
