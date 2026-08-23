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

// TestPeerInfoDialogFieldColors pins the Peer Info dialog's ReadlineEdit
// input fields (eName, eCopy, eNotes) to the "default" style. Python's
// Conversations.py creates these as BARE ReadlineEdit (no urwid.AttrMap) —
// they inherit urwid's default terminal colors, NOT msg_editor (#111/#0bb)
// and NOT the prior Go 0x222222/0xdddddd. The Go port must leave the field
// background and text at tcell.ColorDefault to match.
//
// Python source: Conversations.py:845-889 (e_id, e_name, e_copy, e_notes are
// bare ReadlineEdit, placed raw into dialog_pile with no AttrMap).
func TestPeerInfoDialogFieldColors(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	cd := NewConversationsDisplay(app, nil)
	if cd == nil {
		t.Fatal("NewConversationsDisplay returned nil")
	}

	entry := PeerInfoEntry{
		SourceHash:  "0123456789abcdef0123456789abcdef",
		DisplayName: "TestPeer",
		Notes:       "some notes",
	}

	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{}, func(PeerInfoEntry) {})

	if cd.listSlotOverlay == nil {
		t.Fatal("listSlotOverlay is nil after ShowPeerInfoDialog")
	}
	dialog := cd.listSlotOverlay.Dialog()
	if dialog == nil {
		t.Fatal("dialog is nil")
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 26)
	dialog.SetRect(0, 0, 60, 26)
	dialog.Draw(screen)

	// Establish what ColorDefault renders as on the simulation screen by
	// probing a non-field cell (the divider row at row 4, col 5). This cell
	// uses the dialog's default border/background style.
	_, _, defStyle, _ := cellContent(screen, 5, 4)
	_, defBG, _ := defStyle.Decompose()
	defaultBG := uint32(defBG.Hex()) & 0xffffff

	// eName is the 2nd row of the dialog content (after addrText at row 0).
	// The DialogLineBox border adds 1 row. The label "Name : " is 7 chars
	// (including trailing space), so the field text starts at column 8.
	// Dialog row = 1 (border) + 1 (addrText) = 2.
	_, _, style, _ := cellContent(screen, 8, 2)
	_, bg, _ := style.Decompose()
	if got := uint32(bg.Hex()) & 0xffffff; got != defaultBG {
		t.Errorf("eName field bg = #%06x, want #%06x (ColorDefault); "+
			"Python leaves ReadlineEdit BARE (default style), not 0x222222",
			got, defaultBG)
	}

	// eCopy is the 3rd content row (dialog row 3). Label "Copy : " = 7 chars.
	_, _, style2, _ := cellContent(screen, 8, 3)
	_, bg2, _ := style2.Decompose()
	if got := uint32(bg2.Hex()) & 0xffffff; got != defaultBG {
		t.Errorf("eCopy field bg = #%06x, want #%06x (ColorDefault); "+
			"Python leaves ReadlineEdit BARE (default style), not 0x222222",
			got, defaultBG)
	}
}
