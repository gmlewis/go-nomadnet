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

// The "Ctrl-commands usable throughout" bug (owner report, 2026-09-04): the
// conversation-level Ctrl shortcuts must stay reachable while the COMPOSER
// (an input widget) holds focus — the user should not have to hit Up/Tab
// first. Python reaches this through urwid's bottom-up propagation
// (MessageEdit.keypress consumes ctrl d/p/f/s and bubbles the rest to
// ConversationWidget.keypress, Conversations.py:1807-1825 + 2218-2241);
// tview runs captures top-down, so the widget's frame capture must yield the
// composer's editing keys while keeping the widget-level shortcuts live.
//
// Ctrl-w is the one key Python does NOT bubble from the composer: the
// ReadlineMixin consumes it as word-kill (ReadlineEdit.py:40,59), so the
// widget-level C-w Close stays reachable only after Up/Tab — verified live
// against the installed Python (word-kill, conversation stays open). These
// tests pin that parity: the composer's ctrl-w word-kills, and the bubbling
// keys (ctrl-x clear history, ctrl-t) stay live while typing.

// composerKey dispatches one key through the conversation frame's real input
// chain with the composer focused, then redraws nothing (state assertions
// read the model directly).
func composerKey(t *testing.T, cw *ConversationWidget, key tcell.Key, mod tcell.ModMask) {
	t.Helper()
	if !cw.composerHasFocus() {
		t.Fatalf("precondition: the composer does not hold focus")
	}
	cw.frame.InputHandler()(tcell.NewEventKey(key, 0, mod), func(p tview.Primitive) {
		cw.app.SetFocus(p)
	})
}

// TestComposerCtrlWWordKill pins the Python parity for ctrl-w from the
// composer: the ReadlineMixin consumes it as unix-word-rubout
// (ReadlineEdit.py:40,59), so it word-kills in the editor and the
// widget-level C-w Close (Conversations.py:2225-2226) does NOT fire —
// verified live against the installed Python (word-kill, conversation stays
// open). Close remains reachable from the message body (the widget-level
// path) after Up/Tab.
func TestComposerCtrlWWordKill(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	closed := false
	cw.OnClose = func() { closed = true }

	cw.editor.SetText("draft text")
	cw.editor.SetCursorPos(len("draft text"))
	app.SetFocus(cw.editor)

	composerKey(t, cw, tcell.KeyCtrlW, tcell.ModNone)
	if closed {
		t.Fatal("ctrl-w from the composer closed the conversation; Python's readline word-kill must stay (ReadlineEdit.py:40)")
	}
	if got := cw.editor.GetText(); got != "draft " {
		t.Errorf("ctrl-w from the composer left text %q, want the word-killed %q", got, "draft ")
	}
}

// TestComposerCtrlXClearHistory pins the Python-parity bubbling: ctrl-x from
// the composer opens the clear-history flow (Conversations.py:2232 via the
// ReadlineMixin not consuming ctrl-x), and ctrl-t toggles the editor
// (Conversations.py:2229).
func TestComposerCtrlXClearHistory(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cleared := false
	cw.OnClearHistory = func() { cleared = true }

	app.SetFocus(cw.editor)

	composerKey(t, cw, tcell.KeyCtrlX, tcell.ModNone)
	if !cleared {
		t.Fatal("ctrl-x from the composer did not open the clear-history flow")
	}

	// ctrl-t toggles the full editor (title row appears) with the composer
	// focused.
	before := cw.fullEditorActive
	composerKey(t, cw, tcell.KeyCtrlT, tcell.ModNone)
	if cw.fullEditorActive == before {
		t.Fatalf("ctrl-t from the composer did not toggle the editor (fullEditorActive still %v)", cw.fullEditorActive)
	}
}

// TestComposerReadlineKeysUnchanged pins the composer's readline editing
// keys: ctrl-u still kills to the beginning of the line (NOT the widget-level
// purge) and ctrl-a still moves to bol, exactly like Python's ReadlineMixin
// (ReadlineEdit.py:55-57).
func TestComposerReadlineKeysUnchanged(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	purged := false
	cw.OnPurgeFailed = func() { purged = true }

	cw.editor.SetText("one two three")
	app.SetFocus(cw.editor)

	composerKey(t, cw, tcell.KeyCtrlU, tcell.ModNone)
	if purged {
		t.Fatal("ctrl-u from the composer fired the widget-level purge; the readline kill-to-beg must stay")
	}
	if got := cw.editor.GetText(); got != "" {
		t.Errorf("ctrl-u from the composer left text %q, want the kill-to-beg empty buffer", got)
	}

	// ctrl-a moves the model cursor to bol (still an editing key, not the
	// widget-level attach).
	cw.editor.SetText("abc")
	cw.editor.SetCursorPos(2)
	composerKey(t, cw, tcell.KeyCtrlA, tcell.ModNone)
	if got := cw.editor.CursorPos(); got != 0 {
		t.Errorf("ctrl-a from the composer cursor = %v, want 0 (bol)", got)
	}
}
