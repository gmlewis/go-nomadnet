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

// TestMenuEnterKeepsFocusOnMenuBar verifies that pressing Enter on a menu
// item activates the page but does NOT move focus to the body — matching
// Python's urwid MenuButton on_press (Main.py:99-137 show_* +
// update_active_sub_display), which swaps the body content but never
// touches MainFrame.focus_position. Focus stays on the menu bar (header)
// until the user presses Tab/Down (Main.py MenuColumns:172-176).
//
// In the Go port, tview's Pages.SwitchToPage calls p.Focus(p.setFocus)
// when p.HasFocus() is true. After switching pages (which can leave stale
// hasFocus on hidden page containers) and then FocusMenu, the contentArea
// may still report HasFocus()=true, causing SwitchToPage to steal focus
// to the new page's first child. The fix must restore focus to the menu
// bar after selectMenu when the menu was focused.
func TestMenuEnterKeepsFocusOnMenuBar(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "aaaa", DisplayName: "Node A"},
		{SourceHash: "bbbb", DisplayName: "Node B"},
	})
	app.Main.SetDisplay("network", nd.Widget())
	app.Main.SetDisplay("guide", NewGuideDisplay(app).Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)

	// Start on Guide, enter the body, then switch to Network (creating
	// stale hasFocus on the hidden Guide page's containers), then go to
	// the menu and press Enter on Network.
	app.Main.SelectPage("guide")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	// Switch to Network while body is focused — SwitchToPage's
	// p.Focus(p.setFocus) cascade may leave stale hasFocus on the
	// now-hidden Guide page's containers.
	app.Main.SelectPage("network")
	app.Main.Root().Draw(screen)

	// Go to the menu bar.
	app.Main.FocusMenu()
	app.Main.Root().Draw(screen)

	if app.GetFocus() != app.Main.menuBar {
		t.Fatalf("precondition: focus=%T, want menuBar", app.GetFocus())
	}
	if app.Main.focusRegion != "menu" {
		t.Fatalf("precondition: focusRegion=%q, want menu", app.Main.focusRegion)
	}

	// Check if contentArea has stale focus.
	t.Logf("contentArea.HasFocus()=%v before Enter", app.Main.contentArea.HasFocus())

	// Find the "network" menu item index.
	networkIdx := -1
	for i, item := range app.Main.menuItems {
		if item.Key == "network" {
			networkIdx = i
			break
		}
	}
	if networkIdx < 0 {
		t.Fatal("no 'network' menu item found")
	}
	app.Main.activeMenu = networkIdx
	app.Main.redrawMenuBar()
	app.Main.Root().Draw(screen)

	// Press Enter on the "Network" menu item.
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	result := app.Main.handleInput(enter)
	if result != nil {
		t.Errorf("handleInput(Enter)=%v, want nil (consumed)", result)
	}

	// After Enter, focus MUST still be on the menu bar — Python's
	// MenuButton on_press never touches MainFrame.focus_position.
	if app.GetFocus() != app.Main.menuBar {
		t.Errorf("after Enter: focus=%T, want menuBar (Python keeps focus on the menu bar after Enter)", app.GetFocus())
	}
	if app.Main.focusRegion != "menu" {
		t.Errorf("after Enter: focusRegion=%q, want menu", app.Main.focusRegion)
	}

	// The active page must have switched to "network".
	if app.Main.activePage != "network" {
		t.Errorf("after Enter: activePage=%q, want network", app.Main.activePage)
	}
}
