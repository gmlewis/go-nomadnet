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

// TestConversationsDetailBaseColor pins the Conversations right-pane detail
// base color to the terminal default. Python's empty-state right pane is a bare
// `urwid.Text("\n  No conversation selected")` inside a LineBox/Filler
// (Conversations.py:1881-1884) — no AttrMap — so its base is the terminal
// default. (The populated summary Go renders in cd.detail is a Go-specific
// "Trust/Messages/Last" format with no Python equivalent; the empty state is
// the only Python-specified surface, and it is default.) The Go port previously
// used 0xbbbbbb.
func TestConversationsDetailBaseColor(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	cd := NewConversationsDisplay(app, nil)

	cd.detail.SetText("X")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(40, 5)
	cd.detail.SetRect(0, 0, 40, 5)
	cd.detail.Draw(screen)
	// cd.detail has a border; probe the interior cell (1,1) where "X" lands.
	if c, _, style, _ := screen.GetContent(1, 1); c != 'X' {
		t.Fatalf("detail cell (1,1) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if fg != tcell.ColorDefault {
			t.Errorf("detail base fg = %v, want ColorDefault (Python empty-state bare Text)", fg)
		}
	}
}
