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

// Regression tests for fleet bug #10 (2026-09-01): Ctrl-x on a conversation →
// confirm → Enter did NOTHING. The live diag trace showed the confirm dialog's
// focus stranded on the *tui.urwidColumns BUTTON-ROW CONTAINER (no focused
// child), where a bare Enter was dropped by the row's InputHandler — the Yes
// handler never fired and the conversation was never deleted.

// TestConfirmDialogYesOwnsFocus pins the guarantee that the Yes button owns
// the keyboard after the dialog opens: the app's focused primitive IS the Yes
// button, and Enter at it fires onYes.
func TestConfirmDialogYesOwnsFocus(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	yesFired, noFired := false, false
	app.Dialogs.ShowConfirmDialog("Delete conversation with <bbf3172fdf752ce1afc332ff44119a4f>?",
		func() { yesFired = true },
		func() { noFired = true })
	if app.Dialogs.Count() != 1 {
		t.Fatalf("confirm dialog not on the stack")
	}

	focused := app.GetFocus()
	b, ok := focused.(*UrwidButton)
	if !ok || b.Label() != "Yes" {
		t.Fatalf("confirm dialog focus = %T/%v, want the Yes UrwidButton", focused, focused)
	}

	// Enter at the focused primitive must fire onYes (not No, not nothing).
	if h := focused.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	}
	if !yesFired {
		t.Errorf("Enter on the confirm dialog did not fire onYes — the delete silently does nothing")
	}
	if noFired {
		t.Errorf("Enter fired the No button")
	}
	if app.Dialogs.Count() != 0 {
		t.Errorf("confirm dialog still open after Yes")
	}
}

// TestButtonRowEnterWithoutFocusedChild pins the row-level recovery: when a
// urwidColumns button row holds focus WITHOUT a focused child (the live
// observed state behind bug #10), Enter must reach the row's first focusable
// child (the primary action) instead of being silently dropped.
func TestButtonRowEnterWithoutFocusedChild(t *testing.T) {
	t.Parallel()
	yesFired, noFired := false, false
	yes := NewUrwidButton("Yes").SetSelectedFunc(func() { yesFired = true })
	no := NewUrwidButton("No").SetSelectedFunc(func() { noFired = true })
	row := CreateUrwidButtonRow(yes, no)
	row.SetRect(0, 0, 80, 1)

	// Simulate the interrupted cascade: the ROW's box claims focus while
	// every child is blurred (no child has focus).
	row.Blur()
	row.Box.Focus(nil)

	// Sanity: reproduce the broken pre-fix state — no child reports focus.
	anyChildFocused := false
	for _, child := range row.children {
		if child.HasFocus() {
			anyChildFocused = true
		}
	}
	if anyChildFocused {
		t.Fatalf("test setup: a child unexpectedly holds focus")
	}

	row.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if !yesFired {
		t.Errorf("Enter on a focused button row without a focused child fired nothing — want the first focusable button (Yes)")
	}
	if noFired {
		t.Errorf("Enter fired the No button — the fallback must pick the FIRST focusable child")
	}
}

// TestDeleteConversationEndToEnd walks the full Ctrl-x flow through the page
// dispatch: highlight a conversation, press Ctrl-x, confirm with Enter — the
// OnDeleteConv callback must fire with the SELECTED conversation (the wiring
// performs the actual delete + refresh).
func TestConversationsDeleteEndToEnd(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	conv := ConversationInfo{SourceHash: "bbf3172fdf752ce1afc332ff44119a4f", TrustLevel: "untrusted"}
	cd := NewConversationsDisplay(app, []ConversationInfo{conv})
	cd.SetShowTrusted(false) // the Untrusted tab (the bug report's state)

	deleted := ""
	cd.OnDeleteConv = func(conv ConversationInfo) { deleted = conv.SourceHash }

	// Ctrl-x while the list is focused and row 0 is highlighted.
	fired := cd.fireKey(t, tcell.KeyCtrlX)
	if !fired {
		t.Fatalf("Ctrl-x was not consumed by the list shortcut handler")
	}
	if deleted != conv.SourceHash {
		t.Errorf("OnDeleteConv fired with %q, want the selected untrusted conversation %q", deleted, conv.SourceHash)
	}
}
