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

// While a dialog overlay is open, the page's list-region shortcuts must NOT
// fire: Python's overlay replaces the page widget, so C-n under the QR
// dialog never reached the page. In the Go port the page input capture stays
// in the ancestor chain, so it must self-silence while ANY dialog overlay
// (full-slot, list-slot, or right-pane) is up — otherwise C-n stacked the
// New Conversation dialog on top of the QR dialog, rendering both at once.

func TestConversationsShortcutsDeadWhileDialogOpen(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)
	openConversation(cd, "<hash-a>")

	// QR dialog open (whole-display overlay): C-n must pass through to the
	// overlay (unconsumed), NOT open the New Conversation dialog over it.
	cd.ShowMyQRDialog("<my-lxmf-addr>")
	if cd.fullSlotOverlay == nil {
		t.Fatalf("QR dialog not open")
	}

	before := cd.listSlotOverlay
	consumed := cd.fireKey(t, tcell.KeyCtrlN)
	if consumed {
		t.Errorf("C-n consumed while QR dialog open: list shortcut fired under a dialog")
	}
	if cd.listSlotOverlay != before {
		t.Errorf("C-n changed the dialog state while the QR dialog was open")
	}
	if cd.fullSlotOverlay == nil {
		t.Errorf("QR dialog dismissed by C-n")
	}
}

func TestConversationsShortcutsDeadWhileListSlotDialogOpen(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	// New Conversation dialog over the list slot: C-n must not recurse into
	// another New Conversation dialog.
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	ov := cd.listSlotOverlay
	consumed := cd.fireKey(t, tcell.KeyCtrlN)
	if consumed {
		t.Errorf("C-n consumed while list slot dialog open")
	}
	if cd.listSlotOverlay != ov {
		t.Errorf("dialog state changed by a page shortcut while the dialog was open")
	}

	// Same for the other dual-meaning keys.
	for _, key := range []tcell.Key{tcell.KeyCtrlP, tcell.KeyCtrlG, tcell.KeyCtrlO, tcell.KeyCtrlU} {
		consumed = cd.fireKey(t, key)
		if consumed {
			t.Errorf("key %v consumed while list slot dialog open", key)
		}
	}
}

func TestConversationsShortcutsDeadWhileAttachDialogOpen(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)
	openConversation(cd, "<hash-a>")
	cd.currentWidget.OnAttach()
	if cd.detailSlotOverlay == nil {
		t.Fatalf("attach dialog not open")
	}

	// Keys must route to the attach dialog, not to the page's list region or
	// to the conversation's editor/body shortcuts underneath.
	for _, key := range []tcell.Key{tcell.KeyCtrlG, tcell.KeyCtrlP, tcell.KeyCtrlF} {
		if cd.fireKey(t, key) {
			t.Errorf("key %v consumed while attach dialog open", key)
		}
	}
}

func TestConversationsShortcutsLiveAfterDialogsClose(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)
	openConversation(cd, "<hash-a>")

	cd.ShowMyQRDialog("<my-lxmf-addr>")
	cd.CloseFullSlotDialog()
	if cd.fireKey(t, tcell.KeyCtrlN) {
		// C-n opens the New Conversation dialog again (callback fires, the
		// key itself is consumed and handleInput returns nil). After close,
		// the guard must be lifted or NO list shortcut would ever fire.
	} else {
		t.Errorf("C-n dead after all dialogs closed (guard stuck on)")
	}
}

// fireKey runs one key event through the page's input capture and reports
// whether it was consumed (nil returned) — the same dispatch tview performs.
func (cd *ConversationsDisplay) fireKey(t *testing.T, key tcell.Key) bool {
	t.Helper()
	return cd.handleInput(tcell.NewEventKey(key, 0, 0)) == nil
}
