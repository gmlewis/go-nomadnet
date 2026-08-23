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

// TestFullscreenHidesLeftPane verifies that ToggleFullscreen collapses the
// left pane to width 0 (hidden) so the browser pane fills the full width,
// matching Python's NetworkDisplay.toggle_fullscreen (Network.py:1678-1686).
// Before the fix, SetFixedWidth(0, 0) was treated as "not fixed" (use
// weight) instead of "fixed at 0" (hidden), so both panes remained visible.
func TestFullscreenHidesLeftPane(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "aaaa", DisplayName: "Node A"},
	})
	app.Main.SetDisplay("network", nd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)

	app.Main.SelectPage("network")
	app.Main.Root().Draw(screen)

	// Before fullscreen: left pane should be visible (width > 0).
	leftX, _, leftW, _ := nd.leftPanel.GetRect()
	if leftW <= 0 {
		t.Fatalf("before fullscreen: left pane width=%d, want > 0", leftW)
	}

	// Toggle fullscreen — should hide the left pane.
	nd.ToggleFullscreen()
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.Root().Draw(screen)

	_, _, leftW2, _ := nd.leftPanel.GetRect()
	if leftW2 != 0 {
		t.Errorf("after fullscreen: left pane width=%d, want 0 (hidden)", leftW2)
	}

	// The browser pane should fill the full width.
	browserX, _, browserW, _ := nd.browser.Widget().GetRect()
	if browserW < 130 {
		t.Errorf("after fullscreen: browser width=%d, want >= 130 (full width)", browserW)
	}
	_ = leftX
	_ = browserX
}
