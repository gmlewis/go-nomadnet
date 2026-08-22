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

// TestAnnounceStreamRightTraversesTabBar pins the reported parity bug "in the
// Announce Stream, it is impossible to move from [ Nodes ] to [ Peers ] to
// [ Propagation Nodes ] using the right arrow key". The root cause was the
// outer Network Columns (urwidColumns) handling Right BEFORE forwarding to its
// focused left pane, so Right jumped pane focus to the browser instead of
// letting the Announce Stream's nested tab-bar Columns move between its tab
// buttons. With the left pane marked dynamically self-managing while the tab
// bar has focus, Right forwards into the tab bar and moves Nodes→Peers→PN,
// matching Python's nested-urwid-Columns-first key dispatch
// (urwid/widget/columns.py:1231-1252).
func TestAnnounceStreamRightTraversesTabBar(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)
	nd.SetNavigateCallback(func(string string) {})
	nd.Widget().SetRect(0, 0, 200, 50)

	// Show the Announce Stream. The AnnounceStream Pile defaults focus to its
	// first selectable item — the tab bar (Python urwid Pile focus_position 0).
	nd.toggleList()
	as := nd.announceStream
	if !as.tabBar.HasFocus() {
		t.Fatalf("setup: tab bar should have focus after toggleList, got focus=%T", app.GetFocus())
	}
	if !as.tabNodes.HasFocus() {
		t.Fatalf("setup: tabNodes should have focus initially, got %T", app.GetFocus())
	}

	// Right on [ Nodes ] must move to [ Peers ], NOT jump to the browser pane.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if !as.tabPeers.HasFocus() {
		t.Errorf("after Right on [ Nodes ], focus = %T, want tabPeers (the right arrow must traverse the tab bar, not jump to the browser pane)", app.GetFocus())
	}
	if nd.mainCols.FocusIndex() != 0 {
		t.Errorf("after Right on tab bar, outer Columns focus = %d, want 0 (left pane); Right must not jump to the browser", nd.mainCols.FocusIndex())
	}

	// Right on [ Peers ] must move to [ Propagation Nodes ].
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if !as.tabPN.HasFocus() {
		t.Errorf("after Right on [ Peers ], focus = %T, want tabPN", app.GetFocus())
	}

	// Left must move back [ Propagation Nodes ] -> [ Peers ] -> [ Nodes ].
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if !as.tabPeers.HasFocus() {
		t.Errorf("after Left on [ Propagation Nodes ], focus = %T, want tabPeers", app.GetFocus())
	}
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if !as.tabNodes.HasFocus() {
		t.Errorf("after Left on [ Peers ], focus = %T, want tabNodes", app.GetFocus())
	}
}

// TestAnnounceStreamRightSearchToShowToggle pins the reported parity bug "it is
// impossible to move from 'Search' to '[ Show: Name ]'". The Announce Stream
// filter bar is a Columns of a "Search: " ReadlineEdit and a "Show: Name"
// toggle. In Python, Right at the end of the search Edit's buffer is returned
// unhandled (urwid/widget/edit.py:448-450), so the filter-bar Columns moves
// focus to the toggle (urwid/widget/columns.py:1242-1252). The Go port must
// reproduce that: Right at the end of the (empty) search field moves to the
// toggle, not to the browser pane.
func TestAnnounceStreamRightSearchToShowToggle(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)
	nd.SetNavigateCallback(func(string string) {})
	nd.Widget().SetRect(0, 0, 200, 50)

	nd.toggleList()
	as := nd.announceStream

	// Down twice: tab bar -> filter bar -> entry list is NOT the goal here; one
	// Down moves the Pile focus from the tab bar to the filter bar, whose first
	// focusable child is the search edit.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if !as.search.HasFocus() {
		t.Fatalf("after Down from tab bar, focus = %T, want the search edit", app.GetFocus())
	}
	if !as.filterBar.HasFocus() {
		t.Fatalf("after Down, the filter bar should have focus")
	}

	// Right on the empty search field (cursor at end) must move to the toggle.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if !as.toggle.HasFocus() {
		t.Errorf("after Right on empty Search field, focus = %T, want the [ Show: Name ] toggle (urwid Edit returns right unhandled at end-of-text so the filter-bar Columns moves to the toggle)", app.GetFocus())
	}
	if nd.mainCols.FocusIndex() != 0 {
		t.Errorf("after Right on search, outer Columns focus = %d, want 0 (left pane); Right must not jump to the browser", nd.mainCols.FocusIndex())
	}
}

// TestAnnounceStreamListRightMovesToBrowser pins the counterpart: when the
// Announce Stream entry LIST has focus (not the tab bar / filter bar), Right
// must still move pane focus to the browser — matching Python, where a urwid
// ListBox does not handle Right so it bubbles up to the outer Columns. The
// dynamic self-managing predicate must be false for the list.
func TestAnnounceStreamListRightMovesToBrowser(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	// One node announce so the list has a focusable row.
	nd := NewNetworkDisplay(app, []AnnounceEntry{nodeAnnounce("TestNode")}, nil)
	nd.SetNavigateCallback(func(string string) {})
	nd.Widget().SetRect(0, 0, 200, 50)

	nd.toggleList()
	as := nd.announceStream
	// Down twice: tab bar -> filter bar -> entry list.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if !as.ilb.HasFocus() {
		t.Fatalf("setup: entry list should have focus after two Downs, got %T", app.GetFocus())
	}
	if nd.leftPaneConsumesLeftRight() {
		t.Fatalf("setup: leftPaneConsumesLeftRight should be false when the list has focus")
	}

	// Right on the list must move pane focus to the browser.
	sendViaNetwork(nd, app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := nd.mainCols.FocusIndex(); got != 1 {
		t.Errorf("after Right on the entry list, outer Columns focus = %d, want 1 (browser pane); the list must not consume Right", got)
	}
}
