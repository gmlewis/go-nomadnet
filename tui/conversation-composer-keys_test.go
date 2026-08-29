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

// When the compose editor has focus, the composer itself consumes its keys
// before any widget-level shortcut: Python dispatches keys bottom-up (the
// focused MessageEdit/ReadlineMixin keypress runs first,
// Conversations.py:1807 + ReadlineEdit.py:54), so with the editor focused
// ctrl a/e/u/k/w/l/y and the arrows are EDITING keys, and only ctrl d/p/f/s
// act on the conversation. urwid's widget-level ctrl a (attach) / ctrl w
// (close) / ctrl u (purge failed) fire only on keys the inner widgets did not
// consume. tview input captures run top-down, so the conversation frame's
// capture must explicitly yield the composer's editing keys when the editor
// is focused — otherwise Backspace/C-a/C-k etc. are stolen and the frame's
// shortcuts fire from inside the composer (attach popping up while typing).

// composerCallbacks holds the wire-up counters shared with a test's closures.
type composerCallbacks struct {
	attaches, closes, purges, sends int
}

func newComposerCW(t *testing.T) (*ConversationWidget, *composerCallbacks) {
	t.Helper()
	cd := newInvariantCD(t)
	openConversation(cd, "<hash-a>")
	cw := cd.currentWidget
	cb := &composerCallbacks{}
	cw.OnAttach = func() { cb.attaches++ }
	cw.OnClose = func() { cb.closes++ }
	cw.OnPurgeFailed = func() { cb.purges++ }
	cw.OnSend = func(content, title string, attachments []string) { cb.sends++ }
	return cw, cb
}

// pressComposerKey simulates tview's dispatch for one key with the editor
// focused: the frame input capture runs first; if it returns the event, the
// focused editor's readline handler receives it. Returns nil when the key was
// consumed at one of those levels.
func (cw *ConversationWidget) pressComposerKey(t *testing.T, key tcell.Key) *tcell.EventKey {
	t.Helper()
	ev := tcell.NewEventKey(key, 0, tcell.ModNone)
	got := cw.handleInput(ev)
	if got == nil {
		return nil
	}
	return cw.editor.handleKey(got)
}

func TestComposerFocusedReadlineKeysEditNotAttach(t *testing.T) {
	t.Parallel()
	cw, cb := newComposerCW(t)
	cw.app.SetFocus(cw.editor)

	// ctrl a → beginning-of-line, NOT attach_file.
	cw.editor.SetText("hello world")
	cw.editor.SetCursorPos(6)
	cw.pressComposerKey(t, tcell.KeyCtrlA)
	if cb.attaches != 0 {
		t.Errorf("C-a with editor focus opened the attach dialog %v times", cb.attaches)
	}
	if got := cw.editor.CursorPos(); got != 0 {
		t.Errorf("C-a did not move the composer cursor to line start (pos=%v)", got)
	}

	// Backspace reaches the editor and deletes backward.
	cw.editor.SetCursorPos(len("hello world"))
	cw.pressComposerKey(t, tcell.KeyBackspace2)
	if got := cw.editor.GetText(); got != "hello worl" {
		t.Errorf("Backspace did not reach the composer editor (text=%q)", got)
	}

	// ctrl w → kill previous word, NOT close.
	cw.editor.SetText("hello world")
	cw.editor.SetCursorPos(len("hello world"))
	cw.pressComposerKey(t, tcell.KeyCtrlW)
	if cb.closes != 0 {
		t.Errorf("C-w with editor focus closed the conversation")
	}
	if got := cw.editor.GetText(); got != "hello " {
		t.Errorf("C-w did not kill the previous word (text=%q)", got)
	}

	// ctrl y → yank the killed word back.
	cw.pressComposerKey(t, tcell.KeyCtrlY)
	if got := cw.editor.GetText(); got != "hello world" {
		t.Errorf("C-y did not yank the killed text (text=%q)", got)
	}
}

func TestComposerFocusedCtrlKCtrlUEditNotPurgeFailed(t *testing.T) {
	t.Parallel()
	cw, cb := newComposerCW(t)
	cw.editor.SetText("hello world")
	cw.editor.SetCursorPos(5)
	cw.app.SetFocus(cw.editor)

	// ctrl k → kill to end of line, NOT purge_failed.
	cw.pressComposerKey(t, tcell.KeyCtrlK)
	if cb.purges != 0 {
		t.Errorf("C-k with editor focus purged failed messages %v times", cb.purges)
	}
	if got := cw.editor.GetText(); got != "hello" {
		t.Errorf("C-k did not kill to end of line (text=%q)", got)
	}

	// ctrl u → kill to beginning of line still edits.
	cw.editor.SetText("rest")
	cw.editor.SetCursorPos(3)
	cw.pressComposerKey(t, tcell.KeyCtrlU)
	if got := cw.editor.GetText(); got != "t" {
		t.Errorf("C-u did not kill to beginning of line (text=%q)", got)
	}
}

func TestComposerFocusedCtrlDSendsNotCloses(t *testing.T) {
	t.Parallel()
	cw, cb := newComposerCW(t)
	cw.editor.SetText("a message")
	cw.app.SetFocus(cw.editor)

	// ctrl d → send_message, and the conversation stays open.
	cw.pressComposerKey(t, tcell.KeyCtrlD)
	if cb.closes != 0 {
		t.Errorf("C-d with editor focus closed the conversation")
	}
	if cb.sends != 1 {
		t.Errorf("C-d did not send the composed message")
	}
	if got := cw.editor.GetText(); got != "" {
		t.Errorf("C-d did not clear the composer (text=%q)", got)
	}
}

func TestComposerFocusedCtrlFAttachesCtrlSGuarded(t *testing.T) {
	t.Parallel()
	cw, cb := newComposerCW(t)
	cw.editor.SetText("typing")
	cw.app.SetFocus(cw.editor)

	// ctrl f → attach_file even while the editor is focused (MessageEdit
	// consumes ctrl f itself); ctrl a stays readline.
	cw.pressComposerKey(t, tcell.KeyCtrlF)
	if cb.attaches != 1 {
		t.Errorf("C-f with editor focus did not open the attach dialog")
	}

	// ctrl s → save focused attachments (MessageEdit consumes ctrl s).
	cw.pressComposerKey(t, tcell.KeyCtrlS)
	if got := cw.editor.GetText(); got != "typing" {
		t.Errorf("C-s altered the composer buffer (text=%q)", got)
	}
}

func TestComposerFocusedWidgetBubblingShortcutsStillWork(t *testing.T) {
	t.Parallel()
	cw, cb := newComposerCW(t)
	cw.editor.SetText("typing")
	cw.app.SetFocus(cw.editor)

	// ctrl g (fullscreen) and ctrl t (toggle title editor) are consumed by
	// neither MessageEdit nor ReadlineMixin, so they bubble past the composer
	// to the widget shortcuts even while typing.
	toggles := 0
	cw.OnToggleFullscreen = func() { toggles++ }
	cw.pressComposerKey(t, tcell.KeyCtrlG)
	if toggles != 1 {
		t.Errorf("C-g with editor focus did not reach the fullscreen toggle")
	}
	full := cw.fullEditorActive
	cw.pressComposerKey(t, tcell.KeyCtrlT)
	if cw.fullEditorActive == full {
		t.Errorf("C-t with editor focus did not toggle the title editor")
	}
	if cb.attaches != 0 {
		t.Errorf("C-g/C-t with editor focus spuriously opened the attach dialog")
	}
}

func TestBodyFocusedCtrlAAttachesAndCtrlWCloses(t *testing.T) {
	t.Parallel()
	cw, cb := newComposerCW(t)
	// Focus the message list: the composer is not in the dispatch path, so the
	// widget-level ctrl a (attach) and ctrl w (close) shortcuts must fire again.
	cw.app.SetFocus(cw.messageList)

	cw.pressComposerKey(t, tcell.KeyCtrlA)
	if cb.attaches != 1 {
		t.Errorf("C-a with body focus did not open the attach dialog")
	}
	cw.pressComposerKey(t, tcell.KeyCtrlW)
	if cb.closes != 1 {
		t.Errorf("C-w with body focus did not close the conversation")
	}
}
