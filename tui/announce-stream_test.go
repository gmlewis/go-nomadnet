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
	"strings"
	"testing"
	"time"
)

// TestUrwidColumnWidths pins the column-width distribution against values
// measured from the Python original's urwid Columns (captured at 80x24,
// /tmp/cap-py-ai frame 06): the Announce Stream tab bar uses weights
// [1,1,3] dividechars 1 over inner width 50 → [10,10,28], and the filter bar
// uses weights [2,1] dividechars 1 over 50 → [33,16]. urwid sorts weighted
// columns by (weight,index) and rounds with +0.5, so the leftover lands on the
// heaviest column — NOT tview Flex's last-item rule (which would give [9,9,28]).
func TestUrwidColumnWidths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		maxcol      int
		weights     []int
		dividechars int
		want        []int
	}{
		{"tab bar 1:1:3 @50", 50, []int{1, 1, 3}, 1, []int{10, 10, 28}},
		{"filter bar 2:1 @50", 50, []int{2, 1}, 1, []int{33, 16}},
		{"equal 1:1:1 @30", 30, []int{1, 1, 1}, 1, []int{9, 10, 9}},
		{"single col @50", 50, []int{1}, 0, []int{50}},
		{"no dividechars 1:1:3 @50", 50, []int{1, 1, 3}, 0, []int{10, 10, 30}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := urwidColumnWidths(tc.maxcol, tc.weights, tc.dividechars)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("col %d: got %d, want %d (all %v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

// TestTabButtonRendering pins the TabButton against the captured Python tab bar
// (Network.py:390/409-411): a "[ label ]" urwid Button whose label space-wraps
// in the narrow column, with brackets appearing only on the first row.
func TestTabButtonRendering(t *testing.T) {
	t.Parallel()

	// "Nodes (3)" in a 10-wide column: label area = 6, wraps to "Nodes" + "(3)".
	// Row 0: "[ Nodes  ]", row 1: "  (3)     " (brackets blank on row 1).
	tb := NewTabButton("Nodes (3)")
	rows := renderPrimitive(t, tb, 10, 2)
	if rows[0] != "[ Nodes  ]" {
		t.Errorf("row0 = %q, want %q", rows[0], "[ Nodes  ]")
	}
	if rows[1] != "  (3)     " {
		t.Errorf("row1 = %q, want %q", rows[1], "  (3)     ")
	}

	// "Propagation Nodes (2)" in a 28-wide column: label area = 24, fits on one
	// row → "[ Propagation Nodes (2)    ]" (1 row).
	pn := NewTabButton("Propagation Nodes (2)")
	r2 := renderPrimitive(t, pn, 28, 1)
	if r2[0] != "[ Propagation Nodes (2)    ]" {
		t.Errorf("pn row0 = %q, want %q", r2[0], "[ Propagation Nodes (2)    ]")
	}

	// "Show: Name" in a 16-wide column: label area = 12, fits → "[ Show: Name   ]".
	dt := NewTabButton("Show: Name")
	r3 := renderPrimitive(t, dt, 16, 1)
	if r3[0] != "[ Show: Name   ]" {
		t.Errorf("display toggle row0 = %q, want %q", r3[0], "[ Show: Name   ]")
	}
}

// TestTabButtonRequiredHeight verifies the wrapped-label row count drives the
// tab bar height (2 rows for "Nodes (N)" in a 10-wide column, 1 for the
// propagation-nodes tab in 28 and the display toggle in 16).
func TestTabButtonRequiredHeight(t *testing.T) {
	t.Parallel()
	cases := []struct {
		label string
		w     int
		want  int
	}{
		{"Nodes (3)", 10, 2},
		{"Propagation Nodes (2)", 28, 1},
		{"Show: Name", 16, 1},
		{"Peers (4)", 10, 2},
	}
	for _, c := range cases {
		tb := NewTabButton(c.label)
		if got := tb.RequiredHeight(c.w); got != c.want {
			t.Errorf("RequiredHeight(%q,%d) = %d, want %d", c.label, c.w, got, c.want)
		}
	}
}

// TestAnnounceStreamDisplayLayout pins the AnnounceStream Pile against the
// captured Python tab bar + filter bar (Network.py:394-466, /tmp/cap-py-ai
// frame 06): a 2-row tab bar "[ Nodes  ] [ Peers  ] [ Propagation Nodes (2)    ]"
// over "  (3)        (4)                                  " (the wrapped counts;
// brackets blank on row 2), then the filter bar "Search: ... [ Show: Name   ]",
// then the IndicativeListBox. Counts are per-type over the announce stream.
func TestAnnounceStreamDisplayLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)

	// 3 node, 4 peer, 2 pn announces — the Python capture's counts.
	now := time.Now()
	announces := []AnnounceEntry{
		{DisplayName: "Node A", Type: "node", SourceHash: "aaaa", Timestamp: now, AppData: "Node A"},
		{DisplayName: "Node B", Type: "node", SourceHash: "bbbb", Timestamp: now, AppData: "Node B"},
		{DisplayName: "Node C", Type: "node", SourceHash: "cccc", Timestamp: now, AppData: "Node C"},
		{DisplayName: "Peer 1", Type: "peer", SourceHash: "d0", Timestamp: now, AppData: "Peer 1"},
		{DisplayName: "Peer 2", Type: "peer", SourceHash: "d1", Timestamp: now, AppData: "Peer 2"},
		{DisplayName: "Peer 3", Type: "peer", SourceHash: "d2", Timestamp: now, AppData: "Peer 3"},
		{DisplayName: "Peer 4", Type: "peer", SourceHash: "d3", Timestamp: now, AppData: "Peer 4"},
		{DisplayName: "PN 1", Type: "pn", SourceHash: "e0", Timestamp: now, AppData: "PN 1"},
		{DisplayName: "PN 2", Type: "pn", SourceHash: "e1", Timestamp: now, AppData: "PN 2"},
	}
	nd := NewNetworkDisplay(app, announces, nil)
	// Switch to the Announce Stream so the Pile is the left-pane content.
	nd.toggleList() // showingNodes true -> false (Announce Stream)

	rows := renderPrimitive(t, nd.announceStream.Widget(), 50, 8)

	// Row 0: tab bar first row. The weight-1 tabs wrap their counts to row 1,
	// so only "[ Nodes  ]"/"[ Peers  ]" show; the weight-3 tab fits on one row.
	wantRow0 := "[ Nodes  ] [ Peers  ] [ Propagation Nodes (2)    ]"
	if rows[0] != wantRow0 {
		t.Errorf("row0 = %q, want %q", rows[0], wantRow0)
	}
	// Row 1: wrapped counts "(3)"/"(4)" under the weight-1 tabs; the weight-3
	// tab's second row is blank (its label did not wrap).
	if rows[1] != "  (3)        (4)                                  " {
		t.Errorf("row1 = %q, want the wrapped counts row", rows[1])
	}
	// Row 2: filter bar — "Search: " caption + empty field + display toggle.
	wantRow2 := "Search:" + strings.Repeat(" ", 27) + "[ Show: Name   ]"
	if rows[2] != wantRow2 {
		t.Errorf("row2 = %q, want %q", rows[2], wantRow2)
	}
	// Row 3: IndicativeListBox top indicator.
	if !strings.Contains(rows[3], "───") {
		t.Errorf("row3 = %q, want the top '───' indicator", rows[3])
	}
	// Last row: bottom indicator.
	if !strings.Contains(rows[7], "───") {
		t.Errorf("row7 = %q, want the bottom '───' indicator", rows[7])
	}
}

// TestAnnounceStreamTabFiltering verifies the list shows only the active tab's
// type and the tab labels carry the per-type counts (Network.py:481-525).
func TestAnnounceStreamTabFiltering(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	now := time.Now()
	announces := []AnnounceEntry{
		{DisplayName: "Node A", Type: "node", SourceHash: "aa", Timestamp: now, AppData: "Node A"},
		{DisplayName: "Peer 1", Type: "peer", SourceHash: "bb", Timestamp: now, AppData: "Peer 1"},
		{DisplayName: "PN 1", Type: "pn", SourceHash: "cc", Timestamp: now, AppData: "PN 1"},
	}
	nd := NewNetworkDisplay(app, announces, nil)
	as := nd.announceStream

	// Default tab = nodes: one node entry shown.
	if got := as.ilb.List.GetItemCount(); got != 1 {
		t.Errorf("nodes tab: list has %d items, want 1", got)
	}
	if as.tabNodes.Label() != "Nodes (1)" || as.tabPeers.Label() != "Peers (1)" || as.tabPN.Label() != "Propagation Nodes (1)" {
		t.Errorf("tab labels = %q/%q/%q, want Nodes (1)/Peers (1)/Propagation Nodes (1)",
			as.tabNodes.Label(), as.tabPeers.Label(), as.tabPN.Label())
	}

	as.SetCurrentTab(tabPeers)
	if got := as.ilb.List.GetItemCount(); got != 1 {
		t.Errorf("peers tab: list has %d items, want 1", got)
	}
	as.SetCurrentTab(tabPN)
	if got := as.ilb.List.GetItemCount(); got != 1 {
		t.Errorf("pn tab: list has %d items, want 1", got)
	}
}

// TestAnnounceStreamSearchFilter verifies the search text filters by app data
// (Python on_search_change matches e[2], Network.py:456-458/492-498).
func TestAnnounceStreamSearchFilter(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	now := time.Now()
	announces := []AnnounceEntry{
		{DisplayName: "Alpha Node", Type: "node", SourceHash: "aa", Timestamp: now, AppData: "Alpha Node"},
		{DisplayName: "Beta Node", Type: "node", SourceHash: "bb", Timestamp: now, AppData: "Beta Node"},
	}
	nd := NewNetworkDisplay(app, announces, nil)
	as := nd.announceStream

	if got := as.ilb.List.GetItemCount(); got != 2 {
		t.Fatalf("before search: %d items, want 2", got)
	}
	as.search.SetText("alpha")
	as.onSearchChange()
	if got := as.ilb.List.GetItemCount(); got != 1 {
		t.Errorf("after search 'alpha': %d items, want 1", got)
	}
	// Clearing the search restores both.
	as.search.SetText("")
	as.onSearchChange()
	if got := as.ilb.List.GetItemCount(); got != 2 {
		t.Errorf("after clearing search: %d items, want 2", got)
	}
}

// TestAnnounceStreamDisplayToggle verifies the display-mode toggle flips the
// toggle label and re-renders (Network.py:460-466).
func TestAnnounceStreamDisplayToggle(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, []AnnounceEntry{
		{DisplayName: "X", Type: "node", SourceHash: "aa", Timestamp: time.Now(), AppData: "X"},
	}, nil)
	as := nd.announceStream

	if as.toggle.Label() != "Show: Name" {
		t.Errorf("initial toggle label = %q, want 'Show: Name'", as.toggle.Label())
	}
	as.toggleDisplayMode()
	if as.toggle.Label() != "Show: Dest" {
		t.Errorf("after toggle: label = %q, want 'Show: Dest'", as.toggle.Label())
	}
	as.toggleDisplayMode()
	if as.toggle.Label() != "Show: Name" {
		t.Errorf("after 2nd toggle: label = %q, want 'Show: Name'", as.toggle.Label())
	}
}
