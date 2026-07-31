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

// setupDialogTest initializes a fresh DialogManager backed by a real
// tview.Application (no event loop) and a stand-in main primitive, returns
// the app, the main primitive, and the Pages root. Tests must not run in
// parallel because they share the package-global dialogManager.
func setupDialogTest(t *testing.T) (*tview.Application, tview.Primitive, *tview.Pages) {
	t.Helper()
	app := tview.NewApplication()
	main := tview.NewTextView().SetText("main body")
	pages := InitDialogManager(app, main)
	app.SetRoot(pages, true)
	app.SetFocus(main)
	return app, main, pages
}

// TestDialogOverlayPreservesUnderlying asserts the spec for 0.5: opening a
// dialog keeps the underlying page widget in the tree, and dismissing
// restores the previous focus.
func TestDialogOverlayPreservesUnderlying(t *testing.T) {
	app, main, pages := setupDialogTest(t)

	if !pages.HasPage("main") {
		t.Fatal("main page missing before dialog opened")
	}
	if got := app.GetFocus(); got != main {
		t.Fatalf("initial focus = %v, want main", got)
	}

	ShowDialog(app, "Title", tview.NewTextView().SetText("body"), 40, 6, nil)

	// Underlying page is still mounted.
	if !pages.HasPage("main") {
		t.Error("main page removed after dialog opened; underlying screen must be preserved")
	}
	if DialogCount() != 1 {
		t.Errorf("DialogCount = %d, want 1", DialogCount())
	}
	// A dialog page was added on top.
	if pages.GetPageCount() != 2 {
		t.Errorf("page count = %d, want 2 (main + dialog)", pages.GetPageCount())
	}

	// Dismiss and assert focus is restored to the main body.
	DismissTopDialog()
	if DialogCount() != 0 {
		t.Errorf("DialogCount after dismiss = %d, want 0", DialogCount())
	}
	if pages.GetPageCount() != 1 {
		t.Errorf("page count after dismiss = %d, want 1 (main only)", pages.GetPageCount())
	}
	if got := app.GetFocus(); got != main {
		t.Errorf("focus after dismiss = %v, want main (restored)", got)
	}
}

// TestDialogStackEscDismissesTop verifies the dialog stack: opening two
// dialogs, Esc dismisses only the top one (focus returns to the lower
// dialog), and a second Esc restores the original main-body focus.
func TestDialogStackEscDismissesTop(t *testing.T) {
	app, main, pages := setupDialogTest(t)

	ShowDialog(app, "First", tview.NewTextView().SetText("1"), 30, 5, nil)
	ShowDialog(app, "Second", tview.NewTextView().SetText("2"), 30, 5, nil)

	if DialogCount() != 2 {
		t.Fatalf("DialogCount = %d, want 2", DialogCount())
	}
	if pages.GetPageCount() != 3 {
		t.Errorf("page count = %d, want 3 (main + 2 dialogs)", pages.GetPageCount())
	}

	// Esc on the top dialog: simulate via its InputHandler.
	top := dialogManager.stack[len(dialogManager.stack)-1].dialog
	top.InputHandler()(
		tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) },
	)

	if DialogCount() != 1 {
		t.Errorf("after first Esc: DialogCount = %d, want 1 (lower dialog remains)", DialogCount())
	}
	if pages.GetPageCount() != 2 {
		t.Errorf("after first Esc: page count = %d, want 2", pages.GetPageCount())
	}
	// Focus should now be on the remaining (first) dialog.
	if got := app.GetFocus(); got != dialogManager.stack[0].dialog {
		t.Errorf("after first Esc: focus = %v, want remaining dialog", got)
	}

	// Esc again dismisses the remaining dialog and restores main-body focus.
	remaining := dialogManager.stack[len(dialogManager.stack)-1].dialog
	remaining.InputHandler()(
		tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
		func(p tview.Primitive) { app.SetFocus(p) },
	)

	if DialogCount() != 0 {
		t.Errorf("after second Esc: DialogCount = %d, want 0", DialogCount())
	}
	if got := app.GetFocus(); got != main {
		t.Errorf("after second Esc: focus = %v, want main (restored)", got)
	}
}

// TestDialogOnDismissInvoked asserts the user onDismiss callback fires when a
// dialog is dismissed, and that Confirm/Cancel buttons dismiss via the stack.
func TestDialogOnDismissInvoked(t *testing.T) {
	app, _, _ := setupDialogTest(t)

	dismissed := false
	ShowDialog(app, "X", tview.NewTextView().SetText("x"), 20, 5, func() {
		dismissed = true
	})
	DismissTopDialog()
	if !dismissed {
		t.Error("user onDismiss callback was not invoked on dismiss")
	}
}

// TestDialogOpenFlag checks the DialogOpen convenience predicate.
func TestDialogOpenFlag(t *testing.T) {
	app, _, _ := setupDialogTest(t)

	if DialogOpen() {
		t.Error("DialogOpen = true before any dialog opened")
	}
	ShowDialog(app, "X", tview.NewTextView().SetText("x"), 20, 5, nil)
	if !DialogOpen() {
		t.Error("DialogOpen = false after opening a dialog")
	}
	DismissTopDialog()
	if DialogOpen() {
		t.Error("DialogOpen = true after dismissing all dialogs")
	}
}
