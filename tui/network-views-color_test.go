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

// TestNetworkViewsBaseColor pins the NodeInfo and LocalPeer base text color to
// the terminal default. Python's NodeInfo uses bare `urwid.Text` lines ("Addr :
// ...", "Name : ...") with `widget_style = ""` (Network.py:1372,1387-1388) → the
// empty-string attr resolves to default; LocalPeer uses bare `urwid.Text`
// ("LXMF Addr : ...", "Identity : ...") with NO AttrMap at all (Network.py:1271-
// 1272,1351). Both therefore inherit the terminal default, not a palette color.
// The Go port previously used 0xbbbbbb. (Go additionally over-formats the labels
// with `[::b]`/`[lightblue]`/`[gray]` tags Python does not emit — a separate
// parity gap; this test only pins the base/fallback color.)
func TestNetworkViewsBaseColor(t *testing.T) {
	t.Parallel()

	probe := func(t *testing.T, prim tview.Primitive) {
		t.Helper()
		tv, ok := prim.(*tview.TextView)
		if !ok {
			t.Fatalf("widget is %T, want *tview.TextView", prim)
		}
		tv.SetText("X")
		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatalf("screen.Init: %v", err)
		}
		defer screen.Fini()
		screen.SetSize(40, 3)
		tv.SetRect(0, 0, 40, 3)
		tv.Draw(screen)
		if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
			t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
		} else {
			fg, _, _ := style.Decompose()
			if fg != tcell.ColorDefault {
				t.Errorf("base fg = %v, want ColorDefault (Python bare urwid.Text)", fg)
			}
		}
	}

	t.Run("NodeInfo", func(t *testing.T) {
		t.Parallel()
		ni := NewNodeInfo("abcdef0123456789", "node-name")
		probe(t, ni.widget)
	})

	t.Run("LocalPeer", func(t *testing.T) {
		t.Parallel()
		lp := NewLocalPeer("abcdef0123456789", "peer-name", "just now")
		probe(t, lp.widget)
	})
}
