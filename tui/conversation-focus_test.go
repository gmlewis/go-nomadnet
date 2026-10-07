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
	"testing"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// newEmptyConversationDisplay returns a ConversationsDisplay whose message
// loader yields no messages, so DisplayConversation can open a conversation
// without any wiring (the headless idiom of conversation-empty-body_test.go).
func newEmptyConversationDisplay(t *testing.T) *ConversationsDisplay {
	t.Helper()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }
	return cd
}

// TestCreateClearsOpenConversation pins the state the New Conversation dialog's
// Create leaves behind, verified live against nomadnet 1.2.8: Python's
// confirmed() calls display_conversation(source_hash_text) POSITIONALLY
// (Conversations.py:1076), so the hash binds to the `sender` parameter and
// source_hash stays None — display_conversation then does
// columns_widget.contents[1] = ConversationWidget(None) (the "\n  No
// conversation selected" placeholder, Conversations.py:1892) and
// columns_widget.focus_position = 0 (Conversations.py:1649-1650). The observed
// 1.2.8 behavior is therefore: after Create the right pane reads "No
// conversation selected" and the LIST holds focus — even when a conversation
// was already open before the dialog was opened. Keeping the old conversation
// displayed (and focusing its composer) would leave the user typing into the
// previous peer after creating a new one.
func TestCreateClearsOpenConversation(t *testing.T) {
	t.Parallel()
	cd := newEmptyConversationDisplay(t)
	app := cd.app
	const hashA = "aabb1122aabb1122aabb1122aabb1122"
	cd.DisplayConversation(hashA)

	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("no conversation widget after DisplayConversation")
	}
	if got := app.GetFocus(); got != tview.Primitive(cw.editor) {
		t.Fatalf("after opening a conversation focus = %T, want the composer editor", got)
	}

	// Open the dialog from the list region and accept Create (the dialog's
	// items are Addr, Name, Untrusted, Unknown, Trusted, Create, Back, so five
	// Tabs land on Create).
	cd.setShortcutRegion("list")
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	for range 5 {
		fireDialogKey(t, app, tcell.KeyTab)
	}
	fireDialogKey(t, app, tcell.KeyEnter)

	if cd.currentWidget != nil {
		t.Errorf("after Create the previously open conversation is still displayed (currentWidget != nil); Python replaces the detail pane with the empty placeholder")
	}
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Errorf("after Create focus = %T, want the conversation list (Python columns_widget.focus_position = 0)", got)
	}
	if cd.shortcutFocus != "list" {
		t.Errorf("after Create shortcut region = %q, want %q", cd.shortcutFocus, "list")
	}
}

// TestReopenConversationFocusesComposer pins that re-opening a conversation
// always lands focus on its composer, including after a C-w close. The close
// path (ConversationWidget OnClose) clears currentWidget, rebuilds the detail
// pane to the empty placeholder and hands focus to the list; the next
// DisplayConversation must restore the composer focus so keystrokes reach the
// editor instead of dying on the list.
func TestReopenConversationFocusesComposer(t *testing.T) {
	t.Parallel()
	cd := newEmptyConversationDisplay(t)
	app := cd.app
	const hashA = "aabb1122aabb1122aabb1122aabb1122"
	const hashB = "ccdd3344ccdd3344ccdd3344ccdd3344"

	cd.DisplayConversation(hashA)
	if cd.currentWidget == nil || cd.currentWidget.OnClose == nil {
		t.Fatal("DisplayConversation did not wire OnClose")
	}
	cd.currentWidget.OnClose()
	if cd.currentWidget != nil {
		t.Fatal("OnClose left currentWidget set")
	}
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Fatalf("after OnClose focus = %T, want the conversation list", got)
	}

	cd.DisplayConversation(hashB)
	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("no conversation widget after re-opening")
	}
	if got := app.GetFocus(); got != tview.Primitive(cw.editor) {
		t.Errorf("after re-opening focus = %T, want the composer editor", got)
	}
	if cd.shortcutFocus != "editor" {
		t.Errorf("after re-opening shortcut region = %q, want %q", cd.shortcutFocus, "editor")
	}
}

// TestTabTogglesComposerAndBodyRegion pins ConversationWidget's "tab" handling
// from BOTH focus regions, matching Python's ConversationWidget.keypress
// (Conversations.py:2233-2236) → toggle_focus_area (Conversations.py:2216-2231).
//
// Python consumes Tab in the widget keypress ahead of super().keypress(), so it
// applies whatever the focused part is; it reaches that branch from the
// composer too because MessageEdit.keypress (Conversations.py:1819-1831)
// consumes only ctrl d/p/f/s and its special "up" and returns every other key —
// "tab" included — to the widget above it. Go's frame capture runs top-down, so
// handleInput must decide Tab before it splits between the composer and widget
// paths; deciding it in the composer path only would leave Tab dead while the
// composer had focus, so focus could never come back out of the editor and the
// "[Tab] ↓ Editor" shortcut-bar hint would be a lie.
func TestTabTogglesComposerAndBodyRegion(t *testing.T) {
	t.Parallel()
	cd := newEmptyConversationDisplay(t)
	app := cd.app
	const hashA = "aabb1122aabb1122aabb1122aabb1122"
	cd.DisplayConversation(hashA)
	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("no conversation widget after DisplayConversation")
	}

	// The app dispatches a key through the page capture first and the
	// conversation widget's frame capture second (MainDisplay.handleInput), so
	// the test drives the same order rather than calling the widget directly.
	press := func(key tcell.Key) {
		event := tcell.NewEventKey(key, 0, tcell.ModNone)
		if event = cd.handleInput(event); event != nil {
			cw.handleInput(event)
		}
	}

	if got := app.GetFocus(); got != tview.Primitive(cw.editor) {
		t.Fatalf("after opening a conversation focus = %T, want the composer editor", got)
	}

	press(tcell.KeyTab)
	if got := app.GetFocus(); got != tview.Primitive(cw.messageList) {
		t.Errorf("Tab from the composer left focus on %T, want the message body", got)
	}
	if got := cd.GetShortcutText(); !strings.Contains(got, "[Tab] ↓ Editor") {
		t.Errorf("Tab from the composer: shortcut bar = %q, want the body bar containing %q", got, "[Tab] ↓ Editor")
	}

	press(tcell.KeyTab)
	if got := app.GetFocus(); got != tview.Primitive(cw.editor) {
		t.Errorf("Tab from the body left focus on %T, want the composer editor", got)
	}
	if got := cd.GetShortcutText(); !strings.Contains(got, "[Tab] ↑ Messages") {
		t.Errorf("Tab from the body: shortcut bar = %q, want the composer bar containing %q", got, "[Tab] ↑ Messages")
	}
}
