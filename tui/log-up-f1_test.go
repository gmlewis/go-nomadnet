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
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestF1FirstUpGoesToMenu pins F1: Python's LogTerminal.keypress sends "up"
// straight to the main display header (Log.py:58-61) — the FIRST Up collapses
// focus to the menu regardless of scroll position (the embedded `tail -fn50`
// terminal never scrolls via keys, so there is no scroll-then-escape dance).
// The Go port used to scroll its TextView and only escape at the very top.
func TestF1FirstUpGoesToMenu(t *testing.T) {
	t.Parallel()

	// A log file whose content far exceeds the viewport, so the view can be
	// scrolled away from the top (the precondition the old behavior needed).
	logPath := "/tmp/parity-a-f1-test.log"
	var sb strings.Builder
	for i := range 300 {
		sb.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	if err := os.WriteFile(logPath, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(logPath) })

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	ld := NewLogDisplay(app, logPath, 50)
	app.Main.SetDisplay("log", ld.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("log")
	app.Main.FocusBody()
	app.Main.Root().Draw(screen)

	if got := app.GetFocus(); got != tview.Primitive(ld.logView) && got != tview.Primitive(ld.widget) {
		t.Fatalf("setup: focus = %T, want the log view", got)
	}

	// The FIRST Up goes to the menu even when the log view is NOT scrolled to
	// its top: scroll the view down first so logAtTop() is false.
	ld.logView.ScrollTo(3, 0)
	app.Main.Root().Draw(screen)
	if ld.logAtTop() {
		t.Fatal("setup: log view should not be at the top after ScrollTo(3)")
	}

	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after first Up = %T, want the menu bar (Log.py:58-61)", got)
	}
}
