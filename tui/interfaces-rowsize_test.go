// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
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

// TestInterfaceListBufferRow verifies the interface list leaves 1 blank
// buffer row at the bottom, matching Python's iface_row_offset behavior.
// Python sizes its BoxAdapter to `terminal_rows - iface_row_offset` (offset=4,
// Interfaces.py:2837) and the ListBox renders items + 1 blank buffer row. The
// Go port previously filled all remaining rows, showing 1 extra partial row.
func TestInterfaceListBufferRow(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	ifaces := make([]InterfaceInfo, 10)
	display := NewInterfacesDisplay(app, ifaces)
	if display == nil {
		t.Fatal("NewInterfacesDisplay returned nil")
	}

	// Simulate 80×24 terminal: the interfaces display gets 22 rows
	// (24 - menu - footer). The title takes 2 rows, so the list gets 20.
	// Python shows 18 rows of items + 1 blank buffer. Go should leave the
	// last row blank.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 22)
	display.Widget().SetRect(0, 0, 80, 22)
	display.Widget().Draw(screen)

	// The list area starts after the 2-row title (row 2). The last row
	// of the available space (row 21) should be blank (no interface
	// content), matching Python's 1-row buffer.
	lastRow := 21
	blankRow := true
	for x := 0; x < 80; x++ {
		c, _, _, _ := cellContent(screen, x, lastRow)
		if c != 0 && c != ' ' {
			blankRow = false
			break
		}
	}
	if !blankRow {
		t.Errorf("last row of interface list is not blank; " +
			"Python leaves 1 buffer row (iface_row_offset=4)")
	}
}
