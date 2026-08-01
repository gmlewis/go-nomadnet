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
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// announceStreamTab is the active tab of the AnnounceStream.
type announceStreamTab string

const (
	tabNodes announceStreamTab = "nodes"
	tabPeers announceStreamTab = "peers"
	tabPN    announceStreamTab = "pn"
)

// announceStreamDisplay ports Python's AnnounceStream widget (Network.py:394-
// 551): a Pile of [tab_bar, filter_bar, IndicativeListBox] shown inside the
// bordered "Announce Stream" slot. The tab_bar is a urwid Columns of three
// TabButtons "Nodes (N)" / "Peers (N)" / "Propagation Nodes (N)" (weights
// 1:1:3, dividechars 1); the filter_bar is a urwid Columns of a "Search: "
// ReadlineEdit (weight 2) and a "Show: Name" display-toggle TabButton (weight
// 1, dividechars 1). The list shows only entries whose announce type matches
// the active tab and whose app data contains the (lowercased) search text, and
// the tab labels carry the per-type counts (Network.py:481-525). The display
// mode (Name vs Dest) is shared with NetworkDisplay.displayMode.
type announceStreamDisplay struct {
	nd *NetworkDisplay // back-ref for announce data, display mode, glyphs

	tabNodes  *UrwidButton
	tabPeers  *UrwidButton
	tabPN     *UrwidButton
	tabBar    *urwidColumns
	search    *ReadlineEdit
	toggle    *UrwidButton
	filterBar *urwidColumns
	ilb       *IndicativeListBox // == nd.announcesList
	pile      *tview.Flex

	currentTab announceStreamTab
	searchText string
	entries    []AnnounceEntry // the filtered list currently shown (index maps to list rows)
	tabBarH    int             // cached tab-bar row count for the last width
	lastWidth  int
}

// newAnnounceStreamDisplay builds the AnnounceStream Pile around the network
// display's existing announce IndicativeListBox.
func newAnnounceStreamDisplay(nd *NetworkDisplay) *announceStreamDisplay {
	as := &announceStreamDisplay{
		nd:         nd,
		ilb:        nd.announcesList,
		currentTab: tabNodes,
	}

	as.tabNodes = NewTabButton("Nodes (0)").SetSelectedFunc(as.showNodesTab)
	as.tabPeers = NewTabButton("Peers (0)").SetSelectedFunc(as.showPeersTab)
	as.tabPN = NewTabButton("Propagation Nodes (0)").SetSelectedFunc(as.showPNTab)

	as.tabBar = newURWIDColumns(1, as.tabNodes, as.tabPeers, as.tabPN).
		SetWeight(0, 1).SetWeight(1, 1).SetWeight(2, 3)

	as.search = NewReadlineEdit(nd.app.killRing, "Search: ", "")
	as.search.SetChangedFunc(func(_ string) { as.onSearchChange() })

	as.toggle = NewTabButton(as.nd.DisplayModeLabel()).SetSelectedFunc(as.toggleDisplayMode)
	as.filterBar = newURWIDColumns(1, as.search, as.toggle).
		SetWeight(0, 2).SetWeight(1, 1)

	// Pile [tab_bar (pack), filter_bar (1), ilb (weight 1)]. The ilb is the
	// focused item so the list retains keyboard focus; tab switching via the
	// keyboard (Up from the list → filter → tabs → header) is deferred.
	as.pile = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(as.tabBar, 2, 0, false).
		AddItem(as.filterBar, 1, 0, false).
		AddItem(as.ilb, 0, 1, true)

	// Resize the tab_bar to its wrapped row count before the Flex lays out
	// children (the DrawFunc runs in Box.DrawForSubclass, before Flex.Draw).
	// At inner width 50 the weight-1 tabs wrap "Nodes (N)" to 2 rows and the
	// weight-3 tab fits on 1, so the bar is 2 rows; this keeps it correct if
	// the width or counts change.
	as.pile.SetDrawFunc(func(screen tcell.Screen, x, y, w, h int) (int, int, int, int) {
		tbh := as.tabBar.requiredHeightAt(w)
		if tbh < 1 {
			tbh = 1
		}
		if tbh != as.tabBarH || w != as.lastWidth {
			as.tabBarH = tbh
			as.lastWidth = w
			as.pile.ResizeItem(as.tabBar, tbh, 0)
		}
		as.pile.ResizeItem(as.filterBar, 1, 0)
		return x, y, w, h
	})

	as.update()
	return as
}

// Widget returns the Pile primitive to swap into the bordered list slot.
func (as *announceStreamDisplay) Widget() tview.Primitive { return as.pile }

// entryAt returns the filtered entry currently at list row i.
func (as *announceStreamDisplay) entryAt(i int) (AnnounceEntry, bool) {
	if i < 0 || i >= len(as.entries) {
		return AnnounceEntry{}, false
	}
	return as.entries[i], true
}

// selectedEntry returns the entry currently selected in the announce list.
func (as *announceStreamDisplay) selectedEntry() (AnnounceEntry, bool) {
	if as.ilb == nil || as.ilb.List == nil || as.ilb.List.GetItemCount() == 0 {
		return AnnounceEntry{}, false
	}
	idx := as.ilb.List.GetCurrentItem()
	return as.entryAt(idx)
}

// onSearchChange mirrors Python's on_search_change (Network.py:456-458): lower-
// case the text and rebuild the list.
func (as *announceStreamDisplay) onSearchChange() {
	as.searchText = strings.ToLower(as.search.GetText())
	as.update()
}

// showNodesTab/showPeersTab/showPNTab switch the active tab and rebuild
// (Network.py:530-540).
func (as *announceStreamDisplay) showNodesTab() { as.currentTab = tabNodes; as.update() }
func (as *announceStreamDisplay) showPeersTab() { as.currentTab = tabPeers; as.update() }
func (as *announceStreamDisplay) showPNTab()    { as.currentTab = tabPN; as.update() }

// SetCurrentTab switches the active tab programmatically (used by tests/wiring).
func (as *announceStreamDisplay) SetCurrentTab(tab announceStreamTab) {
	as.currentTab = tab
	as.update()
}

// toggleDisplayMode flips Name/Dest, sharing NetworkDisplay.displayMode as the
// source of truth (Network.py:460-466).
func (as *announceStreamDisplay) toggleDisplayMode() {
	as.nd.displayMode = ToggleDisplayMode(as.nd.displayMode)
	as.toggle.SetLabel(as.nd.DisplayModeLabel())
	as.update()
}

// update mirrors Python's update_widget_list (Network.py:481-528): count entries
// by type, filter by the active tab + search text, repopulate the list, and set
// the tab labels with the per-type counts.
func (as *announceStreamDisplay) update() {
	data := as.nd.announceData
	nodeCount, peerCount, pnCount := 0, 0, 0
	as.entries = as.entries[:0]

	now := time.Now()
	showHash := as.nd.displayMode == DisplayHash
	g := as.nd.glyphs()

	for _, e := range data {
		// Python matches the search against the announce app_data (e[2]).
		if as.searchText != "" {
			hay := strings.ToLower(e.AppData)
			if !strings.Contains(hay, as.searchText) {
				continue
			}
		}
		switch e.Type {
		case "node":
			nodeCount++
			if as.currentTab == tabNodes {
				as.entries = append(as.entries, e)
			}
		case "peer":
			peerCount++
			if as.currentTab == tabPeers {
				as.entries = append(as.entries, e)
			}
		case "pn":
			pnCount++
			if as.currentTab == tabPN {
				as.entries = append(as.entries, e)
			}
		}
	}

	as.ilb.List.Clear()
	for _, e := range as.entries {
		text := FormatAnnounceStreamRow(e, now, showHash, as.nd.SanitizeNames, g)
		as.ilb.List.AddItem(text, "", 0, nil)
	}

	// Empty state: a centered "No <tab> announces" (Network.py:521).
	if len(as.entries) == 0 {
		as.ilb.SetEmptyText(fmt.Sprintf("No %s announces", as.currentTab))
	} else {
		as.ilb.SetEmptyText("")
	}

	as.tabNodes.SetLabel(fmt.Sprintf("Nodes (%d)", nodeCount))
	as.tabPeers.SetLabel(fmt.Sprintf("Peers (%d)", peerCount))
	as.tabPN.SetLabel(fmt.Sprintf("Propagation Nodes (%d)", pnCount))
}
