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
)

// TestConversationsShortcutBarTracksLiveFocus pins Python
// Conversations.py:1765-1779: shortcuts() reads the LIVE focus path. A
// stale shortcutFocus must not suppress the editor bar while the composer
// has focus.
func TestConversationsShortcutBarTracksLiveFocus(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	convs := []ConversationInfo{
		{SourceHash: "aabbccddeeff0011", DisplayName: "Alice", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }
	cd.DisplayConversation("aabbccddeeff0011")
	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("currentWidget not set")
	}

	app.SetFocus(cw.editor)
	wantEditor := "[C-d] Send  [C-p] Paper Msg  [C-t] Title  [C-f] Attach  [C-s] Save  [Tab] ↑ Messages"
	if got := cd.GetShortcutText(); got != wantEditor {
		t.Errorf("editor focused: bar = %q, want %q", got, wantEditor)
	}
	cd.shortcutFocus = "list"
	if got := cd.GetShortcutText(); got != wantEditor {
		t.Errorf("stale list cache with editor focused: bar = %q, want %q", got, wantEditor)
	}

	app.SetFocus(cw.messageList)
	wantBody := "[C-s] Save  [C-u] Purge  [C-o] Sort  [C-x] Clear History  [C-g] Fullscreen  [C-w] Close  [Tab] ↓ Editor"
	if got := cd.GetShortcutText(); got != wantBody {
		t.Errorf("body focused: bar = %q, want %q", got, wantBody)
	}
	cd.shortcutFocus = "editor"
	if got := cd.GetShortcutText(); got != wantBody {
		t.Errorf("stale editor cache with body focused: bar = %q, want %q", got, wantBody)
	}
}
