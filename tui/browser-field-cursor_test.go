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

// TestBrowserCursorMovesWithFocusOutOfFields pins the live report that the
// hardware cursor stays inside a form's text box when Down moves the page focus
// out of the last field: after typing in the name field and then the message
// field, the second Down must place the cursor on the submit line the moment it
// is pressed, without waiting for a further key.
//
// Root cause: a key pressed while a field is in edit mode is consumed by the
// off-tree field overlay, so it never reaches handleNavKey, and
// moveFieldFocus (the overlay's onExit handler) moved focusLine without
// stamping the keypress that opens the cursor-visibility window. drawCursor
// therefore refused to show the page cursor and the terminal cursor stayed
// where the overlay's own caret last was — in the text box the visitor just
// left. Python refreshes that window on every keypress that moves page focus
// (LinkableText.keypress sets delegate.last_keypress before returning "down"/
// "up" to the Pile, MicronParser.py:932-935).
func TestBrowserCursorMovesWithFocusOutOfFields(t *testing.T) {
	t.Parallel()

	p := newFieldPageProbe(t, guestbookTestPage)
	bd := p.bd

	nameLine := findLine(bd, "Name:")
	msgLine := findLine(bd, "Message:")
	submitLine := findLine(bd, "Sign the guestbook")
	if nameLine < 0 || msgLine < 0 || submitLine < 0 {
		t.Fatal("guestbook page lines not found")
	}

	// The page loads with the first field focused and in edit mode.
	if bd.focusLine != nameLine || bd.fieldOverlay == nil {
		t.Fatalf("on load: focusLine = %v overlay = %v, want the name field in edit mode",
			bd.focusLine, bd.fieldOverlay != nil)
	}
	for _, ch := range "Glenn" {
		if p.dispatch(tcell.KeyRune, ch) != nil {
			t.Fatalf("rune %q not consumed by the name field", string(ch))
		}
	}

	// Down: name field → message field. The overlay moves with the focus, so
	// the cursor is inside the new field's box.
	if p.dispatch(tcell.KeyDown, 0) != nil {
		t.Error("Down (name -> message) not consumed")
	}
	if bd.focusLine != msgLine {
		t.Fatalf("focusLine after Down = %v (%q), want the message field",
			bd.focusLine, bd.linePlainText(bd.focusLine))
	}
	if bd.fieldOverlay != bd.lineFields[msgLine][0].editor {
		t.Fatal("the message field is not in edit mode after Down")
	}
	for _, ch := range "hello" {
		if p.dispatch(tcell.KeyRune, ch) != nil {
			t.Fatalf("rune %q not consumed by the message field", string(ch))
		}
	}

	// Down: message field → submit line. No field is mounted any more, so the
	// hardware cursor must come from the page navigation model — on the submit
	// line, immediately.
	if p.dispatch(tcell.KeyDown, 0) != nil {
		t.Error("Down (message -> submit) not consumed")
	}
	if bd.focusLine != submitLine {
		t.Fatalf("focusLine after the 2nd Down = %v (%q), want the submit line",
			bd.focusLine, bd.linePlainText(bd.focusLine))
	}
	if bd.fieldOverlay != nil {
		t.Error("a field overlay is still mounted after Down left the last field")
	}
	if !bd.cursorHasKeypress {
		t.Error("moveFieldFocus did not open the cursor-visibility window")
	}
	// drawFieldScreen returns the focused line's screen row after drawing, so
	// the cursor must be visible on that row — not on the row of the text box
	// the visitor just left.
	screen, row := drawFieldScreen(t, bd, bd.focusLine)
	wantCursor(t, screen, 0, row, "cursor after Down out of the last field")

	// Up must behave the same way in reverse: focus returns to the message
	// field, whose editor takes the cursor back.
	if p.dispatch(tcell.KeyUp, 0) != nil {
		t.Error("Up (submit -> message) not consumed")
	}
	if bd.focusLine != msgLine || bd.fieldOverlay != bd.lineFields[msgLine][0].editor {
		t.Fatalf("after Up: focusLine = %v overlay mounted = %v, want the message field",
			bd.focusLine, bd.fieldOverlay != nil)
	}
}

// TestBrowserTabOutOfLastFieldKeepsCursorVisible pins the same fix for the Tab
// path, which shares moveFieldFocus: Tab from the last field of a form walks
// forward off the field onto the next selectable line, and the cursor must be
// drawn there rather than left in the box.
func TestBrowserTabOutOfLastFieldKeepsCursorVisible(t *testing.T) {
	t.Parallel()

	p := newFieldPageProbe(t, guestbookTestPage)
	bd := p.bd
	msgLine := findLine(bd, "Message:")
	if msgLine < 0 {
		t.Fatal("message field not found")
	}
	p.dispatch(tcell.KeyDown, 0) // name -> message
	if bd.focusLine != msgLine {
		t.Fatalf("focusLine = %v, want the message field", bd.focusLine)
	}

	if got := p.dispatch(tcell.KeyTab, 0); got != nil && got.Key() != tcell.KeyTab {
		t.Errorf("Tab not consumed by the field (got %v)", got.Key())
	}
	if bd.fieldOverlay != nil {
		t.Error("Tab did not leave the last field")
	}
	if !bd.cursorHasKeypress {
		t.Error("Tab out of the last field did not open the cursor-visibility window")
	}
	screen, row := drawFieldScreen(t, bd, bd.focusLine)
	wantCursor(t, screen, 0, row, "cursor after Tab out of the last field")
}
