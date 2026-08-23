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

// TestGuideTopicListFocusAfterRender verifies that after the Guide page
// renders (including the first-run ShowFirstRun + SelectPage flow), focus
// lands on the IndicativeListBox (the topic list), NOT the ScrollBar.
// This is the Python behavior: Guide.py:215 focus_column=0 (topic list),
// and even on first run the display_topic→focus_reader is followed by
// the page switch which resets focus to the first focusable column.
//
// The test also verifies that bodyListAtTop recognizes the focused widget
// (so the Up-at-top→menu transition works when at item 0), and that
// keyboard Down navigates the topic list (not the reader).
func TestGuideTopicListFocusAfterRender(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)

	// Simulate the first-run flow: ShowFirstRun sets focus to the reader
	// (ScrollBar), then SelectPage("guide") switches the page, which
	// cascades focus to the first focusable column (the topic list).
	gd.ShowFirstRun()
	app.Main.SelectPage("guide")
	app.Main.Root().Draw(screen)

	// Focus must be on the IndicativeListBox (topic list), not the ScrollBar.
	focus := app.GetFocus()
	if _, ok := focus.(*IndicativeListBox); !ok {
		t.Errorf("after Guide render: focus=%T, want *IndicativeListBox (topic list, not ScrollBar)", focus)
	}

	// Move to item 0 and verify bodyListAtTop recognizes the IndicativeListBox
	// (so the Up-at-top→menu transition works). This is the specific bug from
	// the TODO: bodyListAtTop didn't recognize the focused widget.
	gd.topics.SetCurrentItem(0)
	app.Main.Root().Draw(screen)
	if !app.Main.bodyListAtTop() {
		t.Errorf("bodyListAtTop=false for focused IndicativeListBox at item 0; want true (Up-at-top must reach the menu)")
	}

	// Keyboard Down must navigate the topic list (cursor moves down
	// within the topic list, not the reader).
	prevItem := gd.topics.GetCurrentItem()
	handler := gd.topicsList.InputHandler()
	if handler == nil {
		t.Fatal("topicsList has no InputHandler")
	}
	handler(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) })
	app.Main.Root().Draw(screen)

	newItem := gd.topics.GetCurrentItem()
	if newItem <= prevItem {
		t.Errorf("after Down: topic item=%d, want > %d (Down must navigate the topic list)", newItem, prevItem)
	}
}
