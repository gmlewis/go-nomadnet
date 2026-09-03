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
	"github.com/rivo/tview"
)

// TestIndicativeListBoxSkipOverSpacer pins the skip-unselectable traversal on
// a channels-shaped list [hub, room, spacer, hub]: Down must walk hub → room →
// (jump the spacer) → hub. The live fleet bug: with a room row present, the
// second Down stopped advancing, leaving the cursor stuck on the room row so
// the RaspPi hub row could never be reached on the local node.
func TestIndicativeListBoxSkipOverSpacer(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}},
		fakeHub{name: "Hub B", status: hubStatusDisconnected},
	})
	// Rows: [Hub A(0), #general(1), spacer(2), Hub B(3)].
	if got := cd.rooms.GetCurrentItem(); got != 0 {
		t.Fatalf("fresh list should start at item 0, got %v", got)
	}

	setFocus := func(p tview.Primitive) { app.SetFocus(p) }
	fire := func() {
		if h := cd.ilb.InputHandler(); h != nil {
			h(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), setFocus)
		}
	}
	fire()
	if got := cd.rooms.GetCurrentItem(); got != 1 {
		t.Fatalf("Down #1 should move to the room row (1), got %v", got)
	}
	fire()
	if got := cd.rooms.GetCurrentItem(); got != 3 {
		t.Fatalf("Down #2 must jump the spacer to Hub B (3), got %v", got)
	}
	fire()
	if got := cd.rooms.GetCurrentItem(); got != 3 {
		t.Fatalf("Down #3 at the last row should stay (3), got %v", got)
	}
}
