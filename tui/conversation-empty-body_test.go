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

	"github.com/gdamore/tcell/v2"
)

// renderConversationRows draws cd.content on a 100x30 simulation screen and
// returns the joined cell text per row (the headless-render idiom used by
// conversations-twopane_test.go). 100 columns keeps the right pane wide enough
// for the untruncated peer-info header line.
func renderConversationRows(t *testing.T, cd *ConversationsDisplay) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 30)
	cd.content.SetRect(0, 0, 100, 30)
	cd.content.Draw(screen)
	screen.Sync()

	rows := make([]string, 30)
	for y := range 30 {
		var b strings.Builder
		for x := range 100 {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// TestEmptyConversationRendersBodyAndComposer pins the Python ConversationWidget
// frame layout for a conversation with ZERO messages (Conversations.py:1874-
// 1902): the frame is ALWAYS built as header | message list | footer editor,
// with the editor focused, regardless of message count —
// update_message_widgets (Conversations.py:2254-2304) leaves an empty
// IndicativeListBox as the body, never removes the body or the footer. A fresh
// conversation created via the New Conversation dialog (and any other
// zero-message conversation) must therefore render a peer-info header line, a
// message-list body area with nonzero height, and a composer editor with
// nonzero width that echoes typed text — never a bare header line with nothing
// below it.
func TestEmptyConversationRendersBodyAndComposer(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.OnLoadMessages = func(string) []ConversationMessage { return nil }
	const hash = "aabb1122aabb1122aabb1122aabb1122"
	cd.DisplayConversation(hash)
	cd.focusEditor()

	cw := cd.currentWidget
	if cw == nil {
		t.Fatal("no conversation widget after DisplayConversation")
	}

	// Structural: the frame's body (message list) and footer (composer editor)
	// are laid out with nonzero rects inside the pane. tview assigns rects
	// during Draw, so render once first.
	cw.editor.SetText("ping parity")
	rows := renderConversationRows(t, cd)

	_, _, w, _ := cd.content.GetInnerRect()
	mx, my, mw, mh := cw.messageList.GetInnerRect()
	if mw <= 0 || mh <= 0 {
		t.Fatalf("message list body rect = (%v,%v %vx%v), want nonzero size", mx, my, mw, mh)
	}
	ex, ey, ew, _ := cw.editor.GetInnerRect()
	if ew <= 0 {
		t.Fatalf("composer editor rect = (%v,%v %vx?), want nonzero width", ex, ey, ew)
	}
	if ey < my+mh {
		t.Errorf("composer row y=%v is not below the message-list body (body spans y=%v..%v)", ey, my, my+mh-1)
	}

	// Rendered: the peer-info header line shows, and the composer echoes text.
	if !anyRowContains(rows, hash) {
		t.Errorf("peer-info header line for %v not rendered", hash)
	}
	echo := false
	for _, r := range rows {
		if strings.Contains(r, "ping parity") {
			echo = true
		}
	}
	if !echo {
		t.Errorf("typed text does not echo in the composer row; rows:\n%v", strings.Join(rows, "\n"))
	}
	if w == 0 {
		t.Error("content pane has zero width")
	}
}
