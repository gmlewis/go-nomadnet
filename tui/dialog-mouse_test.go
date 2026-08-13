// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestStatusDialogOKButtonMouseClick reproduces the reported "click OK and
// nothing happens" bug: a mouse click on the dialog's OK button must fire the
// button's selected callback (DismissTop) and close the dialog. Before the
// fix, DialogLineBox had no MouseHandler, so clicks never reached the button
// (only Enter worked, via InputHandler).
func TestStatusDialogOKButtonMouseClick(t *testing.T) {
	app, _, _ := setupDialogTest(t)
	dm := app.Dialogs

	dm.ShowStatusDialog("Saved", "\n\n\nSaved\n\n", 40, 9)
	if dm.Count() != 1 {
		t.Fatalf("count=%d want 1", dm.Count())
	}

	// Render so every primitive's rect is laid out (Draw sets the dialog
	// content's inner rect, which the button's InRect check gates on).
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(60, 14)
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	root := dm.Pages()
	root.SetRect(0, 0, 60, 14)
	root.Draw(screen)
	screen.Show()

	// Locate the row that renders the OK button and click its center.
	okY := -1
	for y := range 14 {
		var line strings.Builder
		for x := range 60 {
			c, _, _, _ := cellContent(screen, x, y)
			line.WriteRune(c)
		}
		if strings.Contains(line.String(), "OK") {
			okY = y
			break
		}
	}
	if okY < 0 {
		t.Fatal("OK button not found on screen")
	}
	clickX := 30
	t.Logf("clicking OK at (%d,%d)", clickX, okY)

	clickAt := func(action tview.MouseAction, buttons tcell.ButtonMask) {
		ev := tcell.NewEventMouse(clickX, okY, buttons, tcell.ModNone)
		if h := root.MouseHandler(); h != nil {
			h(action, ev, func(p tview.Primitive) { app.Application.SetFocus(p) })
		}
	}
	// Faithful click sequence: Down (focuses the button) then Click (activates).
	clickAt(tview.MouseLeftDown, tcell.Button1)
	clickAt(tview.MouseLeftClick, tcell.Button1)

	if dm.Count() != 0 {
		t.Errorf("clicking OK did NOT dismiss the dialog (count=%d) — DialogLineBox is not forwarding mouse events to its content", dm.Count())
	}
}
