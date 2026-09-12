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

// drawFieldScreen draws the browser page view (including the mounted
// text-field overlay) into a simulation screen and returns it with the screen
// row the field's editor occupies.
func drawFieldScreen(t *testing.T, bd *BrowserDisplay, line int) (tcell.SimulationScreen, int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 8)
	bd.content.SetRect(0, 0, 80, 8)
	bd.content.Draw(screen)
	scrollRow, _ := bd.content.GetScrollOffset()
	return screen, bd.rowsAbove(line) - scrollRow
}

// wantCursor asserts that the hardware cursor is shown at (wantX, wantY).
func wantCursor(t *testing.T, screen tcell.SimulationScreen, wantX, wantY int, ctx string) {
	t.Helper()
	x, y, visible := screen.GetCursor()
	if !visible {
		t.Errorf("%v: hardware cursor is hidden, want it shown at (%v,%v)", ctx, wantX, wantY)
		return
	}
	if x != wantX || y != wantY {
		t.Errorf("%v: caret = (%v,%v), want (%v,%v)", ctx, x, y, wantX, wantY)
	}
}

// TestBrowserFieldOverlayUsesPageColors pins the colors of an in-place text
// field to the page's own style, mirroring Python, which wraps each field in
// `urwid.AttrMap(f, make_style(state))` — the field inherits the Micron
// formatting state at its position (MicronParser.py:363, 371), whose defaults
// are the page foreground and the terminal background
// (micron.DefaultFG/DefaultBG).
//
// The overlay is a *tview.InputField, and NewInputField hardcodes
// textStyle = background(Styles.ContrastBackgroundColor).foreground(
// Styles.PrimaryTextColor) — white on bright blue (tview/inputfield.go:139).
// Every other editor in this repo resets those colors explicitly; a browser
// field that does not is the only place a garish blue box could appear.
func TestBrowserFieldOverlayUsesPageColors(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, fieldTestPage)
	fieldLine := findLine(bd, "Search:")
	if fieldLine < 0 {
		t.Fatal("field line not found")
	}
	// Down onto the field line mounts the overlay (the ReadlineEdit that
	// drawFieldOverlay paints over the placeholder text).
	if bd.handleInput(key(tcell.KeyDown, 0)) != nil {
		t.Error("Down not consumed")
	}
	if bd.fieldOverlay == nil || bd.fieldOverlayLine != fieldLine {
		t.Fatalf("field overlay not mounted on line %d", fieldLine)
	}

	screen, row := drawFieldScreen(t, bd, fieldLine)
	rf := bd.lineFields[fieldLine][0]

	// The field's first cell holds the first rune of its initial text
	// ("initial"), so the overlay really is what painted it.
	got, _, style, _ := cellContent(screen, rf.startCol, row)
	if got != 'i' {
		t.Fatalf("cell (%d,%d) = %q, want 'i' (the field's initial text)", rf.startCol, row, string(got))
	}

	fg, bg, _ := style.Decompose()
	if bg != tcell.ColorDefault {
		t.Errorf("field background = %v, want the page background (tview's NewInputField default is Styles.ContrastBackgroundColor = blue)", bg)
	}

	// The field text must match the page text beside it: cell startCol-1 is the
	// space of the "Search: " label, painted by the page's Micron style.
	_, _, labelStyle, _ := cellContent(screen, rf.startCol-1, row)
	labelFG, labelBG, _ := labelStyle.Decompose()
	if fg != labelFG || bg != labelBG {
		t.Errorf("field colors = (%v, %v), want the page text colors (%v, %v)", fg, bg, labelFG, labelBG)
	}
}

// TestBrowserFieldOverlayCaretAtInsertionPoint pins where the hardware cursor
// sits while a field is being edited: urwid positions the terminal cursor at
// the focused Edit's edit_pos (urwid.Edit.render → canvas.cursor), so the caret
// is at the insertion point inside the field — not at the field's left edge.
//
// The browser keeps tview focus on the page body and drives the overlay through
// bd.handleInput, so the editor is never the tview-focused primitive and
// ReadlineEdit.Draw's `HasFocus()` gate never fires. The caret must therefore
// follow the overlay's own model cursor while it is mounted.
func TestBrowserFieldOverlayCaretAtInsertionPoint(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, fieldTestPage)
	fieldLine := findLine(bd, "Search:")
	if fieldLine < 0 {
		t.Fatal("field line not found")
	}
	if bd.handleInput(key(tcell.KeyDown, 0)) != nil {
		t.Error("Down not consumed")
	}
	rf := bd.lineFields[fieldLine][0]

	// urwid.Edit.set_edit_text leaves edit_pos at the end of the text, so the
	// caret starts one column past the last rune of "initial".
	const initial = "initial"
	wantX := rf.startCol + len([]rune(initial))

	screen, wantY := drawFieldScreen(t, bd, fieldLine)
	wantCursor(t, screen, wantX, wantY, "insertion point after the initial text")
	if got := bd.fieldOverlay.CursorPos(); got != len([]rune(initial)) {
		t.Fatalf("model cursor = %v, want %v", got, len([]rune(initial)))
	}
}

// TestBrowserFieldOverlayCaretMovesWithTyping pins the caret advancing one
// column per typed rune (and following backspace), which is what makes typing
// legible: urwid moves the terminal cursor with edit_pos on every keystroke.
func TestBrowserFieldOverlayCaretMovesWithTyping(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, fieldTestPage)
	fieldLine := findLine(bd, "Search:")
	if fieldLine < 0 {
		t.Fatal("field line not found")
	}
	if bd.handleInput(key(tcell.KeyDown, 0)) != nil {
		t.Error("Down not consumed")
	}
	rf := bd.lineFields[fieldLine][0]

	for _, tc := range []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{name: "type X", event: key(tcell.KeyRune, 'X'), want: "initialX"},
		{name: "type Y", event: key(tcell.KeyRune, 'Y'), want: "initialXY"},
		{name: "backspace", event: key(tcell.KeyBackspace2, 0), want: "initialX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if bd.handleInput(tc.event) != nil {
				t.Fatalf("%v not consumed by the field overlay", tc.name)
			}
			if got := bd.fieldOverlay.GetText(); got != tc.want {
				t.Fatalf("field text = %q, want %q", got, tc.want)
			}
			screen, wantY := drawFieldScreen(t, bd, fieldLine)
			// The caret sits one column past the last rune of the buffer,
			// because typing and backspace both leave edit_pos at the end.
			wantX := rf.startCol + len([]rune(tc.want))
			wantCursor(t, screen, wantX, wantY, "after "+tc.name)
		})
	}
}

// TestBrowserFieldOverlayArrowKeysMoveCaret pins Left/Right moving the field's
// own edit position while the overlay is mounted, mirroring urwid.Edit's
// "left"/"right" keypress (which consumes the key and moves edit_pos). Without
// this the arrows would move the page's part cursor instead, leaving the
// visible caret stranded at the end of the text.
func TestBrowserFieldOverlayArrowKeysMoveCaret(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, fieldTestPage)
	fieldLine := findLine(bd, "Search:")
	if fieldLine < 0 {
		t.Fatal("field line not found")
	}
	if bd.handleInput(key(tcell.KeyDown, 0)) != nil {
		t.Error("Down not consumed")
	}
	rf := bd.lineFields[fieldLine][0]

	for _, tc := range []struct {
		name string
		key  tcell.Key
		want int
	}{
		{name: "left once", key: tcell.KeyLeft, want: 6},
		{name: "left twice", key: tcell.KeyLeft, want: 5},
		{name: "right once", key: tcell.KeyRight, want: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if bd.handleInput(key(tc.key, 0)) != nil {
				t.Fatalf("%v not consumed by the field overlay while editing", tc.name)
			}
			if got := bd.fieldOverlay.CursorPos(); got != tc.want {
				t.Fatalf("model cursor = %v, want %v", got, tc.want)
			}
			screen, wantY := drawFieldScreen(t, bd, fieldLine)
			wantCursor(t, screen, rf.startCol+tc.want, wantY, tc.name)
		})
	}
}

// guestbookTestPage mirrors what guestbook.wasm renders above its entries: the
// form block (assets/wasm-pages/guestbook.wat data segment at offset 1152) with
// two labelled, empty, background-highlighted fields on their own lines and a
// submit link collecting both, then the page's visit line — which ends in the
// quiet site-counter partial the shipped page embeds there.
const guestbookTestPage = "Name: `B444`<name`>`b\n" +
	"Message: `B444`<32|message`>`b\n" +
	"`[Sign the guestbook`:/page/guestbook.wasm`name|message]\n" +
	"\n`!Using the form`!: the selected field has the keyboard, so just type. " +
	"`!Down`! / `!Up`! (or `!Tab`!) move between fields, and `!Enter`! on the " +
	"`!Sign the guestbook`! line submits what you typed. Every field is a " +
	"readline-style editor: `!Ctrl-A`! / `!Ctrl-E`! start / end of the line, " +
	"`!Ctrl-U`! clears back to the start, `!Ctrl-K`! clears to the end, " +
	"`!Ctrl-W`! deletes a word, `!Ctrl-L`! clears the field, and `!Ctrl-Y`! " +
	"pastes back what you cleared.\n" +
	"\nGuestbook has been visited 3 times.`{:/page/hit-counter.wasm`0`quiet=1}\n" +
	"----\n\n"

// newLoadedFieldTestBrowser renders a page with the page body already focused,
// mirroring an in-app page load: showContent restores focus to bd.content when
// the loading body held it, so the page body is focused by the time the page
// finishes rendering (browser.go showContent). newFieldTestBrowser focuses only
// after rendering, which cannot exercise load-time field focus.
func newLoadedFieldTestBrowser(t *testing.T, page string) (*App, *BrowserDisplay) {
	t.Helper()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.content.SetRect(0, 0, 80, 6)
	app.SetFocus(bd.content)
	bd.currentMarkup = page
	bd.renderPage()
	return app, bd
}

// TestBrowserGuestbookFieldTypedOnLoad pins Python's load-time focus for a form
// page: the freshly built Pile focuses its first selectable widget, which on the
// guestbook is the name field's Edit, so a visitor's first keystroke lands in
// the field with no navigation key pressed. Previously the editor overlay
// mounted only when a line-changing key ran syncFieldFocus, so on a freshly
// loaded guestbook every keystroke was discarded.
func TestBrowserGuestbookFieldTypedOnLoad(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, guestbookTestPage)
	if bd.fieldOverlay == nil {
		t.Fatal("no field overlay mounted on load: the field cannot receive keystrokes")
	}
	rf := bd.lineFields[bd.focusLine][0]
	if rf.spec.Name != "name" {
		t.Fatalf("focused field = %q, want %q", rf.spec.Name, "name")
	}

	for _, r := range "Glenn" {
		if bd.handleInput(key(tcell.KeyRune, r)) != nil {
			t.Fatalf("rune %q not consumed on load", string(r))
		}
	}
	// The field is empty until typed into: the form's labels are page text, so
	// nothing precedes the visitor's name.
	if got, want := bd.fieldOverlay.GetText(), "Glenn"; got != want {
		t.Errorf("field text = %q, want %q", got, want)
	}
}

// TestBrowserGuestbookFieldsCaretAcrossFields walks the shipped guestbook form
// the way a visitor does: the page opens with the name field focused and its
// overlay mounted — the exact state of the live gonomadnet session, where that
// field rendered as a white-on-blue box with the caret stranded on its first
// column. Down then moves to the message field, which must take over the caret.
func TestBrowserGuestbookFieldsCaretAcrossFields(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, guestbookTestPage)

	for _, tc := range []struct {
		name      string
		label     string // the line's plain text, identifying the field
		fieldName string
		typed     string
		down      bool // press Down first (the page opens on the first field)
	}{
		{name: "name field", label: "Name:", fieldName: "name", typed: "Glenn"},
		{name: "message field", label: "Message:", fieldName: "message", typed: "hello", down: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if findLine(bd, tc.label) < 0 {
				t.Fatalf("line %q not found", tc.label)
			}
			// Down moves the page focus onto the next field's line, mounting
			// that field's ReadlineEdit overlay.
			if tc.down && bd.handleInput(key(tcell.KeyDown, 0)) != nil {
				t.Error("Down not consumed")
			}
			wantFocus(t, bd, tc.label)
			if bd.fieldOverlay == nil {
				t.Fatal("no field overlay mounted")
			}
			line := bd.focusLine
			rf := bd.lineFields[line][0]
			if rf.spec.Name != tc.fieldName {
				t.Fatalf("mounted field = %q, want %q", rf.spec.Name, tc.fieldName)
			}

			for _, r := range tc.typed {
				if bd.handleInput(key(tcell.KeyRune, r)) != nil {
					t.Fatalf("rune %q not consumed", string(r))
				}
			}
			want := rf.spec.Data + tc.typed
			if got := bd.fieldOverlay.GetText(); got != want {
				t.Fatalf("field text = %q, want %q", got, want)
			}

			screen, row := drawFieldScreen(t, bd, line)
			wantCursor(t, screen, rf.startCol+len([]rune(want)), row,
				tc.name+" caret after typing")

			// The field paints the background its markup declares (the form's
			// `B444` box), never tview's ContrastBackgroundColor default.
			_, _, style, _ := cellContent(screen, rf.startCol, row)
			if _, bg, _ := style.Decompose(); bg != parseColor("#444444") {
				t.Errorf("%v field background = %v, want #444444 (the markup's background), not tview's blue default", tc.name, bg)
			}
		})
	}
}
