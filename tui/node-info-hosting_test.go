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

// nodeInfoHostingData returns a NodeInfoData for the hosting branch with
// deterministic stat providers, matching Python's NodeInfo pile labels
// (Network.py:1473-1517).
func nodeInfoHostingData() NodeInfoData {
	return NodeInfoData{
		HasNode:            true,
		Addr:               "aabbccdd11223344",
		Name:               "MyNode",
		DisablePropagation: false,
		LXMFPropAddr:       "<aa11..22>",
		LastAnnounce:       func() string { return "5 minutes ago" },
		StorageStats:       func() string { return "12.5%, 5.00 MB of 40.00 MB" },
		ActiveLinks:        func() string { return "3" },
		TotalConnects:      func() string { return "42" },
		TotalPages:         func() string { return "100" },
		TotalFiles:         func() string { return "7" },
	}
}

// TestNodeInfoHostingLayout pins the "Local Node Info" hosting branch with
// propagation enabled against Python's NodeInfo pile (Network.py:1473-1494):
// Addr, Name, divider, the centered LXMF propagation address, divider, the six
// stat lines (Last Announce / LXMF Storage / Connected Now / Total Connects /
// Served Pages / Served Files), divider, and the Back/Browse/Rst Stats/Announce
// button row. Labels align the colon at column 16 (value at 17), matching
// Python's per-widget titles.
func TestNodeInfoHostingLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	ni := NewNodeInfoDisplay(app, nodeInfoHostingData())

	// 13 content rows + 2 border rows = 15.
	if got := ni.Height(); got != 15 {
		t.Fatalf("Height = %v, want 15 (13 content + 2 border)", got)
	}

	rows := renderNodeInfo(t, ni, 50, ni.Height())

	// Title in the top border.
	if !strings.Contains(rows[0], "Local Node Info") {
		t.Errorf("row 0 = %q, want title %q", rows[0], "Local Node Info")
	}

	// Collect the rendered text (strip borders) to assert label order.
	joined := strings.Join(rows, "\n")

	wantLabels := []string{
		"Addr : aabbccdd11223344",
		"Name : MyNode",
		"Last Announce  : 5 minutes ago",
		"LXMF Storage   : 12.5%, 5.00 MB of 40.00 MB",
		"Connected Now  : 3",
		"Total Connects : 42",
		"Served Pages   : 100",
		"Served Files   : 7",
	}
	for _, want := range wantLabels {
		if !strings.Contains(joined, want) {
			t.Errorf("rendered panel missing %q", want)
		}
	}

	// The LXMF propagation address line is present (with propagation enabled).
	if !strings.Contains(joined, "LXMF Propagation Node Address is") {
		t.Errorf("rendered panel missing the LXMF propagation address line")
	}
	if !strings.Contains(joined, "<aa11..22>") {
		t.Errorf("rendered panel missing the LXMF propagation address %q", "<aa11..22>")
	}

	// All four buttons are present.
	for _, label := range []string{"Back", "Browse", "Rst Stats", "Announce"} {
		if !strings.Contains(joined, label) {
			t.Errorf("rendered panel missing the %q button", label)
		}
	}
}

// TestNodeInfoHostingLayoutDisabledPropagation pins the hosting branch with
// propagation disabled (Network.py:1497-1517): the LXMF propagation address
// line is omitted, leaving 11 content rows + 2 border = 13.
func TestNodeInfoHostingLayoutDisabledPropagation(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode
	d := nodeInfoHostingData()
	d.DisablePropagation = true
	ni := NewNodeInfoDisplay(app, d)

	if got := ni.Height(); got != 13 {
		t.Fatalf("Height = %v, want 13 (11 content + 2 border, no LXMF line)", got)
	}

	rows := renderNodeInfo(t, ni, 50, ni.Height())
	joined := strings.Join(rows, "\n")

	if strings.Contains(joined, "LXMF Propagation Node Address is") {
		t.Error("LXMF propagation address line present when propagation disabled; want omitted")
	}
	// Stat labels still present.
	for _, want := range []string{
		"Last Announce  : 5 minutes ago",
		"LXMF Storage   : 12.5%, 5.00 MB of 40.00 MB",
		"Connected Now  : 3",
		"Total Connects : 42",
		"Served Pages   : 100",
		"Served Files   : 7",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("rendered panel missing %q", want)
		}
	}
}

// TestNodeInfoHostingButtonCallbacks verifies the Announce, Browse, and Rst
// Stats buttons fire their callbacks when activated.
func TestNodeInfoHostingButtonCallbacks(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	d := nodeInfoHostingData()
	var fired []string
	d.OnAnnounce = func() { fired = append(fired, "announce") }
	d.OnBrowse = func() { fired = append(fired, "browse") }
	d.OnResetStats = func() { fired = append(fired, "reset") }
	ni := NewNodeInfoDisplay(app, d)

	for _, label := range []string{"Announce", "Browse", "Rst Stats"} {
		btn := ni.actionButton(label)
		if btn == nil {
			t.Fatalf("action button %q not found", label)
		}
		handler := btn.InputHandler()
		if handler == nil {
			t.Fatalf("%q button has no input handler", label)
		}
		handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	}
	want := []string{"announce", "browse", "reset"}
	if len(fired) != len(want) {
		t.Fatalf("fired = %v, want %v", fired, want)
	}
	for i, w := range want {
		if fired[i] != w {
			t.Errorf("fired[%v] = %q, want %q", i, fired[i], w)
		}
	}
}

// TestNodeInfoHostingRefresh verifies the 1s refresh ticker re-reads the stat
// providers and updates the displayed lines. The providers return a value
// that changes on each call, so a refresh must surface the new value.
func TestNodeInfoHostingRefresh(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode

	count := 0
	d := nodeInfoHostingData()
	d.TotalConnects = func() string {
		count++
		return "iter-" + itoa(count)
	}
	ni := NewNodeInfoDisplay(app, d)

	// Initial render already read the provider once.
	if count < 1 {
		t.Fatalf("provider not called at construction; count=%v", count)
	}
	before := count

	// A manual refresh re-reads the providers.
	ni.refreshStats()
	if count <= before {
		t.Errorf("refreshStats did not re-read the provider: count=%v before=%v", count, before)
	}

	// The new value is reflected in the rendered output.
	rows := renderNodeInfo(t, ni, 50, ni.Height())
	joined := strings.Join(rows, "\n")
	want := "Total Connects : iter-" + itoa(count)
	if !strings.Contains(joined, want) {
		t.Errorf("refresh did not surface new value: missing %q in render", want)
	}
}

// TestNodeInfoHostingRefreshTicker verifies Start begins the periodic refresh
// and Stop halts it (the ticker must not keep calling providers after Stop).
func TestNodeInfoHostingRefreshTicker(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode

	d := nodeInfoHostingData()
	calls := 0
	d.ActiveLinks = func() string { calls++; return "x" }
	ni := NewNodeInfoDisplay(app, d)
	afterConstruction := calls

	ni.start(false) // synchronous (no event loop in tests; QueueUpdateDraw would block)
	time.Sleep(1200 * time.Millisecond)
	ni.Stop()
	afterRun := calls
	if afterRun <= afterConstruction {
		t.Fatalf("ticker did not fire: calls=%v after construction=%v", afterRun, afterConstruction)
	}

	// After Stop, the ticker must not keep calling the provider.
	time.Sleep(400 * time.Millisecond)
	if calls > afterRun+1 {
		t.Errorf("ticker kept firing after Stop: calls=%v afterRun=%v", calls, afterRun)
	}
}
