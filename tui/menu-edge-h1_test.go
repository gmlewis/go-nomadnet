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

// TestH1MenuEdgesDoNotWrap pins H1: Python's MenuColumns (urwid Columns 4.0.3,
// container.py keypress) does NOT wrap menu focus at the edges — Left at the
// leftmost button keeps the highlight there and the unhandled key dies at the
// MainFrame; Right at the rightmost button likewise stays (verified with a
// urwid 4.0.3 probe: focus 0 + Left → focus 0; last + Right → stays). The
// earlier Go build wrapped Left from Conversations onto QUIT — one accidental
// Enter away from exiting the app.
func TestH1MenuEdgesDoNotWrap(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	n := len(md.menuItems)

	// Left at the leftmost button: the highlight stays on Conversations (0),
	// never wrapping to Quit (n-1).
	md.activeMenu = 0
	md.handleMenuInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if md.activeMenu != 0 {
		t.Errorf("Left at the leftmost menu button moved to %v; want 0 (no wrap)", md.activeMenu)
	}

	// Right at the rightmost button: the highlight stays.
	md.activeMenu = n - 1
	md.handleMenuInput(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if md.activeMenu != n-1 {
		t.Errorf("Right at the rightmost menu button moved to %v; want %v (no wrap)", md.activeMenu, n-1)
	}

	// Interior movement still works.
	md.activeMenu = 2
	md.handleMenuInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if md.activeMenu != 1 {
		t.Errorf("Left from index 2 = %v, want 1", md.activeMenu)
	}
	md.handleMenuInput(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if md.activeMenu != 2 {
		t.Errorf("Right from index 1 = %v, want 2", md.activeMenu)
	}
}
