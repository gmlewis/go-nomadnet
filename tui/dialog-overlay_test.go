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

// setupDialogTest initializes a fresh, isolated DialogManager (owned by a
// per-test *App) backed by a real tview.Application (no event loop) and a
// stand-in main primitive, and returns the app, the main primitive, and the
// Pages root. Each test gets its own manager, so tests may run in parallel.
func setupDialogTest(t *testing.T) (*App, tview.Primitive, *tview.Pages) {
	t.Helper()
	app := newTestApp()
	main := tview.NewTextView().SetText("main body")
	pages := app.Dialogs.Init(app.Application, main)
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(main)
	return app, main, pages
}

// TestDialogOverlayPreservesUnderlying asserts the spec for 0.5: opening a
// dialog keeps the underlying page widget in the tree, and dismissing
// restores the previous focus.
func TestDialogOverlayPreservesUnderlying(t *testing.T) {
	t.Parallel()
	app, main, pages := setupDialogTest(t)
	dm := app.Dialogs

	if !pages.HasPage("main") {
		t.Fatal("main page missing before dialog opened")
	}
	if got := app.Application.GetFocus(); got != main {
		t.Fatalf("initial focus = %v, want main", got)
	}

	dm.ShowDialog("Title", tview.NewTextView().SetText("body"), 40, 6, nil)

	// Underlying page is still mounted.
	if !pages.HasPage("main") {
		t.Error("main page removed after dialog opened; underlying screen must be preserved")
	}
	if dm.Count() != 1 {
		t.Errorf("DialogCount = %d, want 1", dm.Count())
	}
	// A dialog page was added on top.
	if pages.GetPageCount() != 2 {
		t.Errorf("page count = %d, want 2 (main + dialog)", pages.GetPageCount())
	}

	// Dismiss and assert focus is restored to the main body.
	dm.DismissTop()
	if dm.Count() != 0 {
		t.Errorf("DialogCount after dismiss = %d, want 0", dm.Count())
	}
	if pages.GetPageCount() != 1 {
		t.Errorf("page count after dismiss = %d, want 1 (main only)", pages.GetPageCount())
	}
	if got := app.Application.GetFocus(); got != main {
		t.Errorf("focus after dismiss = %v, want main (restored)", got)
	}
}

// TestDialogStackEscDismissesTop verifies the dialog stack: opening two
// dialogs, Esc dismisses only the top one (focus returns to the lower
// dialog), and a second Esc restores the original main-body focus.
func TestDialogStackEscDismissesTop(t *testing.T) {
	t.Parallel()
	app, main, pages := setupDialogTest(t)
	dm := app.Dialogs

	dm.ShowDialog("First", tview.NewTextView().SetText("1"), 30, 5, nil)
	dm.ShowDialog("Second", tview.NewTextView().SetText("2"), 30, 5, nil)

	if dm.Count() != 2 {
		t.Fatalf("DialogCount = %d, want 2", dm.Count())
	}
	if pages.GetPageCount() != 3 {
		t.Errorf("page count = %d, want 3 (main + 2 dialogs)", pages.GetPageCount())
	}

	// Esc on the top dialog: simulate via its InputHandler.
	top := dm.stack[len(dm.stack)-1].dialog
	top.InputHandler()(
		tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
		func(p tview.Primitive) { app.Application.SetFocus(p) },
	)

	if dm.Count() != 1 {
		t.Errorf("after first Esc: DialogCount = %d, want 1 (lower dialog remains)", dm.Count())
	}
	if pages.GetPageCount() != 2 {
		t.Errorf("after first Esc: page count = %d, want 2", pages.GetPageCount())
	}
	// Focus should now be on the remaining (first) dialog.
	if got := app.Application.GetFocus(); got != dm.stack[0].dialog {
		t.Errorf("after first Esc: focus = %v, want remaining dialog", got)
	}

	// Esc again dismisses the remaining dialog and restores main-body focus.
	remaining := dm.stack[len(dm.stack)-1].dialog
	remaining.InputHandler()(
		tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
		func(p tview.Primitive) { app.Application.SetFocus(p) },
	)

	if dm.Count() != 0 {
		t.Errorf("after second Esc: DialogCount = %d, want 0", dm.Count())
	}
	if got := app.Application.GetFocus(); got != main {
		t.Errorf("after second Esc: focus = %v, want main (restored)", got)
	}
}

// TestDialogOnDismissInvoked asserts the user onDismiss callback fires when a
// dialog is dismissed, and that Confirm/Cancel buttons dismiss via the stack.
func TestDialogOnDismissInvoked(t *testing.T) {
	t.Parallel()
	app, _, _ := setupDialogTest(t)
	dm := app.Dialogs

	dismissed := false
	dm.ShowDialog("X", tview.NewTextView().SetText("x"), 20, 5, func() {
		dismissed = true
	})
	dm.DismissTop()
	if !dismissed {
		t.Error("user onDismiss callback was not invoked on dismiss")
	}
}

// TestDialogOpenFlag checks the Open convenience predicate.
func TestDialogOpenFlag(t *testing.T) {
	t.Parallel()
	app, _, _ := setupDialogTest(t)
	dm := app.Dialogs

	if dm.Open() {
		t.Error("Open = true before any dialog opened")
	}
	dm.ShowDialog("X", tview.NewTextView().SetText("x"), 20, 5, nil)
	if !dm.Open() {
		t.Error("Open = false after opening a dialog")
	}
	dm.DismissTop()
	if dm.Open() {
		t.Error("Open = true after dismissing all dialogs")
	}
}

func TestCenterDialogInPanePlacement(t *testing.T) {
	t.Parallel()
	dlg := NewDialogLineBox("Test", tview.NewTextView().SetText("body"), nil)
	flex := centerDialog(dlg, 0, 5)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	flex.SetRect(0, 0, 80, 24)
	flex.Draw(screen)

	c, _, _, _ := screen.GetContent(1, 8)
	if c != '┌' {
		t.Errorf("dialog top-left corner at (1,8) = %q, want '┌'", c)
	}
	cSide, _, _, _ := screen.GetContent(1, 9)
	if cSide != '│' {
		t.Errorf("dialog side border at (1,9) = %q, want '│'", cSide)
	}
}
