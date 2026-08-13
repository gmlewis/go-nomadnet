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

// fieldTestPage is a micron page with a labeled text field and a submit link
// that names the field. In micron a field is entered from formatting mode with
// the `< prefix (backtick enters formatting mode, then '<' triggers field
// parsing); a bare '<' in text mode is a literal character.
//
//	>Search Form                         (line 0: heading)
//	Search: `<query`initial>             (line 1: label "Search: " + text field "query")
//	`[Search`/page/search.mu`query]      (line 2: submit link, Fields="query")
const fieldTestPage = ">Search Form\nSearch: `<query`initial>\n`[Search`/page/search.mu`query]"

// checkboxTestPage exercises checkbox + radio fields and a submit link:
//
//	>Prefs                               (line 0: heading)
//	`<?|opt1|val1`Option 1>              (line 1: checkbox "opt1", value "val1")
//	`<?|opt2`Opt Two>                    (line 2: checkbox "opt2", no value → "1")
//	`<^|choice|a`Choice A>               (line 3: radio "choice", value "a")
//	`[Submit`/save`opt1|opt2|choice]     (line 4: submit link)
const checkboxTestPage = ">Prefs\n`<?|opt1|val1`Option 1>\n`<?|opt2`Opt Two>\n`<^|choice|a`Choice A>\n`[Submit`/save`opt1|opt2|choice]"

// newFieldTestBrowser renders a micron page and focuses the content body.
func newFieldTestBrowser(t *testing.T, page string) (*App, *BrowserDisplay) {
	t.Helper()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = page
	bd.renderPage()
	bd.content.SetRect(0, 0, 80, 6)
	app.SetFocus(bd.content)
	return app, bd
}

// TestBrowserFieldRender builds lineFields for a text field and mounts the
// ReadlineEdit overlay when its row is focused; typing into the overlay updates
// the editor text that collectFields reads.
func TestBrowserFieldRender(t *testing.T) {
	t.Parallel()
	_, bd := newFieldTestBrowser(t, fieldTestPage)

	fieldLine := findLine(bd, "Search:")
	if fieldLine < 0 {
		t.Fatal("field line not found")
	}
	if len(bd.lineFields[fieldLine]) != 1 {
		t.Fatalf("lineFields[%d] = %d fields, want 1", fieldLine, len(bd.lineFields[fieldLine]))
	}
	rf := bd.lineFields[fieldLine][0]
	if rf.editor == nil {
		t.Fatal("text field has no editor overlay")
	}
	if got := rf.editor.GetText(); got != "initial" {
		t.Errorf("field initial text = %q, want %q", got, "initial")
	}
	if rf.spec.Name != "query" {
		t.Errorf("field name = %q, want %q", rf.spec.Name, "query")
	}

	// Down onto the field line mounts the overlay (Python Columns focuses the
	// Edit, skipping the non-selectable label).
	if bd.handleInput(key(tcell.KeyDown, 0)) != nil {
		t.Error("Down not consumed")
	}
	if bd.focusLine != fieldLine {
		t.Fatalf("focusLine = %d, want %d", bd.focusLine, fieldLine)
	}
	if bd.fieldOverlay == nil || bd.fieldOverlayLine != fieldLine {
		t.Error("text-field overlay not mounted on the field line")
	}

	// Typing into the overlay fills the editor.
	bd.fieldOverlay.SetText("")
	bd.fieldOverlay.handleKey(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModNone))
	bd.fieldOverlay.handleKey(tcell.NewEventKey(tcell.KeyRune, 'i', tcell.ModNone))
	if got := bd.fieldOverlay.GetText(); got != "hi" {
		t.Errorf("editor text after typing = %q, want %q", got, "hi")
	}
}

// TestBrowserHandleLinkCollectsFields verifies a submit link collects live field
// values into request_data and forwards them to OnRetrieveURL (Python recurse_down).
func TestBrowserHandleLinkCollectsFields(t *testing.T) {
	t.Parallel()
	_, bd := newFieldTestBrowser(t, fieldTestPage)

	// Move to the field line + simulate typed text.
	bd.handleInput(key(tcell.KeyDown, 0)) // → field line, overlay mounted
	if bd.fieldOverlay == nil {
		t.Fatal("overlay not mounted")
	}
	bd.fieldOverlay.SetText("hello world")

	var gotURL string
	var gotRD map[string]string
	bd.OnRetrieveURL = func(url string, requestData map[string]string) {
		gotURL = url
		gotRD = requestData
	}

	// Submit link naming the field → collects field_query.
	bd.HandleLink("/page/search.mu", "query")
	if gotURL != "/page/search.mu" {
		t.Errorf("OnRetrieveURL url = %q, want /page/search.mu", gotURL)
	}
	if gotRD == nil || gotRD["field_query"] != "hello world" {
		t.Errorf("request_data = %v, want field_query=%q", gotRD, "hello world")
	}

	// "*" collects every field.
	bd.HandleLink("/page/search.mu", "*")
	if gotRD == nil || gotRD["field_query"] != "hello world" {
		t.Errorf("request_data (*) = %v, want field_query=%q", gotRD, "hello world")
	}

	// A plain link (no fields) → nil request data (cache path).
	bd.HandleLink("/page/search.mu", "")
	if gotRD != nil {
		t.Errorf("plain link request_data = %v, want nil", gotRD)
	}
}

// TestBrowserFieldOverlayExit verifies Down while editing leaves the field,
// moves focus to the next line, and unmounts the overlay.
func TestBrowserFieldOverlayExit(t *testing.T) {
	t.Parallel()
	_, bd := newFieldTestBrowser(t, fieldTestPage)

	// The field is on the "Search:" line; the submit link is the next line.
	fieldLine := findLine(bd, "Search:")
	linkLine := fieldLine + 1
	bd.handleInput(key(tcell.KeyDown, 0)) // → field line, overlay mounted
	if bd.fieldOverlay == nil {
		t.Fatal("overlay not mounted")
	}

	// Down while editing → overlay's onExit moves Pile focus off the field.
	bd.fieldOverlay.handleKey(key(tcell.KeyDown, 0))
	if bd.fieldOverlay != nil {
		t.Error("overlay still mounted after Down")
	}
	if bd.focusLine != linkLine {
		t.Errorf("focusLine after Down = %d, want %d", bd.focusLine, linkLine)
	}
}

// TestBrowserCheckboxRadioCollect verifies checkbox/radio toggle on Space and
// field_value collection (radio: only checked; checkbox: default "1"; multiple
// same-name concatenate).
func TestBrowserCheckboxRadioCollect(t *testing.T) {
	t.Parallel()
	_, bd := newFieldTestBrowser(t, checkboxTestPage)

	// Line 1: checkbox opt1 (value val1). focusLine starts at 0 (heading).
	bd.handleInput(key(tcell.KeyDown, 0)) // → opt1 line
	if bd.fieldOverlay != nil {
		t.Error("checkbox line should not mount a text overlay")
	}
	bd.handleInput(key(tcell.KeyRune, ' ')) // toggle opt1
	if !bd.lineFields[bd.focusLine][0].checkbox.IsChecked() {
		t.Error("opt1 checkbox not toggled on")
	}

	// Line 3: radio choice (value a). Down past opt2 (line 2) to choice (line 3).
	bd.handleInput(key(tcell.KeyDown, 0))   // → opt2 line
	bd.handleInput(key(tcell.KeyDown, 0))   // → choice line
	bd.handleInput(key(tcell.KeyRune, ' ')) // toggle radio
	if !bd.lineFields[bd.focusLine][0].checkbox.IsChecked() {
		t.Error("choice radio not toggled on")
	}

	rd := bd.collectFields("opt1|opt2|choice")
	if rd == nil {
		t.Fatal("collectFields returned nil")
	}
	if rd["field_opt1"] != "val1" {
		t.Errorf("field_opt1 = %q, want val1", rd["field_opt1"])
	}
	if rd["field_choice"] != "a" {
		t.Errorf("field_choice = %q, want a", rd["field_choice"])
	}
	// opt2 is unchecked → absent (checkbox only contributes when checked).
	if _, ok := rd["field_opt2"]; ok {
		t.Error("unchecked opt2 should not contribute")
	}
}
