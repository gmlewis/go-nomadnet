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

// fieldFootprintPage is a labelled, empty, background-highlighted field followed
// by text on the same line: "Name: " (6 cells), a 24-cell field, then "|end".
// The markup is the guide's recommended form for a visible empty field
// (tui/guidetopics/markup.mu "Fields": label text, an empty field, and a
// background color tag to make the field visible).
const fieldFootprintPage = "Name: `B444`<name`>`b|end\n\n`[Go`/x]\n"

// TestBrowserFieldFootprintReservesWidth pins a Micron text field's footprint to
// its declared width. Python builds every field as an urwid.Columns child of
// `field_width` cells (MicronParser.py:358-376), so the Edit's AttrMap
// background paints the whole box and anything following the field on the same
// line starts past it:
//
//	Name: `<field`>
//
// renders as 6 cells of label, 24 cells of field, then whatever follows — the
// field's box is visible whether or not it holds pre-defined text, and whether
// or not it currently has the focus. The Go port advanced the line by the
// field's *text* width instead, so an empty field occupied zero cells and the
// mounted editor overlay (24 cells wide) painted over the text after it.
func TestBrowserFieldFootprintReservesWidth(t *testing.T) {
	t.Parallel()

	const labelW = 6  // "Name: "
	const fieldW = 24 // the field's declared footprint
	wantBG := parseColor("#444444")

	for _, tc := range []struct {
		name string
		// down presses Down first, moving the page cursor off the field line so
		// the editor overlay is unmounted and only the page text layer paints.
		down bool
	}{
		{name: "field focused (editor overlay mounted)"},
		{name: "field not focused (page text layer only)", down: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, bd := newLoadedFieldTestBrowser(t, fieldFootprintPage)
			line := findLine(bd, "Name:")
			if line < 0 {
				t.Fatal("field line not found")
			}
			if tc.down {
				bd.handleInput(key(tcell.KeyDown, 0))
				bd.handleInput(key(tcell.KeyDown, 0))
				if bd.focusLine == line {
					t.Fatal("focus stayed on the field line; overlay not unmounted")
				}
			}
			rf := bd.lineFields[line][0]
			if rf.startCol != labelW {
				t.Fatalf("field startCol = %v, want %v", rf.startCol, labelW)
			}
			if rf.width != fieldW {
				t.Fatalf("field width = %v, want %v", rf.width, fieldW)
			}

			screen, row := drawFieldScreen(t, bd, line)
			for i := range fieldW {
				_, _, style, _ := cellContent(screen, rf.startCol+i, row)
				if _, bg, _ := style.Decompose(); bg != wantBG {
					t.Fatalf("field cell %v: background = %v, want %v (#444444): the field must occupy its declared width",
						i, bg, wantBG)
				}
			}
			if c, _, _, _ := cellContent(screen, rf.startCol+fieldW, row); c != '|' {
				t.Errorf("cell after the field = %q, want '|': text following a field must start past its width, not immediately after its text",
					string(c))
			}
		})
	}
}
