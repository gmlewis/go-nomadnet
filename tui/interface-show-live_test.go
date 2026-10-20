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
	"time"
)

// TestInterfaceShowStatusGoesLive pins that an open details view follows the
// interface's state instead of the snapshot it was opened with.
//
// Python's ShowInterface is live: its start() arms poll_stats on a 1 s alarm
// (Interfaces.py:2921-2925) and poll_stats re-arms itself (Interfaces.py:2999),
// so the Type/Status rows and the TX/RX counters follow the interface. The Go
// view must be refreshed the same way, or the one screen whose whole purpose is
// a live reading is the only stale one.
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

// TestInterfaceShowLiveTrafficCharts pins that the details view's RX/TX traffic
// boxes are fed from the interface's byte counters rather than left as a static
// "Loading RX data..." box reading "Peak: 0 B/s" forever (interfaceShow.
// chartsRows). Python's ShowInterface keeps an InterfaceBandwidthChart per
// interface (Interfaces.py:2308), samples it from the interface's rx/tx byte
// counters on a 1 s alarm, and renders get_charts(height=8) plus the peak rate
// on each side (update_bandwidth_charts, Interfaces.py:2759-2790). This test
// drives a deterministic sample series through the view's chart and checks the
// rendered box contents advance: the loading placeholder gives way to a chart
// body, and the peak row follows the series.
func TestInterfaceShowLiveTrafficCharts(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	app.GlyphSet = GlyphUnicode
	id := NewInterfacesDisplay(app, []InterfaceInfo{
		{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: true},
	})
	id.ShowInterfaceDetail(id.Items()[0])
	show := id.showWidget
	if show == nil {
		t.Fatal("ShowInterface view not open")
	}
	base := time.Unix(1700000000, 0)
	clock := base
	show.now = func() time.Time { return clock }

	// Before any poll the box shows the loading placeholder (Python's
	// rx_chart_text default, Interfaces.py:2313).
	rxRows, _ := show.chartBoxRows()
	if len(rxRows) != 2 || rxRows[0].text != "Loading RX data..." {
		t.Fatalf("pre-poll RX box rows = %+v, want the loading placeholder", rxRows)
	}

	// Feed a deterministic sample series: two real updates give rates of
	// 8000 bits/s RX (1000 B/s) and 4000 bits/s TX (500 B/s) on the second
	// sample, with the max so far equal to those values.
	clock = base
	show.SetInfo(InterfaceInfo{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: true, RX: 1000, TX: 500})
	clock = base.Add(time.Second)
	show.SetInfo(InterfaceInfo{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: true, RX: 2000, TX: 1000})

	rxRows, _ = show.chartBoxRows()
	if len(rxRows) < 3 {
		t.Fatalf("RX box rows after polling = %d, want a chart body (>2 rows): %+v", len(rxRows), rxRows)
	}
	// 1000 B/s * 8 = 8000 bits/s → "8.00 Kbps"; the max-rate axis label is
	// 1.1x the max, i.e. 8800 bits/s → "8.80 Kbps".
	if !strings.Contains(rxRows[0].text, "Kbps") {
		t.Errorf("RX chart top row = %q, want a Kbps axis label", rxRows[0].text)
	}
	lastRX := rxRows[len(rxRows)-1]
	if lastRX.align != boxRight || !strings.HasPrefix(lastRX.text, "Peak: ") {
		t.Errorf("RX box last row = %+v, want a right-aligned Peak row", lastRX)
	}
	if lastRX.text == "Peak: 0 B/s" || strings.HasSuffix(lastRX.text, "0 bps") {
		t.Errorf("RX peak row = %q, want a non-zero peak after two samples", lastRX.text)
	}

	// A third sample with a larger delta must advance the peak and the max.
	clock = base.Add(2 * time.Second)
	show.SetInfo(InterfaceInfo{Name: "RNode LoRa", Type: "RNodeInterface", Enabled: true, Connected: true, RX: 5000, TX: 1000})
	rxRows2, _ := show.chartBoxRows()
	if rxRows2[len(rxRows2)-1].text == lastRX.text {
		t.Errorf("RX peak row did not advance after a larger sample: still %q", lastRX.text)
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
