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

// pressKeyThroughFocus sends one key to the currently focused primitive's own
// input handler (the runtime dispatch applies the same capture chain), then
// returns the new focus.
func pressKeyThroughFocus(app *App, key tcell.Key) tview.Primitive {
	focused := app.GetFocus()
	if focused == nil {
		return nil
	}
	if h := focused.InputHandler(); h != nil {
		h(tcell.NewEventKey(key, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	}
	return app.GetFocus()
}

// TestC1IngestURIKeyboardTraversal pins C1 for the Ingest URI dialog: Down
// walks field → Ingest → Back, Enter on Back dismisses (Python's button row
// [Ingest | | Back] with dismiss_dialog, Conversations.py:1251-1256).
func TestC1IngestURIKeyboardTraversal(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.IngestURIDialog(nil)

	if cd.listSlotOverlay == nil {
		t.Fatal("dialog should be open")
	}
	if got := pressKeyThroughFocus(app, tcell.KeyDown); !isButton(got, "Ingest") {
		t.Errorf("focus after Down from the field = %T, want the Ingest button", got)
	}
	if got := pressKeyThroughFocus(app, tcell.KeyDown); !isButton(got, "Back") {
		t.Errorf("focus after Down from Ingest = %T, want the Back button", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if cd.listSlotOverlay != nil {
		t.Error("Enter on Back must dismiss the dialog (dialog still open)")
	}
}

// TestC1SyncDialogKeyboardTraversal pins C1 for the Message Sync dialog: Down
// walks radios → limit → Sync Now → Close, Enter on Close dismisses
// (Conversations.py:1393-1400).
func TestC1SyncDialogKeyboardTraversal(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)
	cd.ShowSyncDialog("", []string{"n1"}, SyncDialogHooks{}, nil)

	want := []string{"Download all", "Limit to", "", "Sync Now", "Close"}
	focus := app.GetFocus()
	for i, wantLabel := range want {
		var got string
		switch b := focus.(type) {
		case *RadioButton:
			got = b.Label()
		case *UrwidButton:
			got = b.Label()
		}
		if got != wantLabel {
			t.Fatalf("focus at step %v = %q (%T), want %q", i, got, focus, wantLabel)
		}
		focus = pressKeyThroughFocus(app, tcell.KeyDown)
	}
	// Focus wrapped back to the first item (urwid Pile wraps Down past the
	// last selectable into the Pile's next, i.e. round to the top).
	if focus == nil {
		t.Fatal("focus lost after walking Down through the whole dialog")
	}
	// Focus Close and activate: the dialog must dismiss.
	app.SetFocus(cd.syncSyncBtn)
	pressKeyThroughFocus(app, tcell.KeyDown) // Sync Now → Close
	if got := app.GetFocus(); !isButton(got, "Close") {
		t.Fatalf("focus after Down from Sync Now = %T, want Close", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if cd.listSlotOverlay != nil {
		t.Error("Enter on Close must dismiss the sync dialog")
	}
}

// TestC1PeerInfoBackButton pins C1 for the Peer Info dialog: Down traversal
// reaches the Back button and Enter on it dismisses (Python's
// dismiss_dialog, Conversations.py:940-944).
func TestC1PeerInfoBackButton(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	entry := PeerInfoEntry{SourceHash: "c10000000000000000000000000000000000", DisplayName: "C1", TrustLevel: "unknown"}
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{}, nil)

	// Walk Down to the LAST item (Back) — the peer info chain is long; walk
	// until the focus stops moving or we hit Back.
	for range 32 {
		before := app.GetFocus()
		if isButton(before, "Back") {
			break
		}
		if after := pressKeyThroughFocus(app, tcell.KeyDown); after == before {
			t.Fatalf("Down traversal stalled on %T before reaching Back", before)
		}
	}
	if !isButton(app.GetFocus(), "Back") {
		t.Fatalf("Down traversal never reached the Back button (focus=%T)", app.GetFocus())
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if cd.listSlotOverlay != nil {
		t.Error("Enter on Back must dismiss the Peer Info dialog")
	}
}

// TestC1NewConversationBackButton pins C1 for the New Conversation dialog:
// Down reaches Back and Enter dismisses without creating anything (Python's
// dismiss_dialog, Conversations.py:1033-1037).
func TestC1NewConversationBackButton(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	created := false
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool {
		created = true
		return true
	})

	for range 16 {
		before := app.GetFocus()
		if isButton(before, "Back") {
			break
		}
		if after := pressKeyThroughFocus(app, tcell.KeyDown); after == before {
			t.Fatalf("Down traversal stalled on %T", before)
		}
	}
	if !isButton(app.GetFocus(), "Back") {
		t.Fatalf("traversal never reached Back (focus=%T)", app.GetFocus())
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if created {
		t.Error("Back must not fire Create")
	}
	if cd.listSlotOverlay != nil {
		t.Error("Enter on Back must dismiss the New Conversation dialog")
	}
}

// TestC1MyQRKeyboardTraversal pins C1 for the My LXMF QR dialog: Down from
// the QR view reaches the Close button; Enter dismisses; Space on the QR view
// dismisses too (B4 owner decision).
func TestC1MyQRKeyboardTraversal(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)
	cd.ShowMyQRDialog("2a6105f57145860441a62fe3b2a1352c")

	if got := pressKeyThroughFocus(app, tcell.KeyDown); !isButton(got, "Close") {
		t.Fatalf("focus after Down from the QR view = %T, want the Close button", got)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if cd.fullSlotOverlay != nil {
		t.Error("Enter on Close must dismiss the QR dialog")
	}

	// Space on the QR view dismisses (B4).
	cd.ShowMyQRDialog("2a6105f57145860441a62fe3b2a1352c")
	focus := app.GetFocus()
	if h := focus.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	}
	if cd.fullSlotOverlay != nil {
		t.Error("Space on the QR view must dismiss the dialog (B4)")
	}
}

// isButton reports whether p is an UrwidButton with the given label.
func isButton(p tview.Primitive, label string) bool {
	b, ok := p.(*UrwidButton)
	return ok && b.Label() == label
}
