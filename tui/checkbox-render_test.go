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

// checkboxStatePage is a form with one unchecked checkbox, one prechecked
// checkbox, a radio group of two options (the first prechecked), a text field,
// and a submit link naming every field.
const checkboxStatePage = ">Prefs\n" +
	"`<?|opt1|val1`Option 1>\n" +
	"`<?|opt2|val2|*`Option 2>\n" +
	"`<^|choice|a`Choice A>\n" +
	"`<^|choice|b|*`Choice B>\n" +
	"Note: `B444`<20|note`>`b\n" +
	"`[Save`/save`opt1|opt2|choice|note]"

// fieldRowText reads the cells a field occupies on its line: the state glyph
// column plus the label, as the screen actually shows them.
func fieldRowText(t *testing.T, bd *BrowserDisplay, screen tcell.Screen, line int, rf *renderedField, n int) string {
	t.Helper()
	_, y0, _, _ := bd.content.GetInnerRect()
	scrollRow, _ := bd.content.GetScrollOffset()
	y := y0 + (bd.rowsAbove(line) - scrollRow)
	x0, _, _, _ := bd.content.GetInnerRect()
	runes := make([]rune, 0, n)
	for i := range n {
		r, _, _, _ := cellContent(screen, x0+rf.startCol+i, y)
		if r == 0 {
			r = ' '
		}
		runes = append(runes, r)
	}
	return string(runes)
}

// drawCheckboxScreen draws the page body at a pane wide enough for the fixture
// and returns the drawn screen.
func drawCheckboxScreen(t *testing.T, bd *BrowserDisplay) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(80, 10)
	bd.content.SetRect(0, 0, 80, 10)
	bd.content.Draw(screen)
	return screen
}

// TestBrowserCheckboxRadioRenderStateGlyph pins that checkbox and radio fields
// draw the state glyph urwid draws, in the four-cell icon column urwid reserves
// before the label (urwid/widget/wimp.py:146-151, 465-470): "[ ] ", "[X] ",
// "( ) ", "(X) ". The Go port rendered only the bare label, so a visitor could
// not see which options were set — and no state at all when a form's markup
// prechecked one.
func TestBrowserCheckboxRadioRenderStateGlyph(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, checkboxStatePage)
	screen := drawCheckboxScreen(t, bd)

	tests := []struct {
		name  string
		label string
		want  string
	}{
		{name: "unchecked checkbox", label: "Option 1", want: "[ ] Option 1"},
		{name: "prechecked checkbox", label: "Option 2", want: "[X] Option 2"},
		{name: "unchecked radio", label: "Choice A", want: "( ) Choice A"},
		{name: "prechecked radio", label: "Choice B", want: "(X) Choice B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := findLine(bd, tt.label)
			if line < 0 {
				t.Fatalf("line %q not found", tt.label)
			}
			if len(bd.lineFields[line]) != 1 {
				t.Fatalf("line %q has %v fields, want 1", tt.label, len(bd.lineFields[line]))
			}
			rf := bd.lineFields[line][0]
			if got := fieldRowText(t, bd, screen, line, rf, len(tt.want)); got != tt.want {
				t.Errorf("field line renders %q, want %q", got, tt.want)
			}
			// The glyph is part of the line's own text, so the cursor model,
			// click model, and wrapped widths all agree with what is drawn.
			if got := bd.linePlainText(line); got != tt.want {
				t.Errorf("line plain text = %q, want %q", got, tt.want)
			}
		})
	}

	// A text field on the same page keeps its own rendering: no glyph, and the
	// label still starts where the span does.
	noteLine := findLine(bd, "Note:")
	if got, want := bd.linePlainText(noteLine)[:len("Note: ")], "Note: "; got != want {
		t.Errorf("text-field line starts %q, want %q", got, want)
	}
	if got := fieldRowText(t, bd, screen, noteLine, bd.lineFields[noteLine][0], 1); got != " " {
		t.Errorf("empty text field cell = %q, want a blank", got)
	}
}

// TestBrowserCheckboxRadioToggleUpdatesGlyph pins the other half: the glyph is
// redrawn from the live widget state, so toggling a field (Space/Enter on its
// line) changes what the visitor sees, not just what the form submits. urwid
// rebuilds the CheckBox's Columns with the new state icon on every change
// (wimp.py:385).
func TestBrowserCheckboxRadioToggleUpdatesGlyph(t *testing.T) {
	t.Parallel()

	_, bd := newFieldTestBrowser(t, checkboxStatePage)

	opt1 := findLine(bd, "Option 1")
	bd.focusLine = opt1
	bd.lineCursors[opt1] = bd.lineFields[opt1][0].runeStart
	if !bd.toggleFieldAtCursor() {
		t.Fatal("Space/Enter on the checkbox line did not toggle the field")
	}
	if got, want := bd.collectFields("opt1")["field_opt1"], "val1"; got != want {
		t.Errorf("submitted value = %q, want %q", got, want)
	}
	screen := drawCheckboxScreen(t, bd)
	rf := bd.lineFields[opt1][0]
	if got, want := fieldRowText(t, bd, screen, opt1, rf, len("[X] Option 1")), "[X] Option 1"; got != want {
		t.Errorf("toggled checkbox renders %q, want %q", got, want)
	}

	// A radio group keeps its mutual exclusion visible: selecting Choice A
	// clears the prechecked Choice B.
	choiceA := findLine(bd, "Choice A")
	bd.focusLine = choiceA
	bd.lineCursors[choiceA] = bd.lineFields[choiceA][0].runeStart
	if !bd.toggleFieldAtCursor() {
		t.Fatal("Space/Enter on the radio line did not toggle the field")
	}
	screen = drawCheckboxScreen(t, bd)
	if got, want := fieldRowText(t, bd, screen, choiceA, bd.lineFields[choiceA][0], len("(X) Choice A")), "(X) Choice A"; got != want {
		t.Errorf("selected radio renders %q, want %q", got, want)
	}
	choiceB := findLine(bd, "Choice B")
	if got, want := fieldRowText(t, bd, screen, choiceB, bd.lineFields[choiceB][0], len("( ) Choice B")), "( ) Choice B"; got != want {
		t.Errorf("radio group sibling renders %q, want %q", got, want)
	}
}
