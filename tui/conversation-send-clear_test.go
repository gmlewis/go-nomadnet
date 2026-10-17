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

import "testing"

// A send can fail without sending anything: when the peer's identity has not
// been recalled, no LXMF message can be built at all (LXMF encrypts to the
// recipient's identity), so App.SendConversation reports failure and nothing
// leaves the node — not directly, and not through a propagation node either.
//
// Python clears the composer only on success (Conversations.py:2412-2436):
//
//	if self.conversation.send(content, title, fields=fields):
//	    self.clear_editor()
//
// so a failed send leaves the draft where the reader can see it and try again.
// Clearing it regardless is silent data loss: the message disappears from the
// screen with nothing sent, nothing stored, and no error shown — the shape the
// operator hit when a QR-scanned peer had no identity yet.
func TestComposerKeepsDraftWhenSendFails(t *testing.T) {
	t.Parallel()

	cw := newComposerWidget(t)
	cw.OnSend = func(string, string, []string) bool { return false }

	cw.editor.SetText("hello raspberrypi")
	cw.sendMessage()

	if got := cw.editor.GetText(); got != "hello raspberrypi" {
		t.Errorf("editor text after a failed send = %q, want the draft kept "+
			"(Python clears the editor only when send() returns True)", got)
	}
}

// TestComposerKeepsStagedAttachmentsWhenSendFails covers the other half of
// Python's clear_editor (Conversations.py:2294-2298), which drops
// pending_attachments along with the text. A failed send must not discard them:
// they are still staged for the retry.
func TestComposerKeepsStagedAttachmentsWhenSendFails(t *testing.T) {
	t.Parallel()

	cw := newComposerWidget(t)
	cw.OnSend = func(string, string, []string) bool { return false }

	cw.editor.SetText("see attached")
	cw.pendingAttachments = []string{"/tmp/report.pdf"}
	cw.sendMessage()

	if len(cw.pendingAttachments) != 1 || cw.pendingAttachments[0] != "/tmp/report.pdf" {
		t.Errorf("pendingAttachments after a failed send = %v, want the staged attachment kept", cw.pendingAttachments)
	}
}

// TestComposerClearsDraftWhenSendSucceeds is the positive half: a send the
// router accepted empties the composer, exactly as Python's clear_editor does.
func TestComposerClearsDraftWhenSendSucceeds(t *testing.T) {
	t.Parallel()

	cw := newComposerWidget(t)
	var got string
	cw.OnSend = func(content, _ string, _ []string) bool {
		got = content
		return true
	}

	cw.editor.SetText("hello raspberrypi")
	cw.pendingAttachments = []string{"/tmp/report.pdf"}
	cw.sendMessage()

	if got != "hello raspberrypi" {
		t.Errorf("OnSend content = %q, want %q", got, "hello raspberrypi")
	}
	if text := cw.editor.GetText(); text != "" {
		t.Errorf("editor text after a successful send = %q, want empty", text)
	}
	if cw.pendingAttachments != nil {
		t.Errorf("pendingAttachments after a successful send = %v, want none", cw.pendingAttachments)
	}
}

// newComposerWidget returns a standalone conversation widget with a wired App,
// for the send/clear tests.
func newComposerWidget(t *testing.T) *ConversationWidget {
	t.Helper()
	cw := NewConversationWidget(newTestApp(), "aabb1122")
	cw.ClearEditor()
	return cw
}
