// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dispatchKey mirrors tview.Application's real key dispatch (application.go
// ~417-446): the app-level inputCapture runs FIRST (MainDisplay.handleInput),
// and only if it returns non-nil does the event reach root.InputHandler. The
// live app takes this path; calling root.InputHandler directly would skip the
// global capture where the Esc→DismissTop guard lives.
func dispatchKey(app *App, root tview.Primitive, event *tcell.EventKey) {
	if cap := app.Application.GetInputCapture(); cap != nil {
		event = cap(event)
		if event == nil {
			return
		}
	}
	if root != nil && root.HasFocus() {
		if h := root.InputHandler(); h != nil {
			h(event, func(p tview.Primitive) { app.Application.SetFocus(p) })
		}
	}
}

// TestShowStatusDialogDismisses reproduces and pins the fix for the reported
// "Saved modal never goes away" bug. After a mouse click opens a status dialog,
// tview can leave focus on the underlying page (the clicked button) rather
// than the dialog, so Esc reaches the page (which ignores it) instead of the
// dialog's dismiss handler. Two independent fixes are verified here:
//
//  1. Esc dismisses regardless of focus, because MainDisplay.handleInput (the
//     app-level capture) routes Esc to DismissTop whenever a dialog is open.
//  2. The OK button gives a guaranteed dismiss path (Enter/Space when the
//     button has focus, or a mouse click) that does not depend on Esc routing.
//
// The "focus stuck on main" case below simulates the mouse-click race by
// forcing focus back onto the main page after the dialog is shown; without
// the guard, Esc would be lost there.
func TestShowStatusDialogDismisses(t *testing.T) {
	app := newTestApp()
	md := NewMainDisplay(app, 0, "default")
	main := md.frame
	pages := app.Dialogs.Init(app.Application, main)
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(main)

	// (1) Esc dismisses even when focus is stuck on the main page.
	app.Dialogs.ShowStatusDialog("Saved", "\n\n\nSaved\n\n", 40, 9)
	if app.Dialogs.Count() != 1 {
		t.Fatalf("after ShowStatusDialog: count=%d want 1", app.Dialogs.Count())
	}
	// Simulate the mouse-click focus race: focus stays on the main page, not
	// the dialog.
	app.Application.SetFocus(main)

	esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
	dispatchKey(app, pages, esc)
	if app.Dialogs.Count() != 0 {
		t.Errorf("Esc with focus on main: dialog NOT dismissed (count=%d) — the global Esc guard is missing", app.Dialogs.Count())
	}

	// (2) The OK button dismisses via Enter when the dialog has focus.
	app.Dialogs.ShowStatusDialog("Saved", "\n\n\nSaved\n\n", 40, 9)
	if app.Dialogs.Count() != 1 {
		t.Fatalf("second ShowStatusDialog: count=%d want 1", app.Dialogs.Count())
	}
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	dispatchKey(app, pages, enter)
	if app.Dialogs.Count() != 0 {
		t.Errorf("Enter on OK button: dialog NOT dismissed (count=%d)", app.Dialogs.Count())
	}

	// (3) Esc dismisses in the normal (focus on dialog) case too.
	app.Dialogs.ShowStatusDialog("Saved", "\n\n\nSaved\n\n", 40, 9)
	dispatchKey(app, pages, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.Dialogs.Count() != 0 {
		t.Errorf("Esc with focus on dialog: dialog NOT dismissed (count=%d)", app.Dialogs.Count())
	}
}

// TestShowStatusDialogHasOKButton verifies the dialog content includes a
// focused OK button (the visible dismiss affordance the Python original has
// and the bare-text port lacked).
func TestShowStatusDialogHasOKButton(t *testing.T) {
	app, _, pages := setupDialogTest(t)
	dm := app.Dialogs

	dm.ShowStatusDialog("Saved", "\n\n\nSaved\n\n", 40, 9)
	if dm.Count() != 1 {
		t.Fatalf("count=%d want 1", dm.Count())
	}
	// Pressing Enter dismisses via the OK button (the focused content).
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	if h := pages.InputHandler(); h != nil {
		h(enter, func(p tview.Primitive) { app.Application.SetFocus(p) })
	}
	if dm.Count() != 0 {
		t.Errorf("Enter did not dismiss the status dialog (count=%d) — OK button missing or not focused", dm.Count())
	}
}

// TestEscDismissesDialogWhenMainFocused uses a bare-TextView dialog (no OK
// button) to specifically pin the global Esc guard: with focus forced onto the
// main page, Esc must still dismiss. This is the exact shape of the original
// "Saved" dialog and the exact failure the user reported.
func TestEscDismissesDialogWhenMainFocused(t *testing.T) {
	app := newTestApp()
	md := NewMainDisplay(app, 0, "default")
	main := md.frame
	pages := app.Dialogs.Init(app.Application, main)
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(main)

	app.Dialogs.ShowDialog("Saved",
		tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("\n\n\nSaved\n\n"),
		40, 9, nil)
	app.Application.SetFocus(main) // simulate the mouse-click focus race

	dispatchKey(app, pages, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.Dialogs.Count() != 0 {
		t.Errorf("bare-text dialog with focus on main: Esc did NOT dismiss (count=%d)", app.Dialogs.Count())
	}
}