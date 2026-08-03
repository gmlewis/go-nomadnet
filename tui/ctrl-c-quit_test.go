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

// TestCtrlCQuitsLikeCtrlQ pins the parity fix that routes Ctrl-C through the
// same onQuit path as Ctrl-Q. In Python, Ctrl-C raises KeyboardInterrupt which
// the urwid loop catches to exit cleanly, and the atexit handler then saves the
// directory (NomadNetworkApp.py:38-42). tcell runs the terminal in raw mode so
// Ctrl-C arrives as a key event (KeyCtrlC) rather than SIGINT; without this
// routing it would be a no-op and the user would lose all discovered nodes on
// Ctrl-C, since the shutdown path is the only one that persists the directory.
// Both keys must invoke onQuit (which performs App.Shutdown → SaveToDisk) and
// consume the event.
func TestCtrlCQuitsLikeCtrlQ(t *testing.T) {
	t.Parallel()

	for _, key := range []tcell.Key{tcell.KeyCtrlQ, tcell.KeyCtrlC} {
		app := newTestApp()
		md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

		quitCalled := false
		md.SetQuitCallback(func() { quitCalled = true })

		// Dispatch from the body region (the default after NewMainDisplay) so we
		// exercise the global-quit branch, not the menu-region handler.
		got := md.handleInput(tcell.NewEventKey(key, 0, tcell.ModNone))
		if got != nil {
			t.Errorf("handleInput(%v) = %v, want nil (consumed)", key, got)
		}
		if !quitCalled {
			t.Errorf("handleInput(%v) did not invoke onQuit; the directory would not be saved on this key", key)
		}
	}
}

// TestCtrlCConsumedInMenuToo ensures Ctrl-C quits regardless of focus region
// (the global-quit branch precedes the menu/body split), so a user who presses
// Ctrl-C while navigating the menu still saves + quits rather than the key
// being swallowed by the menu handler.
func TestCtrlCConsumedInMenuToo(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	md.FocusMenu()

	quitCalled := false
	md.SetQuitCallback(func() { quitCalled = true })

	if got := md.handleInput(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone)); got != nil {
		t.Errorf("handleInput(CtrlC) in menu = %v, want nil (consumed)", got)
	}
	if !quitCalled {
		t.Error("Ctrl-C in menu did not invoke onQuit")
	}
}
