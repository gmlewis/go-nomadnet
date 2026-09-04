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
	"github.com/rivo/tview"
)

// TestRoomEditorGrowsPanelParity pins the fleet-reported parity behavior:
// as the user keeps typing, Python's room composer (urwid Edit multiline=True
// in the RoomFrame footer, Channels.py:605/615) grows by one row per wrapped
// line and the message list shrinks to make room — the whole panel content
// "moves up". gonomadnet previously fixed the composer at one row, clipped
// the text at the right border, and walked the caret past the panel.
func TestRoomEditorGrowsPanelParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "RaspPi Local Hub", "test3")
	rw.editor.SetText(c4Message)
	rw.editor.SetCursorPos(len([]rune(c4Message)))
	rw.editor.Focus(func(tview.Primitive) {})

	// 98-wide room pane = the live fleet geometry (156-col terminal − 36 list
	// − 22 users), so the editor's inner width is 96 and the draft wraps to
	// three rows exactly like the Python capture.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(98, 20)
	rw.chatBox.SetRect(0, 0, 98, 20)
	rw.chatBox.Draw(screen)

	// The composer grew to three rows.
	_, _, _, editorH := rw.editor.GetRect()
	if editorH != 3 {
		t.Fatalf("composer height = %v, want 3 wrapped rows (Python grows the footer)", editorH)
	}
	// The message list shrank by the same amount (header 1 + messages + 3
	// composer rows = 18 inner rows).
	_, _, _, msgsH := rw.messagesArea.GetRect()
	if msgsH != 14 {
		t.Errorf("message area height = %v, want 14 (panel moved up for the composer)", msgsH)
	}

	// The wrapped composer rows render at the bottom of the panel.
	wantRow0 := "Message C4 from glenn-mac-mini-m2 again, but this time typing way beyond the length of the input"
	var got strings.Builder
	for x := 0; x < 96; x++ {
		ch, _, _ := screen.Get(x+1, 16)
		got.WriteString(ch)
	}
	if line := strings.TrimRight(got.String(), " "); line != wantRow0 {
		t.Errorf("composer row 0 = %q, want %q", line, wantRow0)
	}

	// The caret stays inside the composer, on its last row.
	cx, cy, vis := screen.GetCursor()
	if !vis {
		t.Fatal("composer caret is not visible")
	}
	ex, ey, ew, eh := rw.editor.GetRect()
	if cx < ex || cx >= ex+ew || cy < ey || cy >= ey+eh {
		t.Errorf("caret (%v,%v) outside the composer rect (%v,%v,%v,%v)",
			cx, cy, ex, ey, ew, eh)
	}
	if cx != 58 || cy != 18 {
		t.Errorf("caret = (%v,%v), want (58,18) (panel origin + 57 + border)", cx, cy)
	}
}

// TestRoomEditorShrinksAgainParity checks the reverse: sending/clearing the
// draft returns the composer to a single row and gives the space back to the
// message list.
func TestRoomEditorShrinksAgainParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "RaspPi Local Hub", "test3")
	rw.editor.SetText(c4Message)
	rw.editor.SetCursorPos(len([]rune(c4Message)))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(98, 20)
	rw.chatBox.SetRect(0, 0, 98, 20)
	rw.chatBox.Draw(screen)
	_, _, _, grown := rw.editor.GetRect()
	if grown != 3 {
		t.Fatalf("composer height = %v, want 3", grown)
	}

	// Clearing the draft (as sendMessage does) re-collapses on the next draw.
	rw.editor.SetText("")
	rw.chatBox.Draw(screen)
	_, _, _, shrunk := rw.editor.GetRect()
	if shrunk != 1 {
		t.Errorf("composer height after clear = %v, want 1", shrunk)
	}
	_, _, _, msgsH := rw.messagesArea.GetRect()
	if msgsH != 16 {
		t.Errorf("message area height after clear = %v, want 16", msgsH)
	}
}

// TestRoomEditorHeightCappedParity pins the squeezed edge: when the draft
// grows taller than the panel, the composer is capped so the header and at
// least one message row stay visible (urwid's Frame renders the same shape
// live: one body row + a clipped footer, no crash).
func TestRoomEditorHeightCappedParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "room")
	many := strings.Repeat("word wrap line padding the buffer beyond the panel\n", 20)
	rw.editor.SetText(many)
	rw.editor.SetCursorPos(len([]rune(many)))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(98, 10)
	rw.chatBox.SetRect(0, 0, 98, 10)
	rw.chatBox.Draw(screen)

	_, _, _, editorH := rw.editor.GetRect()
	if editorH != 6 {
		t.Errorf("composer height = %v, want 6 (capped at inner height − header − 1 message row)", editorH)
	}
	_, _, _, msgsH := rw.messagesArea.GetRect()
	if msgsH != 1 {
		t.Errorf("message area height = %v, want 1 (urwid keeps one body row)", msgsH)
	}
}
