// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
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

// TestGuideReaderHardwareCursor pins B5 for the Guide reader: after a keypress
// the focused selectable line must drive the real terminal cursor (tcell
// ShowCursor), and each Down must move it to a different row so the
// tmux-test-suite's cursor-y tracker sees movement (and does not false-bottom
// during the within-viewport focus phase). Python positions the cursor via
// urwid LinkableText.render → canvas.cursor (MicronParser.py:982-992); the Go
// port mirrors this in guideReader.Draw → cursorScreenXY → ShowCursor, gated by
// the 2 s key-timeout (cursorVisible). Before any key the cursor is hidden
// (matching Python's key_timeout); after the first Down it appears and advances.
//
// A SimulationScreen is used because it exposes GetCursor — the in-process
// equivalent of `tmux display-message #{cursor_x},#{cursor_y}`. The live
// tmux-test-suite verifies the same property end-to-end (B2 walk reaches the
// bottom at ~500 downs only because cursorEverSeen becomes true).
func TestGuideReaderHardwareCursor(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)

	app.Main.SelectPage("guide")
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.Root().Draw(screen)

	gd.showTopic(7)
	app.SetFocus(gd.scroll) // FocusReader
	app.Main.Root().Draw(screen)

	// Drive the full input path (columns InputCapture → handleReaderKey →
	// focusDown → noteKey) like the live app, and draw after each Down. After the
	// first key the cursor must be in the READER pane (x >= 46, past the topics
	// list) and advance one row per Down, so the tmux-test-suite's cursor-y
	// tracker sees movement and does not false-bottom (B5).
	handler := gd.Widget().InputHandler()
	var prevY int = -1
	for i := 0; i < 6; i++ {
		handler(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
		app.Main.Root().Draw(screen)
		x, y, vis := screen.GetCursor()
		if !vis {
			t.Fatalf("Down#%d: cursor not visible; want visible (hasKey set by noteKey)", i+1)
		}
		if x < 46 {
			t.Fatalf("Down#%d: cursor x=%d, want >= 46 (in the reader pane, not the topics list)", i+1, x)
		}
		if y <= prevY {
			t.Errorf("Down#%d: cursor y=%d not greater than previous %d (cursor must advance per Down)", i+1, y, prevY)
		}
		prevY = y
	}
}
