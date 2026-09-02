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

// Regression tests for fleet bug #14 (the glenn-mac-mini-m2 session: Ctrl-p →
// Esc → dead arrows, unable to exit the message window): the dialog-open
// state can desync from the actual focus — a dialog left logically open but
// invisible/orphaned (directly observed live during the bug #13 forensics:
// focus sat inside its OK row while the screen showed the plain page). Keys
// then dispatch into the invisible dialog (or nowhere), and every page
// shortcut and arrow appears dead until the process is restarted. The
// dispatcher now detects the orphaned state and heals it on the next key.

// TestOrphanedOverlayStateHealedOnKey pins the heal: with dialogOpen stuck
// true and NO dialog owning the focus, the next key recovers the state and is
// processed normally (Ctrl-N opens the New Conversation dialog again).
func TestOrphanedOverlayStateHealedOnKey(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	// Simulate the corrupted state: the flag stuck, no overlay installed,
	// focus on the page's list (the live symptom — a stale invisible dialog).
	cd.dialogOpen = true
	if !cd.overlayStateOrphaned() {
		t.Fatalf("stuck dialogOpen without any focused overlay must be detected as orphaned")
	}

	// Wire the create callback the way the wiring layer does, so Ctrl-N has
	// a visible effect after the heal.
	cd.OnNewConv = func() {
		cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	}

	// The next key heals the state AND is processed: Ctrl-N opens the New
	// Conversation dialog (the shortcut that was dead in the bug report).
	if !cd.fireKey(t, tcell.KeyCtrlN) {
		t.Fatalf("Ctrl-N still consumed=nothing after the heal — the keyboard would stay dead")
	}
	// dialogOpen is true AGAIN — correctly: the healed Ctrl-N opened a real
	// dialog. The stuck flag was cleared and a live one took its place.
	if cd.listSlotOverlay == nil {
		t.Errorf("Ctrl-N did not open the New Conversation dialog after the heal")
	}
	if !cd.dialogOpen {
		t.Errorf("the New Conversation dialog did not set dialogOpen after the heal")
	}
	// Close it so the shared display is left clean for the footer assertion.
	cd.CloseListSlotDialog()
	if got := cd.GetShortcutText(); got != shortcutConversationsList {
		t.Errorf("shortcut bar after the heal = %q, want the list bar", got)
	}
}

// TestLiveOverlayIsNotOrphaned pins the guard's negative case: a REAL open
// overlay holds focus inside its DialogLineBox, so the state is NOT orphaned
// and the modal behavior is preserved (keys pass through to the dialog).
func TestLiveOverlayIsNotOrphaned(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	cd.ShowSyncDialog("", nil, SyncDialogHooks{}, nil)
	if cd.listSlotOverlay == nil {
		t.Fatalf("sync dialog not shown")
	}
	if cd.overlayStateOrphaned() {
		t.Fatalf("a live focused overlay was flagged orphaned — the modal would be torn down by the next key")
	}

	// A page shortcut while the dialog is open must stay blocked (modal).
	if cd.fireKey(t, tcell.KeyCtrlN) {
		t.Errorf("C-n consumed while the sync dialog is open — modality broken")
	}
	if cd.listSlotOverlay == nil {
		t.Errorf("the sync dialog was dismissed by a page key")
	}
	cd.CloseListSlotDialog()
}

// TestGlobalDialogNotOrphaned pins the global-modal exception: when the
// DialogManager owns the open dialog (confirm/input dialogs), the page's
// dialogOpen flag alongside it is consistent and must not be healed away.
func TestGlobalDialogNotOrphaned(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.dialogOpen = true
	app.Dialogs.ShowConfirmDialog("Delete conversation with <peer>?", nil, nil)
	if !app.Dialogs.Open() {
		t.Fatalf("global confirm dialog not open")
	}
	if cd.overlayStateOrphaned() {
		t.Errorf("state flagged orphaned while a global modal owns the keyboard — the guard would fight the global dialog")
	}
	app.Dialogs.DismissTop()
}

// TestRecoverOrphanedOverlaysClearsAllSlots pins the recovery: every slot
// overlay pointer is closed, the dialog-open flags reset, and the shortcut
// bar re-syncs to the focused pane.
func TestRecoverOrphanedOverlaysClearsAllSlots(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	// Orphaned flags with a stale overlay installed and a stale footer
	// region — the corrupted state from the bug report.
	cd.dialogOpen = true
	cd.shortcutFocus = "editor"
	cd.ShowSyncDialog("", nil, SyncDialogHooks{}, nil)
	cd.dialogOpen = true // ShowSyncDialog set it; re-mark to model the stuck flag

	// Strip focus from everything so the state reads as orphaned.
	if focused := app.GetFocus(); focused != nil {
		app.SetFocus(tview.NewBox())
	}
	if !cd.overlayStateOrphaned() {
		t.Fatalf("expected the orphaned state to be detected")
	}

	cd.recoverOrphanedOverlays()
	if cd.listSlotOverlay != nil || cd.fullSlotOverlay != nil || cd.detailSlotOverlay != nil {
		t.Errorf("slot overlays still installed after the recovery")
	}
	if cd.dialogOpen {
		t.Errorf("dialogOpen still true after the recovery")
	}
}
