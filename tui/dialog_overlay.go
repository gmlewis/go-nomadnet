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
	"fmt"
	"sync"

	"github.com/rivo/tview"
)

// dialogEntry is one frame on the modal dialog stack.
type dialogEntry struct {
	pageName  string          // Pages page name for this dialog's overlay
	dialog    *DialogLineBox  // the focused dialog primitive
	overlay   tview.Primitive // the centering wrapper added to Pages
	onDismiss func()          // user callback invoked when this dialog closes
	prevFocus tview.Primitive // focus to restore when the stack empties
}

// DialogManager overlays a stack of modal dialogs on top of the main content
// using a tview.Pages root. The main content stays mounted underneath every
// dialog (so the underlying screen is preserved); Esc dismisses the top
// dialog and restores the previous focus. This replaces the old
// app.SetRoot(dialog, true) approach that destroyed the underlying screen.
//
// One manager per process is held in the package-global dialogManager; it is
// wired up by InitDialogManager (called from App.SetRoot).
type DialogManager struct {
	mu    sync.Mutex
	app   *tview.Application
	pages *tview.Pages
	main  tview.Primitive
	stack []*dialogEntry
	seq   int
}

var dialogManager = &DialogManager{}

// InitDialogManager configures the package-global dialog manager with the
// application and its main content primitive, and returns the tview.Pages
// root that callers should pass to Application.SetRoot. The main content is
// added as the always-present bottom page named "main".
func InitDialogManager(app *tview.Application, main tview.Primitive) *tview.Pages {
	pages := tview.NewPages()
	pages.AddPage("main", main, true, true)

	dialogManager.mu.Lock()
	defer dialogManager.mu.Unlock()
	dialogManager.app = app
	dialogManager.pages = pages
	dialogManager.main = main
	dialogManager.stack = nil
	dialogManager.seq = 0
	return pages
}

// DialogManagerPages returns the Pages root backing the dialog overlay, or
// nil if InitDialogManager has not been called.
func DialogManagerPages() *tview.Pages {
	dialogManager.mu.Lock()
	defer dialogManager.mu.Unlock()
	return dialogManager.pages
}

// DialogCount returns the number of currently-open dialogs on the stack.
func DialogCount() int {
	dialogManager.mu.Lock()
	defer dialogManager.mu.Unlock()
	return len(dialogManager.stack)
}

// DialogOpen reports whether any modal dialog is currently open.
func DialogOpen() bool {
	return DialogCount() > 0
}

// showDialogOverlay pushes a centered dialog onto the stack, records the
// current focus for later restoration, mounts it as a new top page, and
// focuses the dialog. It is the shared implementation behind ShowDialog.
func showDialogOverlay(app *tview.Application, title string, content tview.Primitive, width, height int, onDismiss func()) {
	dm := dialogManager
	dm.mu.Lock()
	if dm.pages == nil || dm.app == nil {
		dm.mu.Unlock()
		// Not initialized (ad-hoc use without InitDialogManager): fall back
		// to replacing the root so the dialog is at least visible.
		dialog := NewDialogLineBox(title, content, onDismiss)
		app.SetRoot(centerDialog(dialog, width, height), true)
		app.SetFocus(dialog)
		return
	}

	prevFocus := app.GetFocus()
	dialog := NewDialogLineBox(title, content, func() { dismissTopDialog() })
	entry := &dialogEntry{
		pageName:  fmt.Sprintf("dialog-%d", dm.seq),
		dialog:    dialog,
		overlay:   centerDialog(dialog, width, height),
		onDismiss: onDismiss,
		prevFocus: prevFocus,
	}
	dm.seq++
	dm.stack = append(dm.stack, entry)
	pages := dm.pages
	// Mutate the shared tview.Pages and app focus while holding the lock.
	// tview.Pages (and its focus state) is not concurrency-safe, and the
	// package-global dialogManager is shared across tests (and across any
	// goroutines that call ShowDialog); serializing AddPage/SetFocus here
	// prevents data races on tview's internal pages slice and Box.focus.
	pages.AddPage(entry.pageName, entry.overlay, true, true)
	app.SetFocus(dialog)
	dm.mu.Unlock()
}

// dismissTopDialog pops the top dialog off the stack, removes its page (so
// the underlying content is revealed), and restores focus to the now-top
// dialog or — when the stack is empty — to the focus that preceded the
// first dialog. The closed dialog's user onDismiss callback is invoked last.
func dismissTopDialog() {
	dm := dialogManager
	dm.mu.Lock()
	if len(dm.stack) == 0 {
		dm.mu.Unlock()
		return
	}
	n := len(dm.stack)
	top := dm.stack[n-1]
	dm.stack = dm.stack[:n-1]
	pages := dm.pages
	app := dm.app

	var focus tview.Primitive
	if len(dm.stack) > 0 {
		focus = dm.stack[len(dm.stack)-1].dialog
	} else {
		focus = top.prevFocus
	}
	onDismiss := top.onDismiss
	// Remove the page and restore focus under the lock for the same
	// concurrency-safety reasons as showDialogOverlay (shared tview.Pages
	// and app focus are not safe for concurrent access).
	pages.RemovePage(top.pageName)
	if focus != nil && app != nil {
		app.SetFocus(focus)
	}
	dm.mu.Unlock()

	// The user callback runs outside the lock: it may re-enter ShowDialog
	// (pushing a new dialog), which would otherwise self-deadlock on dm.mu.
	if onDismiss != nil {
		onDismiss()
	}
}

// DismissTopDialog closes the topmost open dialog (if any). This is the
// programmatic equivalent of pressing Esc on the focused dialog.
func DismissTopDialog() {
	dismissTopDialog()
}

// centerDialog wraps content in a full-screen Flex that centers it at the
// given width and height. The surrounding space is transparent so the
// underlying page shows through.
func centerDialog(content tview.Primitive, width, height int) tview.Primitive {
	row := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(content, width, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(row, height, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)
}
