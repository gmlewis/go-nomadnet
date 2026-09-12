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

// pageScreenRows draws a browser page the way browserPageView.Draw does — the
// page text, then the field overlays on top of it — and returns the screen rows
// as strings.
func pageScreenRows(t *testing.T, bd *BrowserDisplay, width, height int) []string {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("simulation screen init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	bd.content.SetRect(0, 0, width, height)
	bd.content.Draw(screen)
	bd.drawFieldOverlays(screen)

	rows := make([]string, height)
	for y := range height {
		var sb strings.Builder
		for x := range width {
			cell, _, _ := screen.Get(x, y)
			var r rune
			for _, ch := range cell {
				r = ch
				break
			}
			if r == 0 {
				r = ' '
			}
			sb.WriteRune(r)
		}
		rows[y] = sb.String()
	}
	return rows
}

// rowWith returns the first drawn row containing sub.
func rowWith(rows []string, sub string) string {
	for _, row := range rows {
		if strings.Contains(row, sub) {
			return row
		}
	}
	return ""
}

// TestFieldValueSurvivesMovingFocus pins what a visitor sees after filling in a
// field and moving to the next one: the value they typed stays visible in the
// field they left. Only the focused field's editor is mounted for input, but
// every field's editor owns what was typed into it, and a page that dropped the
// text the moment focus moved would look like it had thrown away the visitor's
// answer.
func TestFieldValueSurvivesMovingFocus(t *testing.T) {
	t.Parallel()

	p := newFieldPageProbe(t, guestbookTestPage)
	p.typeText("Glenn")

	nameLine := findLine(p.bd, "Name:")
	if nameLine < 0 {
		t.Fatal("no Name: line in the guestbook form")
	}
	if got := rowWith(pageScreenRows(t, p.bd, 80, 24), "Name:"); !strings.Contains(got, "Glenn") {
		t.Fatalf("name row while editing = %q, want the typed value drawn", got)
	}

	// Down keeps editing (on the next field) and leaves the name field's editor
	// with its text.
	if ev := p.dispatch(tcell.KeyDown, 0); ev != nil {
		t.Fatal("Down not consumed while a field is in edit mode")
	}
	if p.bd.fieldOverlay == nil {
		t.Fatal("Down left no field in edit mode")
	}
	if p.bd.fieldOverlay == p.bd.lineFields[nameLine][0].editor {
		t.Fatal("Down left the text field still focused, want the next field")
	}

	rows := pageScreenRows(t, p.bd, 80, 24)
	if got := rowWith(rows, "Name:"); !strings.Contains(got, "Glenn") {
		t.Errorf("name row after Down = %q, want it to still show the typed value", got)
	}
	messageLine := findLine(p.bd, "Message:")
	if messageLine < 0 {
		t.Fatal("no Message: line in the guestbook form")
	}
	if p.bd.fieldOverlay != p.bd.lineFields[messageLine][0].editor {
		t.Error("Down did not move edit mode to the message field")
	}
	if got := p.bd.lineFields[nameLine][0].editor.GetText(); got != "Glenn" {
		t.Errorf("name field text after Down = %q, want %q", got, "Glenn")
	}
}

// TestFieldValueSurvivesLeavingForTheSubmitLink pins the same persistence when
// the visitor tabs past the last field onto the submit link: the whole form
// stays on the page, ready to be submitted.
func TestFieldValueSurvivesLeavingForTheSubmitLink(t *testing.T) {
	t.Parallel()

	p := newFieldPageProbe(t, guestbookTestPage)
	p.typeText("Glenn")
	p.dispatch(tcell.KeyDown, 0)
	p.typeText("hi there")

	// Tab from the message field leaves the fields for the submit link.
	if ev := p.dispatch(tcell.KeyTab, 0); ev != nil {
		t.Fatal("Tab not consumed while a field is in edit mode")
	}

	rows := pageScreenRows(t, p.bd, 80, 24)
	if got := rowWith(rows, "Name:"); !strings.Contains(got, "Glenn") {
		t.Errorf("name row = %q, want the typed value", got)
	}
	if got := rowWith(rows, "Message:"); !strings.Contains(got, "hi there") {
		t.Errorf("message row = %q, want the typed value", got)
	}
	if fields := p.bd.collectFields("name|message"); fields["field_name"] != "Glenn" || fields["field_message"] != "hi there" {
		t.Errorf("collectFields = %v, want both typed values", fields)
	}
}

// TestUnfocusedFieldValueCoversThePlaceholder pins that a field's value drawn
// over the page text hides whatever the page rendered in its cells, so an
// untouched pre-filled field is not drawn twice.
func TestUnfocusedFieldValueCoversThePlaceholder(t *testing.T) {
	t.Parallel()

	page := "Name: `B444`<name`Glenn`>b and `B444`<other`>`b\n"
	p := newFieldPageProbe(t, page)

	rows := pageScreenRows(t, p.bd, 80, 24)
	row := rowWith(rows, "Name:")
	if got := strings.Count(row, "Glenn"); got != 1 {
		t.Errorf("row %q shows the pre-filled value %v time(s), want exactly 1", row, got)
	}
}
