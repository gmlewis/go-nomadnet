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

	"github.com/gmlewis/tcell/v2"
)

// TestHubInfoRendersRoomTopicsAndLinks pins the hub-info panel's room area:
// every #room in the MOTD is a clickable region (Python _linkify_motd with the
// chat link delegate, Channels.py:1229-1240,1795-1801), joined and available
// room rows are clickable (ChannelListEntry), and an advertised room's topic
// renders under it (Channels.py:1806-1824).
func TestHubInfoRendersRoomTopicsAndLinks(t *testing.T) {
	t.Parallel()

	hia := NewHubInfoArea(newTestApp(), "hub")
	var opened []string
	hia.OnOpenRoom = func(room string) { opened = append(opened, room) }

	hia.SetHubInfo(HubInfoSnapshot{
		Name:            "hub",
		Status:          2,
		MOTD:            "Welcome! Join #general and say hi",
		JoinedRooms:     []string{"lobby"},
		AvailRooms:      []string{"general"},
		AvailRoomTopics: map[string]string{"general": "General chat"},
	})

	raw := hia.view.GetText(false)
	for _, want := range []string{
		`["motdroom0"]#general[""]`,
		`["joinedroom0"]#lobby[""]`,
		`["availroom0"]#general[""]`,
		"General chat",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("hub-info text is missing %q:\n%v", want, raw)
		}
	}

	// A click on a region highlights it, and the handler opens its room. Draw
	// first: tview resolves region ids against the parsed lines, which only
	// exist after a draw at a known width.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	hia.Widget().SetRect(0, 0, 100, 40)
	hia.Widget().Draw(screen)

	hia.view.Highlight("motdroom0")
	hia.view.Highlight("joinedroom0")
	hia.view.Highlight("availroom0")
	want := []string{"general", "lobby", "general"}
	if len(opened) != len(want) {
		t.Fatalf("clicked regions opened %v, want %v", opened, want)
	}
	for i := range want {
		if opened[i] != want[i] {
			t.Errorf("region %v opened %q, want %q", i, opened[i], want[i])
		}
	}
}

// TestLinkifyMOTDLeavesWordPrefixedRoomsAlone pins the lookbehind behavior of
// the span scanner.
func TestLinkifyMOTDLeavesWordPrefixedRoomsAlone(t *testing.T) {
	t.Parallel()

	if got, want := LinkifyMOTD("x#notaroom #yes [#linked]"), "x#notaroom `[#yes`room://yes] [#linked]"; got != want {
		t.Errorf("LinkifyMOTD = %q, want %q", got, want)
	}
}
