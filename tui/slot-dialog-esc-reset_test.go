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

// Live fleet bug (local tmux session, 2026-09-01): dismissing a slot-placed
// dialog with Esc closed the overlay but left cd.dialogOpen stuck true — the
// shortcut bar blanked permanently and EVERY list shortcut (C-e Peer Info,
// C-n New, C-r Sync, …) died until the process was restarted.
//
// Root cause: tview dispatches a key from the root DOWN THROUGH the
// DialogLineBox (an ancestor of the focused dialog item) before the item's own
// handler, and ShowListSlotDialog overwrote dialog.onDismiss with the bare
// CloseListSlotDialog — discarding the constructor's dismiss closure, which is
// the only thing that resets cd.dialogOpen.

// TestSlotDialogEscResetsDialogOpen drives Esc through the Message Sync
// dialog's DialogLineBox — the exact dispatch path tview uses — and requires
// the dialog-open state to reset, the overlay to close, the sync-dismiss hook
// to fire, and the shortcut bar to stay alive.
func TestSlotDialogEscResetsDialogOpen(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	var dismissActions []string
	openSync := func() {
		cd.ShowSyncDialog("", nil, SyncDialogHooks{}, func(result SyncDialogResult) {
			dismissActions = append(dismissActions, result.Action)
		})
	}
	// The wiring layer installs OnSync = ShowSyncDialog; replicate it so the
	// page-level C-r shortcut has the same effect as in production.
	cd.OnSync = openSync
	openSync()
	if !cd.dialogOpen {
		t.Fatalf("ShowSyncDialog did not set dialogOpen")
	}
	if cd.listSlotOverlay == nil {
		t.Fatalf("ShowSyncDialog did not install the list slot overlay")
	}

	// tview dispatches Esc from the root down THROUGH the DialogLineBox before
	// any inner nav item, so the dialog's own onDismiss must run here.
	dialog := cd.listSlotOverlay.Dialog()
	if dialog == nil {
		t.Fatalf("overlay has no DialogLineBox")
	}
	handler := dialog.InputHandler()
	if handler == nil {
		t.Fatalf("DialogLineBox has no InputHandler")
	}
	handler(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), func(p tview.Primitive) {
		if cd.app != nil {
			cd.app.SetFocus(p)
		}
	})

	if cd.dialogOpen {
		t.Errorf("Esc through the dialog left dialogOpen stuck true: the shortcut bar stays blank and every list shortcut stays dead")
	}
	if cd.listSlotOverlay != nil {
		t.Errorf("Esc through the dialog left the list slot overlay installed")
	}
	if len(dismissActions) != 1 || dismissActions[0] != "dismiss" {
		t.Errorf("sync dismiss hook actions = %v, want [dismiss]", dismissActions)
	}

	// The shortcut bar must keep advertising the list options after the dialog
	// closes (Python's shortcuts() never returns an empty bar).
	if got := cd.GetShortcutText(); got != shortcutConversationsList {
		t.Errorf("shortcut bar after Esc-dismissed sync dialog = %q, want %q", got, shortcutConversationsList)
	}

	// List shortcuts must work again: Ctrl-R re-opens the sync dialog.
	if !cd.fireKey(t, tcell.KeyCtrlR) {
		t.Errorf("C-r dead after Esc-dismissed sync dialog (dialog-open guard stuck on)")
	}
	if cd.listSlotOverlay == nil {
		t.Errorf("C-r did not re-open the sync dialog")
	}
}

// TestShowListSlotDialogKeepsDialogDismiss pins the mechanism: ShowListSlotDialog
// must NOT overwrite a dialog's own onDismiss with the bare
// CloseListSlotDialog, because the Esc key is dispatched THROUGH the
// DialogLineBox before it reaches any inner nav item.
func TestShowListSlotDialogKeepsDialogDismiss(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	called := false
	content := tview.NewTextView().SetText("")
	dialog := NewDialogLineBox("T", content, func() { called = true })

	cd.ShowListSlotDialog(dialog, 100, 0, 4)
	dialog.InputHandler()(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone), func(p tview.Primitive) {
		if cd.app != nil {
			cd.app.SetFocus(p)
		}
	})

	if !called {
		t.Errorf("ShowListSlotDialog discarded the dialog's own onDismiss: the constructor dismiss never ran")
	}
}
