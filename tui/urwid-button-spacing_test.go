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

	"github.com/gmlewis/tcell/v2"
)

// TestUrwidButtonRenderingMatchesPython verifies that the UrwidButton renders
// with the same spacing as Python's urwid.Button: one blank after the left
// bracket, NO blank before the right bracket. Expected renderings:
//
//	Cancel at width 20: '< Cancel           >'
//	Go     at width 20: '< Go               >'
func TestUrwidButtonRenderingMatchesPython(t *testing.T) {
	t.Parallel()

	cases := []struct {
		label string
		width int
		want  string
	}{
		{"Cancel", 20, "< Cancel           >"},
		{"Go", 20, "< Go               >"},
	}

	for _, c := range cases {
		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatalf("screen.Init: %v", err)
		}
		screen.SetSize(c.width, 1)

		btn := NewUrwidButton(c.label)
		btn.SetRect(0, 0, c.width, 1)
		btn.Draw(screen)
		screen.Show()

		cells, w, _ := screen.GetContents()
		var row strings.Builder
		for i := range w {
			if len(cells[i].Runes) > 0 {
				row.WriteRune(cells[i].Runes[0])
			} else {
				row.WriteByte(' ')
			}
		}
		got := row.String()
		screen.Fini()

		if got != c.want {
			t.Errorf("UrwidButton(%q) at width %v = %q, want %q",
				c.label, c.width, got, c.want)
		}
	}
}

// TestUrwidButtonMultiByteLabelFillsCells pins that a multi-byte label is
// measured in cells, not bytes, so the leftover columns are padding BEFORE the
// closing bracket and the right bracket sits at the last cell. With the ✉
// glyph set the label is one cell narrower, so the inner pad is four cells at
// this width.
func TestUrwidButtonMultiByteLabelFillsCells(t *testing.T) {
	t.Parallel()

	const width = 24
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, 1)

	btn := NewTabButton("Untrusted (1) ✉ 1")
	btn.SetRect(0, 0, width, 1)
	btn.Draw(screen)
	screen.Show()

	cells, w, _ := screen.GetContents()
	var row strings.Builder
	for i := range w {
		if len(cells[i].Runes) > 0 {
			row.WriteRune(cells[i].Runes[0])
		} else {
			row.WriteByte(' ')
		}
	}
	got := row.String()
	const want = "[ Untrusted (1) ✉ 1    ]"
	if got != want {
		t.Errorf("tab button with multi-byte glyph at width %v = %q, want %q", width, got, want)
	}
	// The right bracket must sit at the LAST cell (brackets are not floating).
	if last := rune(cells[width-1].Runes[0]); last != ']' {
		t.Errorf("last cell = %q, want ']'", string(last))
	}
}
