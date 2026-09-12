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
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// clickScreenX is the screen column of a field's first cell: the content
// pane's inner left edge plus the field's start column within its line.
func clickScreenX(bd *BrowserDisplay, rf *renderedField) int {
	x0, _, _, _ := bd.content.GetInnerRect()
	return x0 + rf.startCol
}

// typeRunes feeds s to the browser one rune per event, failing if any rune is
// not consumed (i.e. did not reach the field editor the test expects to hold
// the keyboard).
func typeRunes(t *testing.T, bd *BrowserDisplay, s string) {
	t.Helper()
	for _, r := range s {
		if bd.handleInput(key(tcell.KeyRune, r)) != nil {
			t.Fatalf("rune %q not consumed by the focused field", string(r))
		}
	}
}

// TestBrowserClickOnFormFieldFocusesIt pins the live report that neither input
// box on a Micron form page could be clicked: the field editors are drawn as
// off-tree overlays, so tview had no widget under the mouse and a click was
// ignored. Python's urwid tree handles this in Pile.mouse_event (focus the row
// under the press) plus Edit.mouse_event (move the edit cursor to the clicked
// cell), so a click in a field puts the caret there and typing goes into it.
func TestBrowserClickOnFormFieldFocusesIt(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, guestbookTestPage)
	mouseScreen(t, bd)

	nameLine := findLine(bd, "Name:")
	msgLine := findLine(bd, "Message:")
	if nameLine < 0 || msgLine < 0 {
		t.Fatal("guestbook fields not found in the rendered page")
	}

	t.Run("first field", func(t *testing.T) {
		rf := bd.lineFields[nameLine][0]
		clickAt(bd, clickScreenX(bd, rf), bd.rowsAbove(nameLine))
		wantFocus(t, bd, "Name:")
		if bd.fieldOverlay != rf.editor {
			t.Fatal("clicking the name field did not mount its editor")
		}
		if got := rf.editor.CursorPos(); got != 0 {
			t.Errorf("caret on an empty field = %v, want 0", got)
		}
		typeRunes(t, bd, "Glenn")
		if got, want := rf.editor.GetText(), "Glenn"; got != want {
			t.Fatalf("clicked field text = %q, want %q", got, want)
		}
		// Clicking inside the filled value moves the caret to the clicked cell,
		// mirroring urwid's Edit.mouse_event → move_cursor_to_coords.
		clickAt(bd, clickScreenX(bd, rf)+3, bd.rowsAbove(nameLine))
		if got, want := rf.editor.CursorPos(), 3; got != want {
			t.Errorf("caret = %v, want %v (the clicked cell)", got, want)
		}
		if got, want := bd.lineCursors[nameLine], rf.runeStart+3; got != want {
			t.Errorf("field part cursor = %v, want %v (inside the field span)", got, want)
		}
	})

	t.Run("second field", func(t *testing.T) {
		rf := bd.lineFields[msgLine][0]
		clickAt(bd, clickScreenX(bd, rf)+1, bd.rowsAbove(msgLine))
		wantFocus(t, bd, "Message:")
		if bd.fieldOverlay != rf.editor {
			t.Fatal("clicking the message field did not mount its editor")
		}
		typeRunes(t, bd, "hello")
		// The caret is clamped to the text length, so clicking the blank area
		// right of a filled field lands at its end.
		clickAt(bd, clickScreenX(bd, rf)+20, bd.rowsAbove(msgLine))
		if got, want := rf.editor.CursorPos(), len([]rune("hello")); got != want {
			t.Errorf("caret after clicking past the text = %v, want %v", got, want)
		}
		if got, want := bd.lineFields[nameLine][0].editor.GetText(), "Glenn"; got != want {
			t.Errorf("name field = %q, want %q (the other field must keep its value)", got, want)
		}
	})
}

// TestBrowserClickOnPlainLineMovesFocus pins the rest of urwid's click model:
// a click on a line that carries no field still moves the page focus (and the
// part cursor) to the clicked line — Python's Pile.mouse_event focuses the row
// under a button-1 press. The hardware cursor must be drawn there, which is why
// a click stamps the same visibility window a nav key does.
func TestBrowserClickOnPlainLineMovesFocus(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, guestbookTestPage)
	mouseScreen(t, bd)

	submit := findLine(bd, "Sign the guestbook")
	if submit < 0 {
		t.Fatal("submit line not found")
	}
	clickAt(bd, 4, bd.rowsAbove(submit))
	if bd.focusLine != submit {
		t.Errorf("focusLine = %v (%q), want %v (the submit line)",
			bd.focusLine, bd.linePlainText(bd.focusLine), submit)
	}
	if bd.fieldOverlay != nil {
		t.Error("a non-field line must not mount a field overlay")
	}
	screen, row := drawFieldScreen(t, bd, submit)
	wantCursor(t, screen, 4, row, "hardware cursor after a click on the submit line")
}

// TestBrowserClickOnBlankLineIsNoOp pins Python's guard on the same click
// model: urwid's Pile moves focus only to a button-1-press row that is
// selectable (pile.py:1083-1084), and a blank line renders as urwid.Text, which
// is not selectable — so clicking empty space below the content changes
// nothing at all, neither the focused line nor the cursor.
func TestBrowserClickOnBlankLineIsNoOp(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, guestbookTestPage)
	mouseScreen(t, bd)

	blank := -1
	for i := range bd.currentLines {
		if bd.linePlainText(i) == "" && len(bd.lineFields[i]) == 0 {
			blank = i
			break
		}
	}
	if blank < 0 {
		t.Fatal("no blank line in the rendered page")
	}
	before := bd.focusLine
	clickAt(bd, 3, bd.rowsAbove(blank))
	if bd.focusLine != before {
		t.Errorf("focusLine = %v, want %v (a blank line is not selectable)",
			bd.focusLine, before)
	}
	if bd.cursorHasKeypress {
		t.Error("a click on a blank line must not open the cursor-visibility window")
	}
}

// clickRoundTripPage gives the click mapping the geometries it has to invert: a
// line that wraps over several rows, a centered line, a right-aligned line, and
// an indented section body.
const clickRoundTripPage = "`cCentered" + " " + "line\n" +
	"`rRight aligned line\n" +
	"`a\n" +
	"Plain paragraph long enough to wrap across several rows in a narrow " +
	"browser pane, so the wrapped-row offset matters.\n" +
	">>Nested heading\n" +
	"Indented body text long enough to wrap too.\n" +
	"<Back at the root.\n"

// TestClickPositionRoundTripsCursorScreenXY pins that the click mapping is the
// exact inverse of the model the hardware cursor uses: for every rune position
// on every line, cursorScreenXY's (x, y) maps back to that same position. A
// click lands where the glyph under the mouse is, so any drift here would put
// the caret on the wrong character (or the wrong wrapped row) on aligned and
// indented pages.
func TestClickPositionRoundTripsCursorScreenXY(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, clickRoundTripPage)
	mouseScreen(t, bd)
	x0, _, _, _ := bd.content.GetInnerRect()
	var sawWrappedRow, sawAligned bool

	for line := range bd.currentLines {
		plain := bd.linePlainText(line)
		if l := bd.currentLines[line]; l != nil && l.Align != micron.AlignLeft {
			sawAligned = true
		}
		bd.focusLine = line
		for pos := 0; pos <= len([]rune(plain)); pos++ {
			bd.lineCursors[line] = pos
			wantX, wantRow, ok := bd.cursorScreenXY()
			if !ok {
				t.Fatalf("line %v pos %v: no cursor position", line, pos)
			}
			if wantRow > 0 {
				sawWrappedRow = true
			}
			got := bd.linePosAtScreenX(line, wantRow, x0+wantX)
			if got != pos {
				t.Errorf("line %v (%q) pos %v → screen (%v,%v) → pos %v",
					line, plain, pos, wantX, wantRow, got)
			}
		}
	}
	if !sawWrappedRow || !sawAligned {
		t.Errorf("fixture did not cover wrapped rows (%v) and aligned lines (%v)",
			sawWrappedRow, sawAligned)
	}
}

// TestBrowserClickOnSubmitButtonFollowsLink pins that the submit control stays
// a working link after its button chrome: the region tags still cover the
// bracketed label, so the existing click→link dispatch fires and collects the
// live values of the fields the link names.
func TestBrowserClickOnSubmitButtonFollowsLink(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, guestbookTestPage)
	var gotURL string
	var gotFields map[string]string
	bd.OnRetrieveURL = func(url string, requestData map[string]string) {
		gotURL, gotFields = url, requestData
	}

	submit := findLine(bd, "Sign the guestbook")
	if submit < 0 {
		t.Fatal("submit line not found")
	}
	bd.lineFields[findLine(bd, "Name:")][0].editor.SetText("Glenn")
	bd.lineFields[findLine(bd, "Message:")][0].editor.SetText("hello")

	// The screen must be drawn first: the click→link path resolves the clicked
	// cell through the TextView's region highlights.
	mouseScreen(t, bd)
	clickAt(bd, 2, bd.rowsAbove(submit))
	if gotURL != ":/page/guestbook.wasm" {
		t.Fatalf("followed URL = %q, want %q", gotURL, ":/page/guestbook.wasm")
	}
	// Python keys the submitted values "field_"+field_name
	// (Browser.py:247-264 recurse_down).
	if gotFields["field_name"] != "Glenn" || gotFields["field_message"] != "hello" {
		t.Errorf("submitted fields = %v, want the typed values", gotFields)
	}
}
