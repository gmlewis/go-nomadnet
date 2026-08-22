package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
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
