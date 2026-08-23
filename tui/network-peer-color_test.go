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

// TestLocalPeerDialogTextColor pins the Local Peer dialog's info text to
// the "default" style. Python's LocalPeer (Network.py:1259-1355) uses BARE
// urwid.Text for LXMF Addr/Identity/Name — no AttrMap, default terminal
// colors. The Go port previously used 0xdddddd which Python never emits
// for this panel.
//
// Python source: Network.py:1271-1273 (bare urwid.Text, no AttrMap),
// Network.py:1351 (bare urwid.LineBox, no AttrMap).
func TestLocalPeerDialogTextColor(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	if nd == nil {
		t.Fatal("NewNetworkDisplay returned nil")
	}

	// Initialize the dialog manager with the app and network root.
	app.Dialogs.Init(app.Application, nd.Widget())

	nd.ShowLocalPeerDialog("abcd1234", "deadbeef", "MyNode", "")

	// Get the dialog from the DialogManager.
	if !nd.app.Dialogs.Open() {
		t.Fatal("no dialog open after ShowLocalPeerDialog")
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 12)

	// Draw the dialog pages root.
	nd.app.Dialogs.Pages().SetRect(0, 0, 60, 12)
	nd.app.Dialogs.Pages().Draw(screen)

	// Probe the text content area inside the dialog. The dialog has a
	// border (1 row/col). The first text line "LXMF Addr :" starts at
	// row 1, col 1. Probe col 15 (within the text, past the label).
	_, _, style, _ := cellContent(screen, 15, 2)
	_, bg, _ := style.Decompose()

	// Establish what ColorDefault renders as on the simulation screen.
	_, _, defStyle, _ := cellContent(screen, 0, 0)
	_, defBG, _ := defStyle.Decompose()
	defaultBG := uint32(defBG.Hex()) & 0xffffff

	if got := uint32(bg.Hex()) & 0xffffff; got != defaultBG {
		t.Errorf("LocalPeer dialog text bg = #%06x, want #%06x (ColorDefault); "+
			"Python uses bare urwid.Text (default), not 0xdddddd",
			got, defaultBG)
	}
}
