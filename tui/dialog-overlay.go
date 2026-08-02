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

	"github.com/gdamore/tcell/v2"
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
// A DialogManager is owned by App (App.Dialogs) and wired up by Init (called
// from App.SetRoot). Keeping it off the package level lets parallel tests
// each use an isolated manager.
type DialogManager struct {
	mu    sync.Mutex
	app   *tview.Application
	pages *tview.Pages
	main  tview.Primitive
	stack []*dialogEntry
	seq   int
}

// Init configures the dialog manager with the application and its main
// content primitive, and returns the tview.Pages root that callers should
// pass to Application.SetRoot. The main content is added as the always-present
// bottom page named "main".
func (dm *DialogManager) Init(app *tview.Application, main tview.Primitive) *tview.Pages {
	pages := tview.NewPages()
	pages.AddPage("main", main, true, true)

	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.app = app
	dm.pages = pages
	dm.main = main
	dm.stack = nil
	dm.seq = 0
	return pages
}

// Pages returns the tview.Pages root backing the dialog overlay, or nil if
// Init has not been called.
func (dm *DialogManager) Pages() *tview.Pages {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.pages
}

// Count returns the number of currently-open dialogs on the stack.
func (dm *DialogManager) Count() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return len(dm.stack)
}

// Open reports whether any modal dialog is currently open.
func (dm *DialogManager) Open() bool {
	return dm.Count() > 0
}

// showOverlay pushes a centered dialog onto the stack, records the current
// focus for later restoration, mounts it as a new top page, and focuses the
// dialog. It is the shared implementation behind ShowDialog.
func (dm *DialogManager) showOverlay(app *tview.Application, title string, content tview.Primitive, width, height int, onDismiss func()) {
	dm.mu.Lock()
	if dm.pages == nil || dm.app == nil {
		dm.mu.Unlock()
		// Not initialized (ad-hoc use without Init): fall back to replacing
		// the root so the dialog is at least visible.
		dialog := NewDialogLineBox(title, content, onDismiss)
		app.SetRoot(centerDialog(dialog, width, height), true)
		app.SetFocus(dialog)
		return
	}

	prevFocus := app.GetFocus()
	dialog := NewDialogLineBox(title, content, func() { dm.dismissTop() })
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
	// tview.Pages (and its focus state) is not concurrency-safe; serializing
	// AddPage/SetFocus here prevents data races on tview's internal pages
	// slice and Box.focus.
	pages.AddPage(entry.pageName, entry.overlay, true, true)
	app.SetFocus(dialog)
	dm.mu.Unlock()
}

// dismissTop pops the top dialog off the stack, removes its page (so the
// underlying content is revealed), and restores focus to the now-top dialog
// or — when the stack is empty — to the focus that preceded the first dialog.
// The closed dialog's user onDismiss callback is invoked last.
func (dm *DialogManager) dismissTop() {
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
	// concurrency-safety reasons as showOverlay (shared tview.Pages and app
	// focus are not safe for concurrent access).
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

// DismissTop closes the topmost open dialog (if any). This is the
// programmatic equivalent of pressing Esc on the focused dialog.
func (dm *DialogManager) DismissTop() {
	dm.dismissTop()
}

// transparentBox is a spacer that draws nothing. Unlike tview.NewBox, whose
// DrawForSubclass fills its rect with the background color (clearing it), a
// transparentBox leaves the underlying screen untouched. centerDialog uses it
// for the dialog margin spacers so that a centered dialog's right/bottom border
// — which sits one cell outside the dialog box, inside the trailing margin's
// rect — is not overwritten when the margin is drawn after the dialog. (tview
// Flex.Draw only defers the focused item's draw to last; when focus is on a
// dialog's inner field rather than the dialog itself, the trailing margins
// would otherwise clear the border.)
type transparentBox struct {
	*tview.Box
}

// newTransparentBox returns a non-clearing spacer.
func newTransparentBox() *transparentBox {
	return &transparentBox{Box: tview.NewBox()}
}

// Draw does nothing — the spacer is transparent and does not clear its rect.
func (t *transparentBox) Draw(screen tcell.Screen) {}

// centerDialog wraps content in a full-screen Flex that centers it at the
// given width and height. The surrounding space is transparent (non-clearing)
// so the underlying page shows through and the dialog's borders survive
// regardless of which inner widget holds focus.
func centerDialog(content tview.Primitive, width, height int) tview.Primitive {
	var row *tview.Flex
	if width <= 0 {
		row = tview.NewFlex().
			AddItem(newTransparentBox(), 2, 0, false).
			AddItem(content, 0, 1, true).
			AddItem(newTransparentBox(), 2, 0, false)
	} else {
		row = tview.NewFlex().
			AddItem(newTransparentBox(), 0, 1, false).
			AddItem(content, width, 0, true).
			AddItem(newTransparentBox(), 0, 1, false)
	}
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(newTransparentBox(), 0, 1, false).
		AddItem(row, height, 0, true).
		AddItem(newTransparentBox(), 0, 1, false)
}
