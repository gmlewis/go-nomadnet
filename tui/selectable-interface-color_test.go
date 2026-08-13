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

// TestSelectableInterfaceItemStatusColors pins the V-Interfaces coloring fix.
// Python's SelectableInterfaceItem (Interfaces.py:1136-1158) styles the
// Enabled/Disabled status and the Connected/Disconnected connection text from
// the palette: connected_status (green) when true, disconnected_status (red)
// when false. The Go port painted every row with tcell.StyleDefault, so the
// page read as monochrome. This test asserts the status/connection cells carry
// the palette foreground color resolved through the same StyleRegistry the app
// uses (robust to the exact RGB of the named "dark green"/"dark red" colors).
//
// Row layout (content starts at x=3): the status row is
// "Status:   <status10> | <conn>" → the status value's first char is at x=13,
// the connection value's first char at x=26 (both on row y=2).
func TestSelectableInterfaceItemStatusColors(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	green, _, _ := app.Styles.Style("connected_status").Decompose()
	red, _, _ := app.Styles.Style("disconnected_status").Decompose()
	if green == tcell.ColorDefault {
		t.Fatal("connected_status did not resolve to a color")
	}
	if red == tcell.ColorDefault {
		t.Fatal("disconnected_status did not resolve to a color")
	}

	draw := func(t *testing.T, s *SelectableInterfaceItem) tcell.Screen {
		t.Helper()
		s.SetRect(0, 0, 60, InterfaceItemHeight)
		sc := tcell.NewSimulationScreen("UTF-8")
		if sc == nil {
			t.Fatal("nil simulation screen")
		}
		if err := sc.Init(); err != nil {
			t.Fatalf("screen.Init: %v", err)
		}
		sc.SetSize(60, InterfaceItemHeight)
		s.Draw(sc)
		return sc
	}
	fgAt := func(sc tcell.Screen, x, y int) tcell.Color {
		_, _, st, _ := cellContent(sc, x, y)
		f, _, _ := st.Decompose()
		return f
	}

	// Enabled + Connected → both green.
	t.Run("enabled_connected_green", func(t *testing.T) {
		t.Parallel()
		s := NewSelectableInterfaceItem("RNode Test", "RNodeInterface", true, true, 100, 200, "ᚱ")
		s.SetApp(app)
		sc := draw(t, s)
		defer sc.Fini()
		if got := fgAt(sc, 13, 2); got != green {
			t.Errorf("Enabled fg = %v, want %v (connected_status green)", got, green)
		}
		if got := fgAt(sc, 26, 2); got != green {
			t.Errorf("Connected fg = %v, want %v (connected_status green)", got, green)
		}
	})

	// Disabled + Disconnected → both red.
	t.Run("disabled_disconnected_red", func(t *testing.T) {
		t.Parallel()
		s := NewSelectableInterfaceItem("Michmesh", "TCPClientInterface", false, false, 0, 0, "ᚱ")
		s.SetApp(app)
		sc := draw(t, s)
		defer sc.Fini()
		if got := fgAt(sc, 13, 2); got != red {
			t.Errorf("Disabled fg = %v, want %v (disconnected_status red)", got, red)
		}
		if got := fgAt(sc, 26, 2); got != red {
			t.Errorf("Disconnected fg = %v, want %v (disconnected_status red)", got, red)
		}
	})

	// Mixed: Enabled but Disconnected → status green, connection red.
	t.Run("enabled_disconnected_mixed", func(t *testing.T) {
		t.Parallel()
		s := NewSelectableInterfaceItem("Ifc", "TCPClientInterface", false, true, 1, 2, "ᚱ")
		s.SetApp(app)
		sc := draw(t, s)
		defer sc.Fini()
		if got := fgAt(sc, 13, 2); got != green {
			t.Errorf("Enabled (disconnected iface) fg = %v, want green", got)
		}
		if got := fgAt(sc, 26, 2); got != red {
			t.Errorf("Disconnected fg = %v, want red", got)
		}
	})

	// No app wired → graceful fallback to StyleDefault (no panic, no color).
	// This keeps the text-only construction tests valid.
	t.Run("nil_app_default_style", func(t *testing.T) {
		t.Parallel()
		s := NewSelectableInterfaceItem("X", "TCPClientInterface", true, true, 0, 0, "ᚱ")
		// no SetApp
		sc := draw(t, s)
		defer sc.Fini()
		if got := fgAt(sc, 13, 2); got != tcell.ColorDefault {
			t.Errorf("nil-app Enabled fg = %v, want ColorDefault (fallback)", got)
		}
	})
}
