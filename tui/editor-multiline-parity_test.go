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

// The goldens in this file were captured live from the source-of-truth urwid
// 4.0.3 Edit widget (multiline=True, wrap="space") — the widget behind Python
// nomadnet's RoomMessageEdit footer (Channels.py:605). The Channels room
// composer must wrap like urwid: the footer grows by one row per wrapped
// line (urwid Frame shrinks the message list), and the caret never leaves
// the panel (fleet bug: gonomadnet's single-row InputField clipped the text
// at the right border and ShowCursor walked the caret beyond the panel).

// c4Message is the exact draft typed live on glenn-mac-mini-m2 during the
// parity report; wrapped at the room panel's inner width (96) it produces the
// three visible editor rows in the Python capture.
const c4Message = "Message C4 from glenn-mac-mini-m2 again, but this time typing way " +
	"beyond the length of the input line to see how it is being handled, and it " +
	"turns out that nomadnet will move the whole panel up one or more lines to " +
	"accomodate the extra lines of input."

func TestEditorRowsUrwidParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		width int
		want  []editorRow
	}{
		{"perfect space wrap", "abcd efgh", 4, []editorRow{{0, 4, 4}, {5, 9, 9}}},
		{"multi space run", "aaaaa   bb", 5, []editorRow{{0, 5, 5}, {6, 10, 10}}},
		{"walk back break", "abcde fghij", 5, []editorRow{{0, 5, 5}, {6, 11, 11}}},
		{"trailing spaces kept", "abc    ", 10, []editorRow{{0, 7, 7}}},
		{"exact fit", "abcd", 4, []editorRow{{0, 4, 4}}},
		{"newline gap", "ab\ncd", 4, []editorRow{{0, 2, 2}, {3, 5, 5}}},
		{"wide rune", "ab漢字", 4, []editorRow{{0, 3, -1}, {3, 4, 4}}},
		{"hard break no tail", "abcdefghij", 4,
			[]editorRow{{0, 4, -1}, {4, 8, -1}, {8, 10, 10}}},
		{"hard break last row", "abcdefghi", 4,
			[]editorRow{{0, 4, -1}, {4, 8, -1}, {8, 9, 9}}},
		{"leading spaces", "  ab", 4, []editorRow{{0, 4, 4}}},
		{"double dropped", "a  bcd", 4, []editorRow{{0, 2, 2}, {3, 6, 6}}},
		{"triple spaces", "ab   cd", 6, []editorRow{{0, 4, 4}, {5, 7, 7}}},
		{"boundary space inside row", "abc   d", 4,
			[]editorRow{{0, 4, 4}, {5, 7, 7}}},
		{"trailing spaces empty row", "abc  ", 4,
			[]editorRow{{0, 4, 4}, {5, 5, 5}}},
		{"trailing newline", "abc\n", 4, []editorRow{{0, 3, 3}, {4, 4, 4}}},
		{"empty middle segment", "abc\n\nx", 4,
			[]editorRow{{0, 3, 3}, {4, 4, 4}, {5, 6, 6}}},
		{"two trailing newlines", "ab\n\n", 4,
			[]editorRow{{0, 2, 2}, {3, 3, 3}, {4, 4, 4}}},
		{"empty text", "", 4, []editorRow{{0, 0, 0}}},
		{"wide runes exact fill", "漢字漢字", 4,
			[]editorRow{{0, 2, -1}, {2, 4, 4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := editorRows([]rune(tt.text), tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("editorRows(%q, %v) rows = %v (len %v), want %v",
					tt.text, tt.width, got, len(got), tt.want)
			}
			for i, row := range got {
				if row != tt.want[i] {
					t.Errorf("editorRows(%q, %v) row %v = %v, want %v",
						tt.text, tt.width, i, row, tt.want[i])
				}
			}
		})
	}
}

// TestEditorRowsC4MessageParity pins the live capture: the draft typed on
// glenn-mac-mini-m2 wraps to exactly three rows at width 96, with the breaks
// and the caret column matching the Python nomadnet panel.
func TestEditorRowsC4MessageParity(t *testing.T) {
	t.Parallel()

	runes := []rune(c4Message)
	rows := editorRows(runes, 96)
	wantRows := []editorRow{{0, 96, 96}, {97, 193, 193}, {194, 251, 251}}
	if len(rows) != len(wantRows) {
		t.Fatalf("C4 rows = %v (len %v), want %v", rows, len(rows), wantRows)
	}
	for i, row := range rows {
		if row != wantRows[i] {
			t.Errorf("C4 row %v = %v, want %v", i, row, wantRows[i])
		}
	}
	wantLines := []string{
		"Message C4 from glenn-mac-mini-m2 again, but this time typing way beyond the length of the input",
		"line to see how it is being handled, and it turns out that nomadnet will move the whole panel up",
		"one or more lines to accomodate the extra lines of input.",
	}
	for i, row := range rows {
		if got := string(runes[row.start:row.end]); got != wantLines[i] {
			t.Errorf("C4 row %v = %q, want %q", i, got, wantLines[i])
		}
	}
	// The caret at the end of the draft sits on row 2, column 57 (unshifted:
	// 57 < 96, so no horizontal window shift), exactly as urwid reports.
	x, y := editorCalcCoords(runes, rows, len(runes))
	if x != 57 || y != 2 {
		t.Errorf("C4 caret = (%v,%v), want (57,2)", x, y)
	}
}

func TestEditorCalcCoordsUrwidParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		w    int
		pos  int
		x, y int
	}{
		{"gap start", "ab  cd", 4, 2, 2, 0},
		{"gap second space", "ab  cd", 4, 3, 3, 0},
		{"gap end maps to next row", "ab  cd", 4, 4, 0, 1},
		{"double dropped tail", "a  bcd", 4, 2, 2, 0},
		{"double dropped next", "a  bcd", 4, 3, 0, 1},
		{"triple dropped tail", "ab   cd", 6, 4, 4, 0},
		{"triple dropped next", "ab   cd", 6, 5, 0, 1},
		{"hard break maps to next row", "abcdefghij", 4, 4, 0, 1},
		{"hard break end", "abcdefghij", 4, 10, 2, 2},
		{"full row end unshifted", "abcdefg", 7, 7, 7, 0},
		{"newline tail", "abc\n", 4, 3, 3, 0},
		{"after newline", "abc\n", 4, 4, 0, 1},
		{"empty middle segment", "abc\n\nx", 4, 4, 0, 1},
		{"empty middle next", "abc\n\nx", 4, 5, 0, 2},
		{"empty text", "", 4, 0, 0, 0},
		{"wide rune col", "漢字漢字", 4, 1, 2, 0},
		{"wide rune next row", "漢字漢字", 4, 2, 0, 1},
		{"line start", "abcd\nefgh", 4, 0, 0, 0},
		{"line end tail", "abcd\nefgh", 4, 4, 4, 0},
		{"second line start", "abcd\nefgh", 4, 5, 0, 1},
		{"second line end", "abcd\nefgh", 4, 9, 4, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rows := editorRows([]rune(tt.text), tt.w)
			x, y := editorCalcCoords([]rune(tt.text), rows, tt.pos)
			if x != tt.x || y != tt.y {
				t.Errorf("calcCoords(%q @%v, pos %v) = (%v,%v), want (%v,%v)",
					tt.text, tt.w, tt.pos, x, y, tt.x, tt.y)
			}
		})
	}
}

func TestEditorCalcPosUrwidParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		w    int
		pref int
		row  int
		want int
	}{
		{"pref beyond row goes to row end", "abcd\nefgh", 4, 5, 0, 4},
		{"pref beyond second row", "abcd\nefgh", 4, 5, 1, 9},
		{"pref within row exact", "abcd\nefgh", 4, 2, 0, 2},
		{"pref zero", "abcd\nefgh", 4, 0, 1, 5},
		{"empty row any pref", "abc\n", 4, 0, 1, 4},
		{"empty row pref high", "abc\n", 4, 7, 1, 4},
		{"hard break row end", "abcdefghij", 4, 5, 0, 3},
		{"wide rune stops before", "漢字漢字", 4, 1, 0, 0},
		{"wide rune exact", "漢字漢字", 4, 2, 0, 1},
		{"wide rune crosses", "漢字漢字", 4, 3, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rows := editorRows([]rune(tt.text), tt.w)
			got := editorCalcPos([]rune(tt.text), rows, tt.pref, tt.row)
			if got != tt.want {
				t.Errorf("calcPos(%q @%v, pref %v, row %v) = %v, want %v",
					tt.text, tt.w, tt.pref, tt.row, got, tt.want)
			}
		})
	}
}

func TestEditorCalcPosEdgeParity(t *testing.T) {
	t.Parallel()

	runes := []rune("ab\ncd")
	rows := editorRows(runes, 4)
	if got := editorCalcPosLeft(runes, rows, 1); got != 3 {
		t.Errorf("calcPosLeft(row 1) = %v, want 3 (row start)", got)
	}
	if got := editorCalcPosRight(runes, rows, 1); got != 5 {
		t.Errorf("calcPosRight(row 1) = %v, want 5 (row tail/end)", got)
	}
	// A hard-broken row has no tail: End lands on the last char position,
	// exactly as urwid's calc_text_pos resolves the closest segment end.
	hard := []rune("abcdefghij")
	hardRows := editorRows(hard, 4)
	if got := editorCalcPosRight(hard, hardRows, 0); got != 3 {
		t.Errorf("calcPosRight(hard row 0) = %v, want 3 (last char pos)", got)
	}
}

// multilineEditor returns a multiline ReadlineEdit laid out at the given
// width so Up/Down/Home/End can resolve wrapped-row geometry.
func multilineEditor(t *testing.T, width int) *ReadlineEdit {
	t.Helper()
	re := NewReadlineEdit(&killRing{}, "", "")
	re.SetMultiline(true)
	re.SetRect(0, 0, width, 4)
	return re
}

// pressKey feeds one key event to the editor's readline handler; the return
// is the unconsumed event (nil when the editor handled it).
func pressKey(re *ReadlineEdit, key tcell.Key, runeCh rune) *tcell.EventKey {
	return re.handleKey(tcell.NewEventKey(key, runeCh, tcell.ModNone))
}

func TestReadlineEditMultilineEnterParity(t *testing.T) {
	t.Parallel()

	re := multilineEditor(t, 20)
	re.SetText("abc")
	if ev := pressKey(re, tcell.KeyEnter, 0); ev != nil {
		t.Fatal("multiline Enter was not consumed")
	}
	if got := re.GetText(); got != "abc\n" {
		t.Errorf("multiline Enter text = %q, want %q (urwid inserts a newline)", got, "abc\n")
	}
	if got := re.CursorPos(); got != 4 {
		t.Errorf("multiline Enter cursor = %v, want 4", got)
	}
}

func TestReadlineEditMultilineUpDownParity(t *testing.T) {
	t.Parallel()

	// urwid probe: fresh buffer "abcd\nefgh" with the cursor at the end
	// (pos 9, row 1): Down returns the key (last row), Up moves to row 0
	// keeping the current column.
	re := multilineEditor(t, 4)
	re.SetText("abcd\nefgh")
	re.SetCursorPos(9)
	if ev := pressKey(re, tcell.KeyDown, 0); ev == nil {
		t.Error("Down on the last wrapped row should return the key to the parent")
	}
	if got := re.CursorPos(); got != 9 {
		t.Errorf("Down on the last row moved the cursor to %v, want 9", got)
	}
	if ev := pressKey(re, tcell.KeyUp, 0); ev != nil {
		t.Fatal("Up from the last row to row 0 was not consumed")
	}
	if got := re.CursorPos(); got != 3 {
		t.Errorf("Up to row 0 landed at %v, want 3 (urwid: column 3)", got)
	}

	// Down from row 0 col 0 → row 1 col 0 (pos 5); consecutive vertical
	// moves keep the preferred column; Up returns the same way.
	re2 := multilineEditor(t, 4)
	re2.SetText("abcd\nefgh")
	re2.SetCursorPos(0)
	if ev := pressKey(re2, tcell.KeyDown, 0); ev != nil {
		t.Fatal("Down from row 0 was not consumed")
	}
	if got := re2.CursorPos(); got != 5 {
		t.Errorf("Down from row 0 landed at %v, want 5", got)
	}
	if ev := pressKey(re2, tcell.KeyUp, 0); ev != nil {
		t.Fatal("Up back to row 0 was not consumed")
	}
	if got := re2.CursorPos(); got != 0 {
		t.Errorf("Up back to row 0 landed at %v, want 0", got)
	}

	// Wrapped (not logical) rows: "abcdefgh\nij" at width 4 has three
	// wrapped rows [0,4) [4,8) [9,11); Down walks all of them.
	re3 := multilineEditor(t, 4)
	re3.SetText("abcdefgh\nij")
	re3.SetCursorPos(0)
	for step, want := range []int{4, 9} {
		if ev := pressKey(re3, tcell.KeyDown, 0); ev != nil {
			t.Fatalf("Down step %v was not consumed", step)
		}
		if got := re3.CursorPos(); got != want {
			t.Errorf("Down step %v landed at %v, want %v", step, got, want)
		}
	}
	// One more Down is out of range and returns the key.
	if ev := pressKey(re3, tcell.KeyDown, 0); ev == nil {
		t.Error("Down past the last wrapped row should return the key")
	}
	if ev := pressKey(re3, tcell.KeyUp, 0); ev != nil {
		t.Fatal("Up was not consumed")
	}
	if got := re3.CursorPos(); got != 4 {
		t.Errorf("Up landed at %v, want 4", got)
	}
}

func TestReadlineEditMultilineUpTopRowDelegates(t *testing.T) {
	t.Parallel()

	re := multilineEditor(t, 4)
	fired := 0
	re.OnFocusTopRow = func() { fired++ }
	re.SetText("abcd\nefgh")
	re.SetCursorPos(1) // row 0
	if ev := pressKey(re, tcell.KeyUp, 0); ev != nil {
		t.Fatal("Up at the top wrapped row should be consumed by the delegate")
	}
	if fired != 1 {
		t.Errorf("OnFocusTopRow fired %v times, want 1 (urwid RoomMessageEdit focuses the body)", fired)
	}

	// A mid-buffer cursor on a LATER wrapped row moves within the rows and
	// must not fire the delegate.
	fired = 0
	re.SetCursorPos(6) // row 1
	if ev := pressKey(re, tcell.KeyUp, 0); ev != nil {
		t.Fatal("Up within wrapped rows was not consumed")
	}
	if fired != 0 {
		t.Errorf("OnFocusTopRow fired %v times from row 1, want 0", fired)
	}
	if got := re.CursorPos(); got != 1 {
		t.Errorf("Up within wrapped rows landed at %v, want 1", got)
	}
}

func TestReadlineEditMultilineHomeEndParity(t *testing.T) {
	t.Parallel()

	// urwid MAX_LEFT/MAX_RIGHT: Home/End move within the WRAPPED row.
	re := multilineEditor(t, 4)
	re.SetText("abcd\nefgh")
	re.SetCursorPos(7) // row 1, column 2
	if ev := pressKey(re, tcell.KeyHome, 0); ev != nil {
		t.Fatal("Home was not consumed")
	}
	if got := re.CursorPos(); got != 5 {
		t.Errorf("Home landed at %v, want 5 (start of wrapped row 1)", got)
	}
	if ev := pressKey(re, tcell.KeyEnd, 0); ev != nil {
		t.Fatal("End was not consumed")
	}
	if got := re.CursorPos(); got != 9 {
		t.Errorf("End landed at %v, want 9 (tail of wrapped row 1)", got)
	}

	// urwid quirk on a hard-broken row (no tail segment): End resolves to
	// the last char position of the row.
	hard := multilineEditor(t, 4)
	hard.SetText("abcdefghij")
	hard.SetCursorPos(5) // row 1 [4,8)
	if ev := pressKey(hard, tcell.KeyEnd, 0); ev != nil {
		t.Fatal("End was not consumed")
	}
	if got := hard.CursorPos(); got != 7 {
		t.Errorf("End on a hard-broken row landed at %v, want 7", got)
	}
}

func TestReadlineEditMultilineBackspaceJoinsParity(t *testing.T) {
	t.Parallel()

	re := multilineEditor(t, 20)
	re.SetText("ab\ncd")
	re.SetCursorPos(3) // start of the second logical line
	if ev := pressKey(re, tcell.KeyBackspace2, 0); ev != nil {
		t.Fatal("Backspace was not consumed")
	}
	if got := re.GetText(); got != "abcd" {
		t.Errorf("Backspace at a line start = %q, want %q (urwid joins the lines)", got, "abcd")
	}
	if got := re.CursorPos(); got != 2 {
		t.Errorf("Backspace cursor = %v, want 2", got)
	}
}

// TestReadlineEditMultilineDrawWraps pins the draw side: the editor renders
// every wrapped row and keeps the hardware caret inside the field, applying
// urwid's row shift when the cursor lands past the row's fill column.
func TestReadlineEditMultilineDrawWraps(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "", "")
	re.SetMultiline(true)
	re.SetFieldBackgroundColor(tcell.ColorBlack)
	re.SetFieldTextColor(tcell.ColorWhite)
	re.SetText(c4Message)
	re.SetCursorPos(len([]rune(c4Message)))
	re.Focus(func(tview.Primitive) {})

	const width = 96
	const height = 4
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	re.SetRect(0, 0, width, height)
	re.Draw(screen)

	// Three wrapped rows at width 96 (urwid golden), rendered in order.
	wantLines := []string{
		"Message C4 from glenn-mac-mini-m2 again, but this time typing way beyond the length of the input",
		"line to see how it is being handled, and it turns out that nomadnet will move the whole panel up",
		"one or more lines to accomodate the extra lines of input.",
	}
	for row, want := range wantLines {
		var got strings.Builder
		for x := range width {
			ch, _, _ := screen.Get(x, row)
			got.WriteString(ch)
		}
		if line := strings.TrimRight(got.String(), " "); line != want {
			t.Errorf("editor row %v = %q, want %q", row, line, want)
		}
	}

	// The hardware caret sits on the last row, column 57 (urwid golden).
	cx, cy, vis := screen.GetCursor()
	if !vis {
		t.Fatal("multiline editor caret is not visible")
	}
	if cx != 57 || cy != 2 {
		t.Errorf("caret = (%v,%v), want (57,2)", cx, cy)
	}

	// Full-row shift rule: a cursor at the end of a row that exactly fills
	// the width shifts that row's window left by one (urwid renders
	// "bcdefg " with the caret at column 6).
	shift := NewReadlineEdit(&killRing{}, "", "")
	shift.SetMultiline(true)
	shift.SetText("abcdefg")
	shift.SetCursorPos(7)
	shift.Focus(func(tview.Primitive) {})
	screen.SetSize(7, 2)
	shift.SetRect(0, 0, 7, 2)
	shift.Draw(screen)
	cx, cy, _ = screen.GetCursor()
	if cx != 6 || cy != 0 {
		t.Errorf("shifted caret = (%v,%v), want (6,0)", cx, cy)
	}
	var got strings.Builder
	for x := range 7 {
		ch, _, _ := screen.Get(x, 0)
		got.WriteString(ch)
	}
	if line := strings.TrimRight(got.String(), " "); line != "bcdefg" {
		t.Errorf("shifted row = %q, want %q (urwid shifts the row window)", line, "bcdefg")
	}
}

// TestReadlineEditSingleLineCursorClamped pins the fleet symptom's root: the
// single-line field's hardware caret must stay inside the field even when
// tview scrolls the text horizontally (the caret previously walked past the
// panel edge and off screen).
func TestReadlineEditSingleLineCursorClamped(t *testing.T) {
	t.Parallel()

	re := NewReadlineEdit(&killRing{}, "", "")
	long := strings.Repeat("x", 40)
	re.SetText(long)
	re.SetCursorPos(len(long))
	re.Focus(func(tview.Primitive) {})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(10, 1)
	re.SetRect(0, 0, 10, 1)
	re.Draw(screen)

	cx, _, vis := screen.GetCursor()
	if !vis {
		t.Fatal("single-line caret is not visible")
	}
	if cx < 0 || cx > 9 {
		t.Errorf("caret x = %v, want within [0,9] (was walking past the panel)", cx)
	}
}
