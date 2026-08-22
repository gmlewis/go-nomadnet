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
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// focusDumpMu serializes tests that swap the package-global focusInvariantDump
// sink so they cannot race with each other. These tests are intentionally not
// parallel.
var focusDumpMu sync.Mutex

// captureFocusDump swaps focusInvariantDump for a sink that records the
// violation message + stack, and returns a restore func plus the capture slice.
// Callers must hold focusDumpMu and defer restore().
func captureFocusDump() (restore func(), got *[]string) {
	var captured []string
	prev := focusInvariantDump
	focusInvariantDump = func(msg string, stack []byte) {
		captured = append(captured, msg+"\n"+string(stack))
	}
	return func() { focusInvariantDump = prev }, &captured
}

// TestHandleInputRecoversNilFocus reproduces the lock-up root cause: the app's
// focused primitive becomes nil (the diag log showed GetFocus()=<nil> for key
// after key, so arrow keys dispatched to nothing). MainDisplay.handleInput must
// recover focus before dispatching and dump a stack so the violation surfaces.
// The nil is forced via the embedded *tview.Application.SetFocus (bypassing the
// *App shadow) to mimic tview's internal cascade delegate(nil).
func TestHandleInputRecoversNilFocus(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	app.Application.SetRoot(app.Dialogs.Init(app.Application, md.Root()), true)
	// Establish a real focused primitive, then force the nil state.
	app.Application.SetFocus(md.menuBar)
	app.Application.SetFocus(nil)
	if got := app.Application.GetFocus(); got != nil {
		t.Fatalf("precondition: GetFocus=%v, want nil", got)
	}

	md.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))

	if got := app.Application.GetFocus(); got == nil {
		t.Fatal("after handleInput with nil focus: GetFocus=nil, want recovered non-nil")
	}
	if len(*dumps) == 0 {
		t.Fatal("nil-focus violation was not reported (no stack dump captured)")
	}
	if !strings.Contains((*dumps)[0], "nil focus") {
		t.Errorf("dump message = %q, want it to mention nil focus", (*dumps)[0])
	}
}

// TestDismissTopRestoresFocusOnNilPrevFocus pins the dialog-overlay fix: when a
// dialog was opened while focus was already nil (prevFocus=nil), the old code
// skipped SetFocus and left the app with no focused primitive. DismissTop must
// now fall back to the main content so focus is never nil after a dismiss.
func TestDismissTopRestoresFocusOnNilPrevFocus(t *testing.T) {
	app, main, pages := setupDialogTest(t)
	app.Application.SetRoot(pages, true)
	// Force the nil state, then open a dialog (its prevFocus capture will be
	// nil), then dismiss — focus must be restored, not left nil.
	app.Application.SetFocus(nil)
	if got := app.Application.GetFocus(); got != nil {
		t.Fatalf("precondition: GetFocus=%v, want nil", got)
	}
	app.Dialogs.ShowDialog("test", tview.NewTextView(), 30, 5, nil)
	if app.Dialogs.Count() != 1 {
		t.Fatalf("dialog count=%d, want 1", app.Dialogs.Count())
	}
	app.Dialogs.DismissTop()
	if got := app.Application.GetFocus(); got == nil {
		t.Fatal("after DismissTop with nil prevFocus: GetFocus=nil, want restored to main")
	}
	// The fallback target is dm.main, so focus should have cascaded onto main
	// (a leaf TextView whose Box.Focus holds focus).
	if got := app.Application.GetFocus(); got != main {
		t.Errorf("after DismissTop: GetFocus=%v, want main (nil-prevFocus fallback)", got)
	}
}

// TestAppSetFocusRefusesNil verifies the *App.SetFocus shadow enforces the
// invariant: a nil argument is refused (dumped + no-op) rather than nil-ing
// a.focus, which would freeze keyboard input.
func TestAppSetFocusRefusesNil(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	restore, dumps := captureFocusDump()
	defer restore()

	app := newTestApp()
	main := tview.NewTextView()
	app.Application.SetRoot(main, true)
	app.Application.SetFocus(main)
	if got := app.GetFocus(); got != main {
		t.Fatalf("precondition: GetFocus=%v, want main", got)
	}

	app.SetFocus(nil) // shadow must refuse, not nil focus.

	if got := app.GetFocus(); got != main {
		t.Errorf("after SetFocus(nil): GetFocus=%v, want main (nil refused, focus unchanged)", got)
	}
	if len(*dumps) == 0 {
		t.Fatal("SetFocus(nil) was not reported (no stack dump captured)")
	}
}

// TestSetFocusInvariantSink pins the wiring API the cmd/gonomadnet layer uses to
// route focus-invariant dumps into the app logger (the "[ Log ]" menu's file):
// SetFocusInvariantSink redirects the sink, and a nil argument restores the
// default /tmp sink. This is what makes the stack trace appear in-menu rather
// than only in a scratch file.
func TestSetFocusInvariantSink(t *testing.T) {
	focusDumpMu.Lock()
	defer focusDumpMu.Unlock()
	defer SetFocusInvariantSink(nil) // restore default

	var got string
	SetFocusInvariantSink(func(msg string, stack []byte) {
		got = msg + "|" + string(stack)
	})
	dumpFocusInvariantViolation("wired-sink-test")

	if !strings.Contains(got, "wired-sink-test") {
		t.Errorf("wired sink captured %q, want it to contain the message", got)
	}
	if !strings.Contains(got, "focus-invariant_test.go") {
		t.Errorf("wired sink captured no stack (want this file in the trace), got %q", got)
	}

	// nil restores the default sink (no panic, dump goes to /tmp again).
	SetFocusInvariantSink(nil)
	dumpFocusInvariantViolation("default-restored") // must not panic.
}
