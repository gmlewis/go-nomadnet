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
)

func TestDisplayModeToggle(t *testing.T) {
	t.Parallel()

	mode := DisplayName
	if mode != DisplayName {
		t.Error("initial mode should be DisplayName")
	}

	mode = ToggleDisplayMode(mode)
	if mode != DisplayHash {
		t.Errorf("after toggle: got %v, want DisplayHash(%v)", mode, DisplayHash)
	}

	mode = ToggleDisplayMode(mode)
	if mode != DisplayName {
		t.Errorf("after second toggle: got %v, want DisplayName(%v)", mode, DisplayName)
	}
}

// TestNetworkToggleListFiresOnToggleList pins Python NetworkLeftPile.keypress
// "ctrl l" → NetworkDisplay.toggle_list (Network.py:1600-1601 + 1668-1678):
// the display swaps the left-pane widget between the Announce Stream and
// Saved Nodes (boot state = Saved Nodes, Python list_display=1), then the
// OnToggleList callback fires so the wiring layer can refresh the
// newly-shown list's data.
func TestNetworkToggleListFiresOnToggleList(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	nd := NewNetworkDisplay(app, []AnnounceEntry{
		{DisplayName: "A", SourceHash: "aa", Timestamp: time.Now()},
	}, nil)

	if !nd.ShowingNodes() {
		t.Fatal("the Network page boots on the Saved Nodes list (Python list_display=1)")
	}
	var toggles int
	nd.OnToggleList = func() { toggles++ }

	nd.toggleList()
	if nd.ShowingNodes() {
		t.Error("the first toggleList should show the Announce Stream")
	}
	if toggles != 1 {
		t.Errorf("OnToggleList fired %v times, want 1", toggles)
	}

	nd.toggleList()
	if !nd.ShowingNodes() {
		t.Error("second toggleList should restore the Saved Nodes list")
	}
	if toggles != 2 {
		t.Errorf("OnToggleList fired %v times, want 2", toggles)
	}
}

// TestNetworkUseAsPNPassesAnnounce pins Python use_pn (Network.py:189-194):
// the AnnounceInfo "Use as default" action pops back to the announce stream
// and hands the ANNOUNCE's source hash to the callback so the wiring layer
// can set it as the user-selected propagation node.
func TestNetworkUseAsPNPassesAnnounce(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	ann := AnnounceEntry{
		DisplayName: "PN1",
		Type:        "pn",
		SourceHash:  "bbaabbcc",
		Timestamp:   time.Now(),
	}
	nd := NewNetworkDisplay(app, []AnnounceEntry{ann}, nil)

	var got AnnounceEntry
	var called bool
	nd.OnUseAsPN = func(a AnnounceEntry) { called = true; got = a }

	nd.showAnnounceDetail(0)
	if !nd.inInfoView {
		t.Fatal("showAnnounceDetail did not enter info view")
	}
	nd.useAsPN(ann)

	if !called {
		t.Fatal("OnUseAsPN did not fire")
	}
	if got.SourceHash != "bbaabbcc" {
		t.Errorf("OnUseAsPN announce = %+v, want the announce the button acted on", got)
	}
	if nd.inInfoView {
		t.Error("useAsPN should return to the stream view (Python show_announce_stream)")
	}
}
