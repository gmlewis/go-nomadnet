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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// newD1App mounts a NetworkDisplay in a fully wired app, switches to the
// Announce Stream, and focuses the announce list (the state after two Downs
// from the tab bar per Python's AnnounceStream Pile, Network.py:437-441).
func newD1App(t *testing.T) (*App, *NetworkDisplay) {
	t.Helper()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	app.Main.SetDisplay("network", nd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("network")
	app.Main.FocusBody()
	nd.toggleList() // Saved Nodes → Announce Stream
	nd.UpdateAnnounces([]AnnounceEntry{
		{DisplayName: "D1 Node", SourceHash: "d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1", Type: "node", Timestamp: time.Unix(1700000000, 0)},
	})
	app.Main.Root().Draw(screen)

	// Walk the pile like the user does: two Downs (tab bar → filter bar →
	// list) from the pile's initial focus — Python's AnnounceStream Pile
	// (Network.py:437-441) starts on the tab bar and the announce list needs
	// two Downs before Enter opens an Announce Info.
	if as := nd.announceStream; as == nil || as.pile == nil {
		t.Fatal("announce stream pile missing")
	} else {
		as.pile.SetFocusIndex(0)
		app.SetFocus(as.pile)
		h := as.pile.InputHandler()
		setFocus := func(p tview.Primitive) { app.SetFocus(p) }
		h(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), setFocus)
		h(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), setFocus)
	}
	app.Main.Root().Draw(screen)
	return app, nd
}

// TestD1EnterOnAnnounceOpensInfo pins D1: Enter on an announce in the
// Announce Stream opens the Announce Info view (Python AnnounceInfo,
// Network.py:59-256 — Time/Addr/Type/Name/Oprtr/Trust rows + Announce Data +
// a Back/Connect/Msg Op/Save button row). The earlier Go build swallowed the
// Enter on the announce-stream pileFiller, so the overlay never opened.
func TestD1EnterOnAnnounceOpensInfo(t *testing.T) {
	t.Parallel()

	app, nd := newD1App(t)

	if nd.inInfoView {
		t.Fatal("Announce Info must not be open before Enter")
	}

	// Dispatch Enter through the real chain: mainCols → left panel → pile →
	// announce list (the same dispatch the event loop uses).
	handler := nd.mainCols.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })

	if !nd.inInfoView {
		t.Fatal("Enter on an announce did not open the Announce Info view (key swallowed)")
	}

	// The Announce Info rows must include the announce's fields and the
	// Back/Connect/Msg Op/Save buttons.
	info := nd.listBox.GetItem(0)
	rows := dialogRowTexts(info)
	joined := ""
	for _, r := range rows {
		joined += r + "\n"
	}
	for _, want := range []string{"Time", "Addr", "Type", "Name", "Trust"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Announce Info missing %q row; rows: %q", want, rows)
		}
	}
	for _, want := range []string{"Back", "Connect", "Msg Op", "Save"} {
		if !dialogHasButton(info, want) {
			t.Errorf("Announce Info missing the %q button", want)
		}
	}
}
