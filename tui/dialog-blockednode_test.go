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
)

// TestShowBlockedNodeConfirmDialog pins the Go-only "blocked node" connect
// warning modal: buttons are [Cancel, Connect] with Cancel first, so the
// initial (default) focus is Cancel — a bare Enter CANCELS. Tab reaches
// Connect, whose Enter proceeds. Both paths dismiss the dialog.
func TestShowBlockedNodeConfirmDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)

	var connected, cancelled int
	app.Dialogs.ShowBlockedNodeConfirmDialog(
		"Are you sure you want to connect to this blocked node?\nBad Node",
		func() { connected++ },
		func() { cancelled++ },
	)

	if got := app.Dialogs.Count(); got != 1 {
		t.Fatalf("dialog count = %v, want 1", got)
	}
	if got := app.GetFocus(); !isButton(got, "Cancel") {
		t.Fatalf("initial focus = %T, want the default Cancel button", got)
	}

	pressKeyThroughFocus(app, tcell.KeyEnter)
	if connected != 0 || cancelled != 1 {
		t.Fatalf("after Enter on the default: connected=%v cancelled=%v, want 0/1", connected, cancelled)
	}
	if got := app.Dialogs.Count(); got != 0 {
		t.Fatalf("dialog count after cancel = %v, want 0", got)
	}

	// Tab moves to Connect; Enter confirms, dismissing the dialog.
	app.Dialogs.ShowBlockedNodeConfirmDialog("msg\ndisplay", func() { connected++ }, func() { cancelled++ })
	if got := pressKeyThroughFocus(app, tcell.KeyTab); !isButton(got, "Connect") {
		t.Fatalf("focus after Tab = %T, want the Connect button", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if connected != 1 || cancelled != 1 {
		t.Fatalf("after Tab+Enter: connected=%v cancelled=%v, want 1/1", connected, cancelled)
	}
	if got := app.Dialogs.Count(); got != 0 {
		t.Fatalf("dialog count after connect = %v, want 0", got)
	}
}

// TestShowBlockedNodeConfirmDialogEscCancels pins that Esc dismisses the
// warning modal through the cancel path (DialogLineBox default Esc handling
// plus wireDialogNav's Escape → dismiss).
func TestShowBlockedNodeConfirmDialogEscCancels(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	var connected, cancelled int
	app.Dialogs.ShowBlockedNodeConfirmDialog("msg\ndisplay", func() { connected++ }, func() { cancelled++ })

	// wireDialogNav's Escape handler dismisses through the cancel callback.
	if got := app.GetFocus(); !isButton(got, "Cancel") {
		t.Fatalf("initial focus = %T, want Cancel", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEscape)
	if connected != 0 || cancelled != 1 {
		t.Fatalf("after Esc: connected=%v cancelled=%v, want 0/1", connected, cancelled)
	}
	if got := app.Dialogs.Count(); got != 0 {
		t.Fatalf("dialog count after Esc = %v, want 0", got)
	}
}
