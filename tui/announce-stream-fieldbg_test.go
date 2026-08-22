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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestAnnounceStreamSearchFieldBackground pins the parity fix for the "Search
// input area is rendered differently" bug: Python's search_edit is a bare
// ReadlineEdit with no AttrMap (Network.py:419), so urwid renders it in the
// default text style with NO background. tview's InputField otherwise paints
// its text area with Styles.ContrastBackgroundColor (blue), producing a colored
// Search input area the original does not have. The field background must be
// the terminal default (transparent), not the contrast color.
func TestAnnounceStreamSearchFieldBackground(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	nd.toggleList() // show the Announce Stream (builds the search edit)

	_, bg, _ := nd.announceStream.search.GetFieldStyle().Decompose()
	if bg != tcell.ColorDefault {
		t.Errorf("search field background = %v, want ColorDefault (transparent); "+
			"tview default ContrastBackgroundColor = %v", bg, tview.Styles.ContrastBackgroundColor)
	}
}

// TestLocalPeerNameFieldBackground pins the same fix for the Local Peer Info
// "Name" edit (Python LocalPeer.e_name, Network.py:1273): a bare ReadlineEdit
// with no background.
func TestLocalPeerNameFieldBackground(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	lp := NewLocalPeerDisplay(app, "addr", "id", "", time.Time{})
	_, bg, _ := lp.nameEdit.GetFieldStyle().Decompose()
	if bg != tcell.ColorDefault {
		t.Errorf("local peer name field background = %v, want ColorDefault (transparent)", bg)
	}
}

// TestKnownNodeInfoFieldBackground pins the same fix for the KnownNodeInfo form
// edits (Python e_name/e_sort, Network.py:678-679): bare ReadlineEdit with no
// background.
func TestKnownNodeInfoFieldBackground(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	ki := newKnownNodeInfoDisplay(nd, "deadbeef", KnownNodeInfoData{
		DisplayStr: "Node", SortStr: "None", TrustLevel: "unknown",
	})
	_, bg, _ := ki.nameEdit.GetFieldStyle().Decompose()
	if bg != tcell.ColorDefault {
		t.Errorf("known node info name field background = %v, want ColorDefault", bg)
	}
	_, sbg, _ := ki.sortEdit.GetFieldStyle().Decompose()
	if sbg != tcell.ColorDefault {
		t.Errorf("known node info sort field background = %v, want ColorDefault", sbg)
	}
}
