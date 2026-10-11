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
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"testing"
)

// TestInterfaceShowStatusGoesLive pins the details view against the report from
// the tablet: the radio was plugged in, the list page moved the RNode row to
// Connected within a second, and the details view that was open at the time went
// on saying "Disconnected".
//
// Python's ShowInterface is live. Its start() arms poll_stats on a 1 s alarm
// (Interfaces.py:2921-2925) and poll_stats re-arms itself (Interfaces.py:2999),
// so the Type/Status rows and the TX/RX counters follow the interface. Go built
// the view once from a captured InterfaceInfo and never revisited it, which left
// the one screen whose whole purpose is a live reading as the only stale one.
func TestInterfaceShowStatusGoesLive(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	id := NewInterfacesDisplay(app, []InterfaceInfo{
		{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: false,
			Params: map[string]string{"port": "/dev/ttyACM0"}},
	})
	app.Main.SetDisplay("interfaces", id.Widget())
	app.SetRoot()

	id.ShowInterfaceDetail(id.Items()[0])
	show := id.showWidget
	if show == nil {
		t.Fatal("ShowInterface view not open")
	}

	before := show.body.GetText(true)
	if !strings.Contains(before, "Disconnected") {
		t.Fatalf("setup: the details view does not show the interface as disconnected:\n%v", before)
	}

	// The radio comes up: the transport's next snapshot has it connected and
	// counting bytes.
	id.SetInterfaces([]InterfaceInfo{
		{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: true,
			TX: 4096, RX: 8192,
			Params: map[string]string{"port": "/dev/ttyACM0"}},
	})

	after := show.body.GetText(true)
	if strings.Contains(after, "Disconnected") {
		t.Errorf("the open details view still reports the interface as disconnected after it came up:\n%v", after)
	}
	if !strings.Contains(after, "Connected") {
		t.Errorf("the open details view does not report the interface as connected:\n%v", after)
	}
	for _, want := range []string{"4.0 KB", "8.0 KB"} {
		if !strings.Contains(after, want) {
			t.Errorf("the open details view did not pick up the new counter %v:\n%v", want, after)
		}
	}
}

// TestInterfaceShowRefreshesOnlyItsOwnInterface guards the other half: an
// unrelated interface's snapshot must not rewrite the view on screen.
func TestInterfaceShowRefreshesOnlyItsOwnInterface(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	id := NewInterfacesDisplay(app, []InterfaceInfo{
		{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: false},
		{Name: "Home Hub", Type: "TCPClientInterface", Enabled: true, Connected: true},
	})
	app.Main.SetDisplay("interfaces", id.Widget())
	app.SetRoot()

	id.ShowInterfaceDetail(id.Items()[0])
	show := id.showWidget

	id.SetInterfaces([]InterfaceInfo{
		{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: false},
		{Name: "Home Hub", Type: "TCPClientInterface", Enabled: true, Connected: true, TX: 999},
	})

	got := show.body.GetText(true)
	if strings.Contains(got, "999") {
		t.Errorf("another interface's counters leaked into the open details view:\n%v", got)
	}
	if !strings.Contains(got, "Type:    RNodeInterface") {
		t.Errorf("the details view no longer shows the interface it was opened on:\n%v", got)
	}
}
