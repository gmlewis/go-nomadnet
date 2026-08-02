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
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// drawReadlineEditFocused renders re focused on a fresh 1-row simulation
// screen of width w and returns the resulting hardware cursor (x, y, visible).
func drawReadlineEditFocused(t *testing.T, re *ReadlineEdit, w int) (int, int, bool) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, 1)
	re.SetRect(0, 0, w, 1)
	// Mark the field focused so its Draw shows the caret (mirrors tview
	// Application focus: Box.Focus sets hasFocus, and InputField.Draw forwards
	// HasFocus to the inner text area).
	re.InputField.Focus(func(p tview.Primitive) {}) //nolint:errcheck
	re.Draw(screen)
	screen.Sync()
	x, y, vis := screen.GetCursor()
	return x, y, vis
}

// TestReadlineEditCaretTracksCursor pins the Phase-0 cursor-parity behavior for
// ReadlineEdit: the terminal hardware caret must sit at the model cursorPos,
// not at the end of the buffer. tview's InputField exposes no public cursor
// setter (its internal cursor stays at the end after SetText), so ReadlineEdit
// repositions the hardware cursor itself on draw. tmux capture cannot see the
// cursor, so this is verified against a SimulationScreen's GetCursor instead.
func TestReadlineEditCaretTracksCursor(t *testing.T) {
	t.Parallel()

	label := "Name: "
	labelW := runewidth.StringWidth(label)
	re := NewReadlineEdit(&killRing{}, label, "")
	re.SetText("hello") // model cursor at end (5)

	// Non-end cursor move to the middle (offset 2).
	re.SetCursorPos(2)
	x, y, vis := drawReadlineEditFocused(t, re, 40)
	if !vis {
		t.Fatal("caret not visible after SetCursorPos(2)")
	}
	wantX := labelW + 2
	if x != wantX || y != 0 {
		t.Errorf("caret at SetCursorPos(2) = (%d,%d), want (%d,0)", x, y, wantX)
	}

	// Move to the very beginning (Ctrl-A equivalent): caret at label end.
	re.SetCursorPos(0)
	x, _, vis = drawReadlineEditFocused(t, re, 40)
	if !vis {
		t.Fatal("caret not visible after SetCursorPos(0)")
	}
	if x != labelW {
		t.Errorf("caret at SetCursorPos(0) = %d, want %d (label width)", x, labelW)
	}

	// End position still correct.
	re.SetCursorPos(5)
	x, _, _ = drawReadlineEditFocused(t, re, 40)
	if x != labelW+5 {
		t.Errorf("caret at SetCursorPos(5) = %d, want %d", x, labelW+5)
	}
}

// TestReadlineEditCaretHiddenWhenUnfocused ensures the caret is NOT shown when
// the field is not focused (the hardware cursor should only track the model
// cursor on the focused input).
func TestReadlineEditCaretHiddenWhenUnfocused(t *testing.T) {
	t.Parallel()
	re := NewReadlineEdit(&killRing{}, "Name: ", "")
	re.SetText("hello")
	re.SetCursorPos(2)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(40, 1)
	re.SetRect(0, 0, 40, 1)
	// Deliberately do NOT focus.
	re.Draw(screen)
	screen.Sync()
	_, _, vis := screen.GetCursor()
	if vis {
		t.Error("caret visible on an unfocused ReadlineEdit; want hidden")
	}
}