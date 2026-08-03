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
	"github.com/rivo/tview"
)

// Golden border runes captured from the Python source-of-truth:
//   - single-line set: urwid/widget/constants.py:516 (BOX_SYMBOLS.LIGHT),
//     the default urwid.LineBox characters.
//   - rounded corners: nomadnet/ui/textui/Interfaces.py:1189-1196 and
//     1454-1460 (tlcorner=╭, trcorner=╮, blcorner=╰, brcorner=╯).
// The original never renders double-line borders; the Go port must not either.

// TestApplySingleLineBorders asserts tview's *Focus border fields (which
// default to double-line ╔═╗) are flipped to the single-line ┌─┐ set, while
// the non-focus fields are left unchanged.
func TestApplySingleLineBorders(t *testing.T) {
	t.Parallel()

	// tview.Borders is a library-global applied once (sync.Once) by
	// ApplySingleLineBorders / TestMain; tests must not save/restore it because
	// that write would race with tview's Draw reads in parallel tests.
	ApplySingleLineBorders()

	cases := []struct {
		name string
		got  rune
		want rune
	}{
		{"HorizontalFocus", tview.Borders.HorizontalFocus, BorderHorizontal},
		{"VerticalFocus", tview.Borders.VerticalFocus, BorderVertical},
		{"TopLeftFocus", tview.Borders.TopLeftFocus, BorderTopLeft},
		{"TopRightFocus", tview.Borders.TopRightFocus, BorderTopRight},
		{"BottomLeftFocus", tview.Borders.BottomLeftFocus, BorderBottomLeft},
		{"BottomRightFocus", tview.Borders.BottomRightFocus, BorderBottomRight},
		// Non-focus fields must remain single-line (already so in tview).
		{"TopLeft", tview.Borders.TopLeft, BorderTopLeft},
		{"BottomRight", tview.Borders.BottomRight, BorderBottomRight},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%v = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestFocusedBoxRendersSingleLine proves the global fix end-to-end: a
// focused tview.Box with SetBorder(true) renders single-line corners, not
// tview's default double-line ╔╗╚╝.
func TestFocusedBoxRendersSingleLine(t *testing.T) {
	t.Parallel()

	// Borders are applied once globally (see TestApplySingleLineBorders); do
	// not save/restore tview.Borders here — that write would race with Draw.
	ApplySingleLineBorders()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(20, 5)

	box := tview.NewBox().SetBorder(true).SetTitle("Hi")
	box.SetRect(0, 0, 20, 5)
	box.Focus(nil)
	box.Draw(screen)

	mustCell := func(x, y int, want rune) {
		c, _, _, _ := screen.GetContent(x, y)
		if c != want {
			t.Errorf("focused box cell(%v,%v) = %q, want %q", x, y, c, want)
		}
	}
	mustCell(0, 0, BorderTopLeft)
	mustCell(19, 0, BorderTopRight)
	mustCell(0, 4, BorderBottomLeft)
	mustCell(19, 4, BorderBottomRight)
	mustCell(5, 0, BorderHorizontal) // not under the title
	mustCell(0, 2, BorderVertical)
}

// TestBorderedBoxSingleLine renders a titled BorderedBox and asserts the
// border runes match the original's single-line LineBox characters.
func TestBorderedBoxSingleLine(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(20, 5)

	box := NewBorderedBox("Hi", nil, false)
	box.SetRect(0, 0, 20, 5)
	box.Draw(screen)

	mustCell := func(x, y int, want rune) {
		c, _, _, _ := screen.GetContent(x, y)
		if c != want {
			t.Errorf("cell(%v,%v) = %q, want %q", x, y, c, want)
		}
	}
	mustCell(0, 0, BorderTopLeft)
	mustCell(19, 0, BorderTopRight)
	mustCell(0, 4, BorderBottomLeft)
	mustCell(19, 4, BorderBottomRight)
	mustCell(5, 0, BorderHorizontal) // not under the title
	mustCell(0, 2, BorderVertical)
	mustCell(19, 2, BorderVertical)

	// Title "Hi" centered on the top border: width 20, len 2, tx = (20-2)/2 = 9.
	mustCell(9, 0, 'H')
	mustCell(10, 0, 'i')
}

// TestBorderedBoxRounded asserts the rounded variant uses ╭╮╰╯ corners
// (Interfaces detail), keeping the single-line horizontal/vertical edges.
func TestBorderedBoxRounded(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(16, 4)

	box := NewBorderedBox("Details", nil, true)
	box.SetRect(0, 0, 16, 4)
	box.Draw(screen)

	mustCell := func(x, y int, want rune) {
		c, _, _, _ := screen.GetContent(x, y)
		if c != want {
			t.Errorf("cell(%v,%v) = %q, want %q", x, y, c, want)
		}
	}
	mustCell(0, 0, BorderTopLeftRounded)
	mustCell(15, 0, BorderTopRightRounded)
	mustCell(0, 3, BorderBottomLeftRounded)
	mustCell(15, 3, BorderBottomRightRounded)
	mustCell(2, 0, BorderHorizontal) // not under the title; edges stay single-line
	mustCell(0, 1, BorderVertical)
}

// TestBorderedBoxDrawsContent wraps a TextView with known text and asserts it
// is drawn inside the inner rect (not over the border).
func TestBorderedBoxDrawsContent(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(12, 5)

	tv := tview.NewTextView()
	tv.SetText("Hi")
	box := NewBorderedBox("", tv, false)
	box.SetRect(0, 0, 12, 5)
	box.Draw(screen)

	// Inner rect is (1,1)-(10,3). "Hi" is left-aligned at (1,1).
	c, _, _, _ := screen.GetContent(1, 1)
	if c != 'H' {
		t.Errorf("content cell(1,1) = %q, want 'H'", c)
	}
	c, _, _, _ = screen.GetContent(2, 1)
	if c != 'i' {
		t.Errorf("content cell(2,1) = %q, want 'i'", c)
	}
	// Border must not be clobbered by content.
	c, _, _, _ = screen.GetContent(0, 0)
	if c != BorderTopLeft {
		t.Errorf("border cell(0,0) = %q, want %q", c, BorderTopLeft)
	}
}
