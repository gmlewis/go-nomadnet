package tui

import (
	"testing"

	"github.com/gmlewis/tcell/v2"
)

func TestVtermMockColorSequenceChunked(t *testing.T) {
	s := newVtermScreen(40, 6)
	// Feed each SGR/text in a SEPARATE Write call, mimicking the live mock
	// editor's per-o.Write chunked delivery over the PTY.
	writes := [][]byte{
		{0x1b, '[', '?', '1', '0', '4', '9', 'h'},
		{0x1b, '[', '7', 'm'},
		[]byte("REVERSE HEADER"),
		{0x1b, '[', '0', 'm', 0x0d, 0x0a},
		{0x1b, '[', '3', '1', 'm'},
		[]byte("RED TEXT"),
		{0x1b, '[', '0', 'm', 0x0d, 0x0a},
		{0x1b, '[', '1', ';', '4', 'm'},
		[]byte("BOLD UNDERLINE"),
		{0x1b, '[', '0', 'm', 0x0d, 0x0a},
		{0x1b, '[', '3', '8', ';', '2', ';', '2', '5', '5', ';', '1', '2', '8', ';', '0', 'm'},
		[]byte("TRUECOLOR ORANGE"),
		{0x1b, '[', '0', 'm'},
	}
	for _, w := range writes {
		s.Write(w)
	}
	fg1, _, attr1 := s.grid[1][0].style.Decompose()
	if attr1&tcell.AttrReverse != 0 {
		t.Errorf("RED TEXT: reverse IS set (want cleared), attr=%v", attr1)
	}
	if fg1 != tcell.ColorValid+tcell.Color(1) {
		t.Errorf("RED TEXT: fg=%v, want ColorMaroon (ColorValid+1)", fg1)
	}
	fg3, _, _ := s.grid[3][0].style.Decompose()
	if fg3 != tcell.NewRGBColor(255, 128, 0) {
		t.Errorf("TRUECOLOR: fg=%v, want NewRGBColor(255,128,0)", fg3)
	}
}

func TestVtermResizeVirtualCursorStickiness(t *testing.T) {
	t.Parallel()
	// Landscape: 80x24. The cursor is placed at (60, 18).
	s := newVtermScreen(80, 24)
	s.Write([]byte("\x1b[19;61H")) // 1-based row 19, col 61 -> 0-based (60, 18)
	if x, y, _ := s.Cursor(); x != 60 || y != 18 {
		t.Fatalf("expected cursor at (60, 18), got (%v, %v)", x, y)
	}

	// Rotate to portrait: 40x50. Column shrinks, clamped to 39, row preserved.
	s.Resize(40, 50)
	if x, y, _ := s.Cursor(); x != 39 || y != 18 {
		t.Fatalf("expected cursor at (39, 18) after resize, got (%v, %v)", x, y)
	}

	// Rotate back to landscape: 80x24. Virtually sticks and restores to (60, 18).
	s.Resize(80, 24)
	if x, y, _ := s.Cursor(); x != 60 || y != 18 {
		t.Fatalf("expected cursor restored to (60, 18), got (%v, %v)", x, y)
	}

	// Portrait: place cursor at (20, 45).
	s.Resize(40, 50)
	s.Write([]byte("\x1b[46;21H")) // 1-based row 46, col 21 -> 0-based (20, 45)
	if x, y, _ := s.Cursor(); x != 20 || y != 45 {
		t.Fatalf("expected cursor at (20, 45), got (%v, %v)", x, y)
	}

	// Rotate to landscape: 80x24. Row shrinks, clamped to 23.
	s.Resize(80, 24)
	if x, y, _ := s.Cursor(); x != 20 || y != 23 {
		t.Fatalf("expected cursor at (20, 23) after resize, got (%v, %v)", x, y)
	}

	// Rotate back to portrait: 40x50. Virtually sticks and restores to (20, 45).
	s.Resize(40, 50)
	if x, y, _ := s.Cursor(); x != 20 || y != 45 {
		t.Fatalf("expected cursor restored to (20, 45), got (%v, %v)", x, y)
	}
}
