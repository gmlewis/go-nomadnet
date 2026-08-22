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
	"bytes"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestKeyToANSI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ev   *tcell.EventKey
		want []byte
	}{
		{"enter", tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), []byte{0x0d}},
		{"tab", tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone), []byte{0x09}},
		{"backspace", tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone), []byte{0x7f}},
		{"esc", tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone), []byte{0x1b}},
		{"up", tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), []byte{0x1b, '[', 'A'}},
		{"down", tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), []byte{0x1b, '[', 'B'}},
		{"right", tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), []byte{0x1b, '[', 'C'}},
		{"left", tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), []byte{0x1b, '[', 'D'}},
		{"home", tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), []byte{0x1b, '[', 'H'}},
		{"end", tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), []byte{0x1b, '[', 'F'}},
		{"pgup", tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone), []byte{0x1b, '[', '5', '~'}},
		{"pgdn", tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone), []byte{0x1b, '[', '6', '~'}},
		{"delete", tcell.NewEventKey(tcell.KeyDelete, 0, tcell.ModNone), []byte{0x1b, '[', '3', '~'}},
		{"ctrl-c", tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone), []byte{0x03}},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), []byte{0x18}},
		{"ctrl-a", tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone), []byte{0x01}},
		{"rune A", tcell.NewEventKey(tcell.KeyRune, 'A', tcell.ModNone), []byte{'A'}},
		{"rune z", tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone), []byte{'z'}},
		{"rune e-acute", tcell.NewEventKey(tcell.KeyRune, 0xe9, tcell.ModNone), []byte{0xc3, 0xa9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := keyToANSI(tt.ev)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("keyToANSI(%v) = %x, want %x", tt.name, got, tt.want)
			}
		})
	}
}

func TestVtermPrintAndCursor(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(10, 3)
	s.Write([]byte{'H', 'i'})
	if s.grid[0][0].char != 'H' || s.grid[0][1].char != 'i' {
		t.Errorf("expected H,i at (0,0),(0,1); got %q,%q", s.grid[0][0].char, s.grid[0][1].char)
	}
	cx, cy, vis := s.Cursor()
	if cx != 2 || cy != 0 || !vis {
		t.Errorf("cursor = (%d,%d,vis=%v), want (2,0,true)", cx, cy, vis)
	}
}

func TestVtermCRLF(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(10, 3)
	s.Write([]byte{'A', 'B', 0x0d, 'C'})
	if s.grid[0][0].char != 'C' || s.grid[0][1].char != 'B' {
		t.Errorf("after CR+overwrite, row0 = %q%q, want CB", s.grid[0][0].char, s.grid[0][1].char)
	}
}

func TestVtermLineFeedScrolls(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(3, 2)
	s.Write([]byte{'A', 'A', 'A', 0x0d, 0x0a, 'B', 'B', 'B'})
	if string(cellRow(s, 0)) != "AAA" {
		t.Errorf("row0 = %q, want AAA", string(cellRow(s, 0)))
	}
	if string(cellRow(s, 1)) != "BBB" {
		t.Errorf("row1 = %q, want BBB", string(cellRow(s, 1)))
	}
	s.Write([]byte{0x0a})
	if string(cellRow(s, 0)) != "BBB" {
		t.Errorf("after scroll, row0 = %q, want BBB", string(cellRow(s, 0)))
	}
}

func TestVtermCSICursor(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(20, 5)
	s.Write([]byte{0x1b, '[', '2', ';', '3', 'H'}) // row 2, col 3 (1-based) -> grid[1][2]
	s.Write([]byte{'X'})
	if s.grid[1][2].char != 'X' {
		t.Errorf("after CSI 2;3 H + X, cell(1,2) = %q, want X", s.grid[1][2].char)
	}
}

func TestVtermEraseLine(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(6, 1)
	s.Write([]byte{'A', 'B', 'C', 'D', 'E', 'F'})
	s.Write([]byte{0x1b, '[', '2', 'K'})
	for x := range 6 {
		if s.grid[0][x].char != ' ' {
			t.Errorf("after 2K, cell(0,%d) = %q, want space", x, s.grid[0][x].char)
		}
	}
}

func TestVtermEraseDisplay(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(4, 3)
	s.Write([]byte{'A', 'B', 'C', 'D', 0x0a, 'E', 'F', 'G', 'H', 0x0a, 'I', 'J', 'K', 'L'})
	s.Write([]byte{0x1b, '[', 'H', 0x1b, '[', '2', 'J'})
	for y := range 3 {
		for x := range 4 {
			if s.grid[y][x].char != ' ' {
				t.Errorf("after 2J, cell(%d,%d) = %q, want space", y, x, s.grid[y][x].char)
			}
		}
	}
}

func TestVtermSGRColor(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(5, 1)
	s.Write([]byte{0x1b, '[', '3', '1', 'm'})
	s.Write([]byte{'R'})
	fg, _, _ := s.grid[0][0].style.Decompose()
	// SGR 31 (red) maps to tcell.ColorMaroon = ColorValid+1 (the ColorValid flag
	// is required; a bare Color(n) is invalid and renders as default).
	if fg != tcell.ColorValid+tcell.Color(1) {
		t.Errorf("after SGR 31, fg = %v, want %v (ColorMaroon)", fg, tcell.ColorValid+tcell.Color(1))
	}
}

func TestVtermSGRTruecolor(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(5, 1)
	s.Write([]byte{0x1b, '[', '3', '8', ';', '2', ';', '1', '0', ';', '2', '0', ';', '3', '0', 'm'})
	s.Write([]byte{'T'})
	fg, _, _ := s.grid[0][0].style.Decompose()
	want := tcell.NewRGBColor(10, 20, 30)
	if fg != want {
		t.Errorf("truecolor fg = %v, want %v", fg, want)
	}
}

func TestVtermCursorVisibility(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(5, 1)
	s.Write([]byte{0x1b, '[', '?', '2', '5', 'l'})
	_, _, vis := s.Cursor()
	if vis {
		t.Error("after ?25l, cursor should be hidden")
	}
	s.Write([]byte{0x1b, '[', '?', '2', '5', 'h'})
	_, _, vis = s.Cursor()
	if !vis {
		t.Error("after ?25h, cursor should be visible")
	}
}

func TestVtermResize(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(5, 2)
	s.Write([]byte{'H', 'E', 'L', 'L', 'O'})
	s.Resize(8, 3)
	if s.cols != 8 || s.rows != 3 {
		t.Fatalf("size = %dx%d, want 8x3", s.cols, s.rows)
	}
	if string(cellRow(s, 0))[:5] != "HELLO" {
		t.Errorf("after resize, row0 = %q, want HELLO...", string(cellRow(s, 0)))
	}
}

func TestVtermAltScreenSaveRestore(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(8, 2)
	s.Write([]byte{'M', 'A', 'I', 'N'})
	// Enter alt screen: save MAIN, clear, home.
	s.Write([]byte{0x1b, '[', '?', '1', '0', '4', '9', 'h'})
	if string(cellRow(s, 0))[:4] != "    " {
		t.Errorf("after ?1049h, row0 = %q, want cleared", string(cellRow(s, 0)))
	}
	s.Write([]byte{'A', 'L', 'T'})
	if string(cellRow(s, 0))[:3] != "ALT" {
		t.Errorf("alt row0 = %q, want ALT", string(cellRow(s, 0)))
	}
	// Leave alt screen: restore MAIN.
	s.Write([]byte{0x1b, '[', '?', '1', '0', '4', '9', 'l'})
	if string(cellRow(s, 0))[:4] != "MAIN" {
		t.Errorf("after ?1049l, row0 = %q, want MAIN restored", string(cellRow(s, 0)))
	}
}

func TestVtermMouseModes(t *testing.T) {
	t.Parallel()
	s := newVtermScreen(10, 1)
	if report, sgr := s.MouseModes(); report || sgr {
		t.Fatal("mouse modes should be off initially")
	}
	s.Write([]byte{0x1b, '[', '?', '1', '0', '0', '0', 'h'}) // enable reporting
	s.Write([]byte{0x1b, '[', '?', '1', '0', '0', '6', 'h'}) // enable SGR-1006
	if report, sgr := s.MouseModes(); !report || !sgr {
		t.Errorf("after ?1000h ?1006h, modes = (%v,%v), want (true,true)", report, sgr)
	}
	s.Write([]byte{0x1b, '[', '?', '1', '0', '0', '0', 'l'}) // disable reporting
	if report, _ := s.MouseModes(); report {
		t.Error("after ?1000l, mouse reporting should be off")
	}
}

func TestMouseToSGR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		button int
		col    int
		row    int
		press  bool
		want   []byte
	}{
		{"left press", 0, 5, 3, true, []byte{0x1b, '[', '<', '0', ';', '5', ';', '3', 'M'}},
		{"right press", 2, 1, 1, true, []byte{0x1b, '[', '<', '2', ';', '1', ';', '1', 'M'}},
		{"left release", 0, 5, 3, false, []byte{0x1b, '[', '<', '0', ';', '5', ';', '3', 'm'}},
		{"wheel up", 64, 9, 2, true, []byte{0x1b, '[', '<', '6', '4', ';', '9', ';', '2', 'M'}},
		{"drag left (motion bit 32)", 32, 7, 4, true, []byte{0x1b, '[', '<', '3', '2', ';', '7', ';', '4', 'M'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mouseToSGR(tt.button, tt.col, tt.row, tt.press)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("mouseToSGR(%d,%d,%d,%v) = %x, want %x", tt.button, tt.col, tt.row, tt.press, got, tt.want)
			}
		})
	}
}
func cellRow(s *vtermScreen, y int) []rune {
	r := make([]rune, s.cols)
	for x := 0; x < s.cols; x++ {
		r[x] = s.grid[y][x].char
	}
	return r
}
