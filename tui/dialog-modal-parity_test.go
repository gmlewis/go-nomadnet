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

// renderDialogToScreen builds a Python-style confirm dialog (DialogLineBox
// wrapping a Pile of a leading blank line + a centered message + a flat button
// row) and renders it to a simulation screen, returning the per-row text and a
// style check. It mirrors Python's confirm-dialog structure
// (Conversations.py:797-810): DialogLineBox(Pile([Text(""), Text(msg, CENTER),
// Columns([Button(0.45), Text(0.10), Button(0.45)])]), title).
func renderDialogToScreen(t *testing.T, title, msg string, btnLabels ...string) ([]string, func(x, y int) tcell.Style) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	const w = 60
	screen.SetSize(w, 8)

	btns := make([]*UrwidButton, len(btnLabels))
	for i, l := range btnLabels {
		btns[i] = NewUrwidButton(l)
	}
	buttonRow := CreateUrwidButtonRow(btns...)
	msgView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorDefault).
		SetText(msg)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText(""), 1, 0, false). // Python Text("") leading blank
		AddItem(msgView, 2, 0, false).
		AddItem(buttonRow, 1, 0, true)

	d := NewDialogLineBox(title, layout, nil)
	// The modal (DialogManager centerDialog) uses border-outside: the box rect
	// is the content area and the border sits one cell outside. SetRect at
	// (1,1) with a 58x4 content rect so the 60-wide border lands at row 0/col 0.
	d.SetRect(1, 1, w-2, 4)
	screen.Clear()
	d.Draw(screen)
	screen.Sync()

	rows := make([]string, 6)
	for y := range 6 {
		var b strings.Builder
		for x := range w {
			b.WriteString(cellString(screen, x, y))
		}
		rows[y] = b.String()
	}
	styleAt := func(x, y int) tcell.Style {
		_, st, _ := screen.Get(x, y)
		return st
	}
	return rows, styleAt
}

// TestModalRenderingMatchesPython pins the modal rendering to match nomadnet's
// urwid LineBox + flat-button dialogs. Python renders the dialog entirely in the
// DEFAULT style (urwid canvas attrs are None): light box-drawing border
// (┌─┐│└─┘), centered " title " on the top border, centered message, and flat
// "< label >" urwid buttons — NO forced colors and NO tview bordered/colored
// buttons. This test reproduces a Python confirm dialog
// (urwid.LineBox(Pile([Text(""), Text("Block peer?\n", CENTER), Columns([Button
// "Yes, block", Text(""), Button "Cancel"])]), title="Confirm block")) and
// verifies the Go render matches that shape and uses only the default style.
func TestModalRenderingMatchesPython(t *testing.T) {
	t.Parallel()
	rows, styleAt := renderDialogToScreen(t, "Confirm block", "Block peer?\n", "Yes, block", "Cancel")

	// Top border: ┌, ─ fill, " Confirm block ", ─ fill, ┐.
	top := rows[0]
	if !strings.HasPrefix(top, "┌") || !strings.HasSuffix(top, "┐") {
		t.Errorf("top border = %q, want ┌...┐", top)
	}
	if !strings.Contains(top, " Confirm block ") {
		t.Errorf("top border %q missing centered title \" Confirm block \"", top)
	}
	for _, r := range top {
		if r != '┌' && r != '┐' && r != '─' && r != ' ' && !(r >= 'A' && r <= 'z') {
			t.Errorf("top border has unexpected rune %q in %q", r, top)
			break
		}
	}

	// Bottom border: └, ─ fill, ┘.
	bottom := rows[5]
	if !strings.HasPrefix(bottom, "└") || !strings.HasSuffix(bottom, "┘") {
		t.Errorf("bottom border = %q, want └...┘", bottom)
	}

	// Side borders on the content rows.
	for _, y := range []int{1, 2, 3, 4} {
		rr := []rune(rows[y])
		if string(rr[0]) != "│" {
			t.Errorf("row %d left border = %q, want │", y, string(rr[0]))
		}
		if string(rr[len(rr)-1]) != "│" {
			t.Errorf("row %d right border = %q, want │", y, string(rr[len(rr)-1]))
		}
	}

	// Centered message on row 2.
	if !strings.Contains(rows[2], "Block peer?") {
		t.Errorf("row 2 = %q, missing centered message \"Block peer?\"", rows[2])
	}

	// Flat urwid buttons on row 4: "< Yes, block ... >" and "< Cancel ... >".
	if !strings.Contains(rows[4], "< Yes, block") {
		t.Errorf("row 4 = %q, missing flat button \"< Yes, block\"", rows[4])
	}
	if !strings.Contains(rows[4], "< Cancel") {
		t.Errorf("row 4 = %q, missing flat button \"< Cancel\"", rows[4])
	}
	if !strings.Contains(rows[4], ">") {
		t.Errorf("row 4 = %q, missing \">\" button bracket", rows[4])
	}

	// EVERY cell must be the default style (default fg, default bg) — Python's
	// urwid LineBox/Button/Text emit attr=None. No forced color (e.g. the former
	// 0xdddddd border/text or tview.Button's black/green) may appear.
	for y := range 6 {
		for x := range 60 {
			fg, bg, attrs := styleAt(x, y).Decompose()
			if fg != tcell.ColorDefault {
				t.Errorf("cell (%d,%d) fg = %v, want ColorDefault (Python dialog is default-style)", x, y, fg)
				return
			}
			if bg != tcell.ColorDefault {
				t.Errorf("cell (%d,%d) bg = %v, want ColorDefault", x, y, bg)
				return
			}
			if attrs&tcell.AttrBold != 0 || attrs&tcell.AttrReverse != 0 {
				t.Errorf("cell (%d,%d) has attr %v, want none", x, y, attrs)
				return
			}
		}
	}
}

// TestModalBorderIsLightBoxChars verifies the DialogLineBox border uses urwid's
// default light box-drawing glyphs (┌─┐│└─┘), matching urwid.LineBox.
func TestModalBorderIsLightBoxChars(t *testing.T) {
	t.Parallel()
	rows, _ := renderDialogToScreen(t, "T", "m", "Yes", "No")
	want := map[rune]bool{'┌': true, '┐': true, '└': true, '┘': true, '─': true, '│': true}
	topR := []rune(rows[0])
	botR := []rune(rows[5])
	if !want[topR[0]] || !want[topR[len(topR)-1]] {
		t.Errorf("top border corners not light box chars: %q", rows[0])
	}
	if !want[botR[0]] || !want[botR[len(botR)-1]] {
		t.Errorf("bottom border corners not light box chars: %q", rows[5])
	}
}
