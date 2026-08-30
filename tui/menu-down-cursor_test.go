// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestFocusBodyRemovesMenuCursor pins the fix for the DownArrow menu-cursor bug.
// The solid green menu cursor is the terminal hardware cursor, painted by a
// DrawFunc installed on menuBar while focusRegion == "menu". When the user
// presses Down/Tab to drop to the body, FocusBody must re-run redrawMenuBar so
// its else branch clears that DrawFunc (SetDrawFunc(nil)). Before the fix,
// FocusBody only flipped focusRegion and moved tview focus without redrawing,
// so the stale DrawFunc kept painting the cursor onto the menu bar on every
// frame — the cursor appeared to stay in the menu even though focus had moved
// to the body, deviating from Python's MainFrame (focus_position = "body"
// removes the menu highlight).
func TestFocusBodyRemovesMenuCursor(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	// Enter the menu region — this installs the green-cursor DrawFunc.
	md.FocusMenu()
	if df := md.menuBar.GetDrawFunc(); df == nil {
		t.Fatal("FocusMenu did not install the menu cursor DrawFunc (prerequisite)")
	}

	// Drop to the body — the cursor must be removed.
	md.FocusBody()

	if df := md.menuBar.GetDrawFunc(); df != nil {
		t.Error("FocusBody left the menu cursor DrawFunc installed; want nil (green cursor should be removed on Down)")
	}
	if md.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body", md.focusRegion)
	}

	// Re-entering the menu must reinstall the cursor (round-trip).
	md.FocusMenu()
	if df := md.menuBar.GetDrawFunc(); df == nil {
		t.Error("FocusMenu did not reinstall the menu cursor DrawFunc on re-entry")
	}
}

// TestHandleMenuInputDownRemovesMenuCursor pins the full key path: a Down key
// while the menu region is focused routes through handleMenuInput → FocusBody
// and must leave the menu cursor DrawFunc cleared, not just flip focusRegion.
// handleMenuInput consumes the Down key (Python's MenuColumns.keypress sets
// frame.focus_position = "body" and drops the key — urwid Frame.keypress
// dispatches by the entry-time focus part, so the mid-flight change never
// re-dispatches into the body; verified live on nomadnet 1.2.8, where the
// first Down from the menu renders nothing).
func TestHandleMenuInputDownRemovesMenuCursor(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.FocusMenu()
	if md.menuBar.GetDrawFunc() == nil {
		t.Fatal("FocusMenu did not install the menu cursor DrawFunc (prerequisite)")
	}

	got := md.handleMenuInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got != nil {
		t.Errorf("handleMenuInput(Down) = %v, want nil (Python drops the first Down from the menu)", got)
	}
	if df := md.menuBar.GetDrawFunc(); df != nil {
		t.Error("Down in menu left the cursor DrawFunc installed; want nil")
	}
	if md.focusRegion != "body" {
		t.Errorf("focusRegion = %q, want body", md.focusRegion)
	}
}
