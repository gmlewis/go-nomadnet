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

// newE1App mounts the Guide display in a fully wired app.
func newE1App(t *testing.T) (*App, *GuideDisplay) {
	t.Helper()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("guide")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)
	return app, gd
}

// TestE1TopicListNavigableAfterOpen pins E1: after opening a topic the focus
// moves to the reader (Python focus_reader, Guide.py:152-154), the reader's
// Up/Down navigate the reader's focus model (Python: Up/Down work on whichever
// Columns column is focused), and Left returns focus to the topics list
// (Python urwid Columns traversal + micron_released_focus → focus_topics,
// Guide.py:100-101 + 279-281) — after which the list is still arrow-navigable.
// The earlier Go build stranded focus on the reader's ScrollBar wrapper where
// arrows did nothing.
func TestE1TopicListNavigableAfterOpen(t *testing.T) {
	t.Parallel()

	app, gd := newE1App(t)

	// Open topic 1 from the list: Down then Enter.
	app.SetFocus(gd.topicsList)
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := gd.topics.GetCurrentItem(); got != 1 {
		t.Fatalf("topic selection after Down = %v, want 1", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if gd.currentIdx != 1 {
		t.Fatalf("topic 1 did not open (currentIdx=%v)", gd.currentIdx)
	}
	// Python focus_reader: the reader holds focus after a topic opens.
	if f := app.GetFocus(); f != tview.Primitive(gd.scroll) {
		t.Errorf("focus after opening a topic = %T, want the reader (ScrollBar wrapper)", f)
	}

	// The reader's Up/Down move the reader's focus model.
	before := gd.focusedLineIndex()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := gd.focusedLineIndex(); got == before {
		t.Errorf("reader focus did not advance after Down (%v)", got)
	}

	// Left returns to the topics list (micron_released_focus).
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if f := app.GetFocus(); f != tview.Primitive(gd.topicsList) {
		t.Fatalf("focus after Left from the reader = %T, want the topics list", f)
	}

	// The topics list is still arrow-navigable.
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := gd.topics.GetCurrentItem(); got != 2 {
		t.Errorf("topic selection after Down = %v, want 2 (list still navigable)", got)
	}
}

// TestE2UpOnFirstTopicToMenubar pins E2: Up on the FIRST topic collapses
// focus to the menubar (Python TopicList.keypress, Guide.py:191-195).
func TestE2UpOnFirstTopicToMenubar(t *testing.T) {
	t.Parallel()

	app, gd := newE1App(t)

	app.SetFocus(gd.topicsList)
	gd.topics.SetCurrentItem(0)

	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after Up on the first topic = %T, want the menu bar", got)
	}
}
