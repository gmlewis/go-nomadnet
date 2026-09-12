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
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// alignedFieldPage puts a field on a centered line before an inline link, and a
// sized field on a left-aligned line before plain text and another link (the
// `l resets the alignment the `c line set). Both shapes used to be measured from
// the raw span text, so the field's box, every part boundary after it, and the
// link it hides all landed in the wrong place.
const alignedFieldPage = "`cSearch: `B444`<name`>`b `[Go`/go]\n" +
	"`lPlain: `B444`<32|q`>`b tail `[Go2`/go2]"

// drawnRowText reads n cells of screen row y from column x0 (a blank cell reads
// as a space), i.e. what the page actually shows on that row.
func drawnRowText(t *testing.T, screen tcell.Screen, x0, y, n int) string {
	t.Helper()
	var row strings.Builder
	for i := range n {
		r, _, _, _ := cellContent(screen, x0+i, y)
		if r == 0 {
			r = ' '
		}
		row.WriteRune(r)
	}
	return row.String()
}

// TestBrowserFieldStartColMatchesDrawnRow pins that a field's start column is
// the column the renderer drew its box at, for both a `c`-aligned line and a
// left-aligned one: the column is derived from the rendered row (indent + the
// alignment pad for that row), not from the raw span texts. Measuring the raw
// text put the centered line's field 11 columns right of its box, so clicks,
// carets, and the editor overlay all missed it.
func TestBrowserFieldStartColMatchesDrawnRow(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, alignedFieldPage)

	for _, want := range []string{"Search:", "Plain:"} {
		line := findLine(bd, want)
		if line < 0 {
			t.Fatalf("line %q not found", want)
		}
		rf := bd.lineFields[line][0]
		screen, row := drawFieldScreen(t, bd, line)
		drawn := drawnRowText(t, screen, 0, row, 80)
		at := strings.Index(drawn, want)
		if at < 0 {
			t.Fatalf("row %d does not show %q: %q", row, want, drawn)
		}
		// The label sits before the field, so the box starts right after it.
		wantCol := at + len(want) + 1
		if rf.startCol != wantCol {
			t.Errorf("line %q: field startCol = %d, but the box is drawn at column %d (row %q)",
				want, rf.startCol, wantCol, strings.TrimRight(drawn, " "))
		}
		// The cursor model must agree with the same geometry: the field's first
		// rune offset maps back to the column the box starts at.
		bd.focusLine = line
		bd.lineCursors[line] = rf.runeStart
		if x, _, ok := bd.cursorScreenXY(); !ok || x != rf.startCol {
			t.Errorf("line %q: cursor at the field start = %d (ok=%v), want %d", want, x, ok, rf.startCol)
		}
	}
}

// TestBrowserFieldLinePartOffsetsUseRenderedWidth pins that part boundaries and
// link lookup walk the RENDERED line: a field that draws wider than its raw
// text (its padded box, or a checkbox's icon column) must not shift the parts
// after it. Previously a cursor anywhere in the six columns of plain text that
// follow a 32-column field resolved to the link that actually sits 32 columns
// further right — so Enter followed a link the visitor was not on, and the link
// itself could not be activated at all.
func TestBrowserFieldLinePartOffsetsUseRenderedWidth(t *testing.T) {
	t.Parallel()

	_, bd := newLoadedFieldTestBrowser(t, alignedFieldPage)
	line := findLine(bd, "Plain:")
	if line < 0 {
		t.Fatal("line \"Plain:\" not found")
	}
	plain := bd.linePlainText(line)
	if got, want := len([]rune(plain)), 48; got != want {
		t.Fatalf("rendered line width = %d runes, want %d (%q)", got, want, plain)
	}

	// "Plain: " + 32-column field + " tail " + "Go2".
	if got, want := bd.linePartPositions(line), []int{0, 7, 39, 45, 48}; !slices.Equal(got, want) {
		t.Errorf("part positions = %v, want %v", got, want)
	}

	tests := []struct {
		pos  int
		want string
	}{
		{pos: 0, want: ""},      // "Plain: "
		{pos: 7, want: ""},      // the field's box
		{pos: 13, want: ""},     // inside the box, where a raw-text offset used to hit the link
		{pos: 39, want: ""},     // " tail "
		{pos: 44, want: ""},     // the space before the link
		{pos: 45, want: "/go2"}, // the link
		{pos: 47, want: "/go2"}, // the link's last cell
	}
	for _, tt := range tests {
		bd.lineCursors[line] = tt.pos
		got := ""
		if link := bd.lineLinkAtCursor(line); link != nil {
			got = link.URL
		}
		if got != tt.want {
			t.Errorf("lineLinkAtCursor(%d) = %q, want %q", tt.pos, got, tt.want)
		}
	}

	// Enter on the link's rendered position follows it, with the field's value
	// collected into the request.
	var gotURL string
	bd.OnRetrieveURL = func(url string, requestData map[string]string) { gotURL = url }
	bd.focusLine = line
	bd.lineCursors[line] = 45
	bd.handleNavKey(key(tcell.KeyEnter, 0))
	if gotURL != "/go2" {
		t.Errorf("Enter on the link = %q, want /go2", gotURL)
	}

	// The centered line's link follows its field the same way.
	centered := findLine(bd, "Search:")
	if got, want := bd.linePartPositions(centered), []int{0, 8, 32, 33, 35}; !slices.Equal(got, want) {
		t.Errorf("centered part positions = %v, want %v", got, want)
	}
	bd.focusLine = centered
	bd.lineCursors[centered] = 33
	gotURL = ""
	bd.handleNavKey(key(tcell.KeyEnter, 0))
	if gotURL != "/go" {
		t.Errorf("Enter on the centered link = %q, want /go", gotURL)
	}
}

// TestBrowserFieldOnWrappedRow pins a field whose span starts on a wrapped row
// (the text before it fills the first rows): the field's box, its editor
// overlay, and the caret all belong to the row the span actually starts on, not
// to the line's first row. Placing them on the first row left the editor one row
// (or more) above the field, so typing appeared to land in unrelated text.
func TestBrowserFieldOnWrappedRow(t *testing.T) {
	t.Parallel()

	// 20 five-column words fill 100 columns, so the field starts on a later row
	// of the line at the 80-column content width.
	_, bd := newLoadedFieldTestBrowser(t, strings.Repeat("word ", 20)+"`B444`<name`>`b tail")
	line := 0
	if len(bd.lineFields[line]) != 1 {
		t.Fatalf("line %d has %v fields, want 1", line, len(bd.lineFields[line]))
	}
	rf := bd.lineFields[line][0]
	if rf.rowOffset == 0 {
		t.Fatalf("fixture field did not land on a wrapped row (startCol=%d)", rf.startCol)
	}
	if rows := bd.lineRowCount(line); rows <= rf.rowOffset {
		t.Fatalf("line has %d rows, want more than the field's row %d", rows, rf.rowOffset)
	}

	screen, firstRow := drawFieldScreen(t, bd, line)
	boxRow := firstRow + rf.rowOffset
	drawn := drawnRowText(t, screen, 0, boxRow, 80)
	if box := drawn[rf.startCol : rf.startCol+rf.width]; strings.TrimSpace(box) != "" {
		t.Errorf("field row %d box cells = %q, want blanks", boxRow, box)
	}
	if tail := strings.TrimSpace(drawn[rf.startCol+rf.width:]); tail != "tail" {
		t.Errorf("text after the field's box = %q, want %q", tail, "tail")
	}

	// A click inside the box on that row focuses the field and puts the caret on
	// the clicked cell (clamped to the empty text, i.e. the field's start).
	if !bd.clickFocus(rf.startCol+2, boxRow) {
		t.Fatal("click on the field's box did not focus the page")
	}
	if bd.fieldOverlay == nil {
		t.Fatal("click on the field did not mount its editor overlay")
	}
	if got, want := bd.lineCursors[line], rf.runeStart; got != want {
		t.Errorf("line cursor after the click = %d, want %d (the field's start)", got, want)
	}

	// The hardware cursor must land in the box on the field's own row.
	screen, _ = drawFieldScreen(t, bd, line)
	x, y, visible := screen.GetCursor()
	if !visible {
		t.Fatal("hardware cursor not visible after a click (stampKeypress must arm it)")
	}
	if x != rf.startCol || y != boxRow {
		t.Errorf("hardware cursor = (%d,%d), want (%d,%d) in the field's box", x, y, rf.startCol, boxRow)
	}
}
