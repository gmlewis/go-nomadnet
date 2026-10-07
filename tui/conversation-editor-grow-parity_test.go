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

// The expected values below were captured from the source-of-truth urwid
// (4.0.3, the build the installed Python nomadnet runs on) by rendering an
// urwid.Edit(caption="", edit_text=c4Message, multiline=True) at the
// conversation composer's inner width (98):
//
//	rows 3, cursor (57, 2), row 0 =
//	"Message C4 from glenn-mac-mini-m2 again, but this time typing way beyond
//	 the length of the input", row 2 ends "...extra lines of input."

// TestConversationEditorGrowsPanelParity pins the fleet-reported behavior for
// the conversation composer: Python builds the message editor as
// MessageEdit(caption="", edit_text="", multiline=True) in the frame footer
// (Conversations.py:1904/1936), so a long draft WRAPS onto as many rows as it
// needs and the frame footer grows with it, pushing the message list up.
// gonomadnet built the editor as a single-line field, so the text ran off the
// right border and everything past it was invisible (the RRC room composer
// already wrapped — this is the same fix for the conversation panel).
func TestConversationEditorGrowsPanelParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	// Trusted peer ⇒ the header is the single peer-info row, so the panel's
	// inner 18 rows split as header 1 + messages + composer.
	cw.TrustLevel = "trusted"
	cw.refreshTrustBanner()
	cw.editor.SetText(c4Message)
	cw.editor.SetCursorPos(len([]rune(c4Message)))
	cw.editor.Focus(func(tview.Primitive) {})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 20)
	cw.frame.SetRect(0, 0, 100, 20)
	cw.frame.Draw(screen)

	// The composer grew to the draft's three wrapped rows.
	_, _, _, editorH := cw.editor.GetRect()
	if editorH != 3 {
		t.Fatalf("composer height = %v, want 3 wrapped rows (Python grows the footer)", editorH)
	}
	// The message list shrank by the same amount (header 1 + messages 14 +
	// composer 3 = 18 inner rows).
	_, _, _, msgsH := cw.messageList.GetRect()
	if msgsH != 14 {
		t.Errorf("message list height = %v, want 14 (panel moved up for the composer)", msgsH)
	}

	// Every wrapped row renders in full — nothing is clipped at the right
	// border (the reported symptom). Rows are compared against the urwid
	// capture above.
	wantRows := []string{
		"Message C4 from glenn-mac-mini-m2 again, but this time typing way beyond the length of the input",
		"line to see how it is being handled, and it turns out that nomadnet will move the whole panel up",
		"one or more lines to accomodate the extra lines of input.",
	}
	ex, ey, ew, _ := cw.editor.GetRect()
	for i, want := range wantRows {
		var got strings.Builder
		for x := range ew {
			ch, _, _ := screen.Get(ex+x, ey+i)
			got.WriteString(ch)
		}
		if line := strings.TrimRight(got.String(), " "); line != want {
			t.Errorf("composer row %v = %q, want %q", i, line, want)
		}
	}

	// The caret sits on the last wrapped row, inside the composer — urwid's
	// cursor for this draft at width 98 is (57, 2).
	cx, cy, vis := screen.GetCursor()
	if !vis {
		t.Fatal("composer caret is not visible")
	}
	if cx != ex+57 || cy != ey+2 {
		t.Errorf("caret = (%v,%v), want (%v,%v) (urwid cursor (57,2) in the composer)",
			cx, cy, ex+57, ey+2)
	}
}

// TestConversationEditorShrinksAgainParity checks the reverse: sending or
// clearing the draft collapses the composer back to one row and gives the
// space back to the message list.
func TestConversationEditorShrinksAgainParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.TrustLevel = "trusted"
	cw.refreshTrustBanner()
	cw.editor.SetText(c4Message)
	cw.editor.SetCursorPos(len([]rune(c4Message)))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 20)
	cw.frame.SetRect(0, 0, 100, 20)
	cw.frame.Draw(screen)
	if _, _, _, grown := cw.editor.GetRect(); grown != 3 {
		t.Fatalf("composer height = %v, want 3", grown)
	}

	cw.ClearEditor()
	cw.frame.Draw(screen)
	if _, _, _, shrunk := cw.editor.GetRect(); shrunk != 1 {
		t.Errorf("composer height after clear = %v, want 1", shrunk)
	}
	if _, _, _, msgsH := cw.messageList.GetRect(); msgsH != 16 {
		t.Errorf("message list height after clear = %v, want 16", msgsH)
	}
}

// TestConversationEditorHeightCappedParity pins the squeezed edge: when the
// draft grows taller than the panel, the composer is capped so the header and
// at least one message row stay visible (urwid's Frame squeezes the body the
// same way for an over-tall footer).
func TestConversationEditorHeightCappedParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.TrustLevel = "trusted"
	cw.refreshTrustBanner()
	many := strings.Repeat("word wrap line padding the buffer beyond the panel\n", 20)
	cw.editor.SetText(many)
	cw.editor.SetCursorPos(len([]rune(many)))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 10)
	cw.frame.SetRect(0, 0, 100, 10)
	cw.frame.Draw(screen)

	_, _, _, editorH := cw.editor.GetRect()
	if editorH != 6 {
		t.Errorf("composer height = %v, want 6 (capped at inner height − header − 1 message row)", editorH)
	}
	_, _, _, msgsH := cw.messageList.GetRect()
	if msgsH != 1 {
		t.Errorf("message list height = %v, want 1 (urwid keeps one body row)", msgsH)
	}
}

// TestConversationEditorUpEscapesAtTopRow pins MessageEdit's "up" focus path
// for the multiline composer (Python Conversations.py:1816-1825): Up moves
// the cursor up a wrapped row while rows remain above it, and only from the
// TOP row does it leave the draft — to the frame body in the minimal editor,
// and to the title editor when the full editor is active (where urwid's Pile
// moves to the previous selectable).
func TestConversationEditorUpEscapesAtTopRow(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	if !cw.editor.multiline {
		t.Fatal("conversation composer is not multiline (Python MessageEdit multiline=True)")
	}
	if cw.editor.OnFocusTopRow == nil {
		t.Fatal("composer has no top-row escape wired")
	}

	cw.editor.SetText(c4Message)
	cw.editor.SetCursorPos(len([]rune(c4Message)))
	cw.editor.Focus(func(tview.Primitive) {})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 20)
	cw.frame.SetRect(0, 0, 100, 20)
	cw.frame.Draw(screen)

	// The wrapped rows: Up inside the draft moves the cursor and stays put.
	rowBefore := cw.editor.CursorRow()
	if rowBefore != 2 {
		t.Fatalf("cursor row = %v, want 2 (last of three wrapped rows)", rowBefore)
	}
	if got := cw.editor.handleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)); got != nil {
		t.Errorf("Up inside the draft was not consumed: %v", got)
	}
	if row := cw.editor.CursorRow(); row != 1 {
		t.Errorf("cursor row after Up = %v, want 1", row)
	}

	// From the top row, Up leaves the draft for the frame body.
	cw.editor.SetCursorPos(0)
	cw.editor.OnFocusTopRow()
	if app.GetFocus() != tview.Primitive(cw.messageList) {
		t.Errorf("focus after top-row Up = %T, want the message list", app.GetFocus())
	}

	// With the full editor active the content editor's top row hands focus to
	// the title editor instead.
	cw.toggleEditor()
	cw.editor.OnFocusTopRow()
	if app.GetFocus() != tview.Primitive(cw.titleEditor) {
		t.Errorf("focus after top-row Up in the full editor = %T, want the title editor", app.GetFocus())
	}
}

// TestComposerUpDispatchesWithinWrappedRows drives Up through the widget's own
// tview dispatch order (frame input capture, then the focused editor) — the
// live path — and pins that the frame capture no longer steals Up from a
// multiline draft: the caret walks up the wrapped rows, and only the top row
// hands focus to the frame body.
func TestComposerUpDispatchesWithinWrappedRows(t *testing.T) {
	t.Parallel()

	cw, _ := newComposerCW(t)
	cw.editor.SetText(c4Message)
	cw.editor.SetCursorPos(len([]rune(c4Message)))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 20)
	cw.frame.SetRect(0, 0, 100, 20)
	cw.frame.Draw(screen)
	cw.app.SetFocus(cw.editor)

	if rows := cw.editor.MultilineRows(98); rows != 3 {
		t.Fatalf("composer wraps to %v rows, want 3", rows)
	}
	for wantRow := 2; wantRow > 0; wantRow-- {
		if got := cw.editor.CursorRow(); got != wantRow {
			t.Fatalf("cursor row = %v, want %v", got, wantRow)
		}
		if got := cw.pressComposerKey(t, tcell.KeyUp); got != nil {
			t.Fatalf("Up inside the draft was not consumed: %v", got)
		}
		if cw.app.GetFocus() != tview.Primitive(cw.editor) {
			t.Fatalf("Up inside the draft left the composer for %T", cw.app.GetFocus())
		}
	}

	// The draft's top row: Up leaves the composer for the message list.
	if got := cw.pressComposerKey(t, tcell.KeyUp); got != nil {
		t.Fatalf("top-row Up was not consumed: %v", got)
	}
	if cw.app.GetFocus() != tview.Primitive(cw.messageList) {
		t.Errorf("focus after top-row Up = %T, want the message list", cw.app.GetFocus())
	}
}
