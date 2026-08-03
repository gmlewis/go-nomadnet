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

// renderNewConversationDialog opens the New Conversation dialog over a
// ConversationsDisplay and renders it on an 80x24 simulation screen, returning
// the joined cell text per row. With showError=true the error-row variant is
// rendered directly (the state a failed Create re-shows).
func renderNewConversationDialog(t *testing.T, showError bool) []string {
	t.Helper()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	pages := app.Dialogs.Init(app.Application, cd.Widget())
	app.Application.SetRoot(pages, true)
	app.Application.SetFocus(cd.Widget())

	onCreate := func(addrHex, name, trust string) bool { return true }
	if showError {
		cd.showNewConversationDialog("zz", "", true, onCreate)
	} else {
		cd.ShowNewConversationDialog(onCreate)
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	// tview primitives do not self-size: the Pages root must be given its
	// rect before Draw, exactly as Application.Draw does via ResizeSensor.
	pages.SetRect(0, 0, 80, 24)
	pages.Draw(screen)
	screen.Sync()

	rows := make([]string, 24)
	for y := 0; y < 24; y++ {
		var b strings.Builder
		for x := 0; x < 80; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// containsRow reports whether any rendered row contains the given substring.
func containsRow(rows []string, substr string) bool {
	for _, r := range rows {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

// TestNewConversationDialogLayout pins the rendered layout of the New
// Conversation dialog against the Python ground truth (Conversations.py:1024-
// 1120): Addr/Name fields, the two-checked radio quirk, flat Create/Back
// buttons, the "New Conversation" title, and a 48-wide bordered dialog.
func TestNewConversationDialogLayout(t *testing.T) {
	t.Parallel()
	rows := renderNewConversationDialog(t, false)

	if !containsRow(rows, "New Conversation") {
		t.Errorf("dialog title %q missing from render", "New Conversation")
	}
	if !containsRow(rows, "Addr : ") {
		t.Errorf("Addr field label missing from render")
	}
	if !containsRow(rows, "Name : ") {
		t.Errorf("Name field label missing from render")
	}
	// urwid RadioButton construction quirk: both Untrusted and Unknown show
	// "(X)" on open, Trusted shows "( )".
	if !containsRow(rows, "(X) Untrusted") {
		t.Errorf("(X) Untrusted missing — construction quirk not reproduced")
	}
	if !containsRow(rows, "(X) Unknown") {
		t.Errorf("(X) Unknown missing — construction quirk not reproduced")
	}
	if !containsRow(rows, "( ) Trusted") {
		t.Errorf("( ) Trusted missing")
	}
	// Flat urwid buttons "< Create >" / "< Back >" (NO bordered box).
	if !containsRow(rows, "< Create") {
		t.Errorf("< Create button missing")
	}
	if !containsRow(rows, "< Back") {
		t.Errorf("< Back button missing")
	}

	// The bordered dialog is 48 wide (inner content 46). Anchor on the
	// title row (the dialog's top border) so the outer ConversationsDisplay
	// border is not mistaken for the dialog, and measure in rune columns.
	dialogTop := -1
	for i, r := range rows {
		if strings.Contains(r, "New Conversation") {
			dialogTop = i
			break
		}
	}
	if dialogTop < 0 {
		t.Fatal("dialog title row not found")
	}
	runes := []rune(rows[dialogTop])
	left, right := -1, -1
	for i, r := range runes {
		if r == '┌' {
			left = i
		}
		if r == '┐' && right < 0 {
			right = i
		}
	}
	if left < 0 || right < 0 {
		t.Fatalf("dialog top-border corners not found (left=%v right=%v)", left, right)
	}
	if got := right - left + 1; got != 48 {
		t.Errorf("dialog top border width = %v, want 48", got)
	}
}

// TestNewConversationDialogErrorRow verifies the error variant renders the
// centered "Could not start conversation. Check your input." text (the state a
// failed Create re-shows), matching the original's error_text append.
func TestNewConversationDialogErrorRow(t *testing.T) {
	t.Parallel()
	rows := renderNewConversationDialog(t, true)
	if !containsRow(rows, "Could not start conversation") {
		t.Errorf("error text missing from error variant render")
	}
	if !containsRow(rows, "input.") {
		t.Errorf("error text second line %q missing (must wrap to 2 rows)", "input.")
	}
	// The error variant is 13 rows tall (11 content + 2 border) vs 10 for the
	// plain variant. Anchor on the dialog's own title row (not the outer
	// ConversationsDisplay border) and find the bottom border below it.
	top, bottom := -1, -1
	for i, r := range rows {
		if strings.Contains(r, "New Conversation") {
			top = i
			break
		}
	}
	if top < 0 {
		t.Fatal("dialog title row not found")
	}
	for i := top + 1; i < len(rows); i++ {
		if strings.Contains(rows[i], "└") {
			bottom = i
			break
		}
	}
	if bottom < 0 {
		t.Fatalf("dialog bottom border not found (top=%v)", top)
	}
	if got := bottom - top + 1; got != 13 {
		t.Errorf("error dialog height = %v rows, want 13", got)
	}
}
