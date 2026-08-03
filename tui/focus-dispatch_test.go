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

// TestFocusDispatch asserts MainDisplay.handleInput matches the Python focus
// model (Main.py MenuColumns:171-176, MainFrame:80-86; TextUI.py:262-264
// unhandled_input). Rules under test:
//
//   - Ctrl-Q is the ONLY global quit (Esc/q/digits are NOT quits).
//   - In the menu region: Left/Right move focus between buttons WITHOUT
//     switching the body page; Enter/Space activate (switch page, focus STAYS
//     in the menu — Python's show_* does not move focus_position);
//     Tab/Down drop to body without switching.
//   - In the body region: Left/Right/Up/Tab/Esc are forwarded to the page
//     (returned unconsumed) so the page can do pane focus / Esc-to-dialog /
//     Up-at-top→menu. The body page is unchanged by the main dispatcher.
func TestFocusDispatch(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	quitCalled := false
	md.SetQuitCallback(func() { quitCalled = true })

	// reset puts md into a known (region, menu index, page) state for one case.
	reset := func(region string, menu int, page string) {
		quitCalled = false
		md.focusRegion = region
		md.activeMenu = menu
		md.activePage = page
		md.contentArea.SwitchToPage(page)
		if region == "menu" {
			app.SetFocus(md.menuBar)
		} else {
			app.SetFocus(md.contentArea)
		}
	}

	type dispatchCase struct {
		name        string
		region      string // starting focus region
		menu        int    // starting activeMenu
		page        string // starting activePage
		key         *tcell.EventKey
		wantRegion  string
		wantMenu    int // expected activeMenu after; -1 = skip check
		wantPage    string
		wantConsume bool // true => handleInput returns nil
		wantQuit    bool
	}

	// lastIndex for wrap checks (8 items => indices 0..7).
	const last = 7

	cases := []dispatchCase{
		// ---- Body region: everything forwards, nothing quits or switches ----
		{"body/ctrl-q quits", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone),
			"body", 0, "conversations", true, true},
		{"body/left forwards", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone),
			"body", 0, "conversations", false, false},
		{"body/right forwards", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone),
			"body", 0, "conversations", false, false},
		{"body/up forwards (page handles top→menu)", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone),
			"body", 0, "conversations", false, false},
		{"body/tab forwards", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone),
			"body", 0, "conversations", false, false},
		{"body/esc forwards (NOT quit)", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
			"body", 0, "conversations", false, false},
		{"body/q is NOT a quit", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone),
			"body", 0, "conversations", false, false},
		{"body/no digit shortcuts", "body", 0, "conversations",
			tcell.NewEventKey(tcell.KeyRune, '3', tcell.ModNone),
			"body", 0, "conversations", false, false},

		// ---- Menu region ----
		{"menu/left wraps focus, no page switch", "menu", 0, "conversations",
			tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone),
			"menu", last, "conversations", true, false},
		{"menu/right moves focus, no page switch", "menu", 0, "conversations",
			tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone),
			"menu", 1, "conversations", true, false},
		{"menu/right at end wraps", "menu", last, "conversations",
			tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone),
			"menu", 0, "conversations", true, false},
		{"menu/enter activates page, keeps menu focus", "menu", 1, "conversations",
			tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
			"menu", 1, "network", true, false},
		{"menu/space activates page, keeps menu focus", "menu", 2, "conversations",
			tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone),
			"menu", 2, "channels", true, false},
		// The Quit menu item (index 7, key "quit") triggers graceful shutdown
		// (Python Main.py:158 handler.quit raises urwid.ExitMainLoop → NomadNetworkApp
		// atexit exit_handler saves directory + tears down RRC). It must NOT switch
		// the body to the "quit" page — Python shows no page on quit.
		{"menu/enter on quit triggers shutdown", "menu", last, "conversations",
			tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
			"menu", last, "conversations", true, true},
		{"menu/space on quit triggers shutdown", "menu", last, "conversations",
			tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone),
			"menu", last, "conversations", true, true},
		{"menu/tab drops to body, no switch", "menu", 1, "conversations",
			tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone),
			"body", 1, "conversations", true, false},
		{"menu/down drops to body, no switch", "menu", 3, "conversations",
			tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone),
			"body", 3, "conversations", true, false},
		{"menu/ctrl-q quits", "menu", 0, "conversations",
			tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone),
			"menu", 0, "conversations", true, true},
		{"menu/esc forwards (NOT quit)", "menu", 0, "conversations",
			tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
			"menu", 0, "conversations", false, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// Sub-tests share md (reset mutates it); run sequentially.
			reset(c.region, c.menu, c.page)

			got := md.handleInput(c.key)

			if (got == nil) != c.wantConsume {
				t.Errorf("consume = %v, want %v (returned %v)", got == nil, c.wantConsume, got)
			}
			if quitCalled != c.wantQuit {
				t.Errorf("quitCalled = %v, want %v", quitCalled, c.wantQuit)
			}
			if md.focusRegion != c.wantRegion {
				t.Errorf("focusRegion = %q, want %q", md.focusRegion, c.wantRegion)
			}
			if c.wantMenu >= 0 && md.activeMenu != c.wantMenu {
				t.Errorf("activeMenu = %v, want %v", md.activeMenu, c.wantMenu)
			}
			if md.activePage != c.wantPage {
				t.Errorf("activePage = %q, want %q", md.activePage, c.wantPage)
			}
		})
	}
}

// TestFocusMenuBody asserts the region transitions and app-focus target.
func TestFocusMenuBody(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.FocusBody()
	if md.focusRegion != "body" {
		t.Errorf("after FocusBody, region = %q, want body", md.focusRegion)
	}

	md.FocusMenu()
	if md.focusRegion != "menu" {
		t.Errorf("after FocusMenu, region = %q, want menu", md.focusRegion)
	}
}
