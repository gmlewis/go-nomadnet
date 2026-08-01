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

	"github.com/rivo/tview"
)

// TestNetworkDisplayNoOuterBorder asserts the Network page has NO outer
// LineBox/title around the whole page: Python's NetworkDisplay sets
// `self.widget = self.columns` directly (Network.py:1666) — the two columns sit
// in the body with no enclosing border, each pane providing its own. The prior
// Go skeleton wrapped everything in a bordered Flex titled "Network", which is
// not parity. The columns Flex itself therefore carries no title.
func TestNetworkDisplayNoOuterBorder(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	cols, ok := nd.Widget().(*tview.Flex)
	if !ok {
		t.Fatalf("Widget is %T, want *tview.Flex (the columns)", nd.Widget())
	}
	if got := cols.GetTitle(); got != "" {
		t.Errorf("outer columns title = %q, want empty (Python has no outer border/title)", got)
	}
}

// TestNetworkDisplayLeftPaneTitled asserts the left pane carries its own titled
// border matching the active list mode, mirroring Python's sub-widget LineBoxes:
// AnnounceStream is titled "Announce Stream" (Network.py:446), KnownNodes is
// titled "Saved Nodes" (Network.py:867). The default mode shows the announce
// stream; toggling swaps to saved nodes and updates the title.
func TestNetworkDisplayLeftPaneTitled(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, nil, nil)

	if got := nd.leftPanel.GetTitle(); got != "Announce Stream" {
		t.Errorf("default left pane title = %q, want Announce Stream", got)
	}

	nd.toggleList() // showingNodes -> true
	if got := nd.leftPanel.GetTitle(); got != "Saved Nodes" {
		t.Errorf("after toggle left pane title = %q, want Saved Nodes", got)
	}

	nd.toggleList() // back to announces
	if got := nd.leftPanel.GetTitle(); got != "Announce Stream" {
		t.Errorf("after second toggle left pane title = %q, want Announce Stream", got)
	}
}
