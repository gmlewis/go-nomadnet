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
)

// chartSpaces is a helper for the golden chart rows below: the graph body is a
// fixed run of spaces between the axis column and the plotted line.
func chartSpaces(n int) string { return strings.Repeat(" ", n) }

// TestInterfaceBandwidthChartGetCharts pins the 8-row traffic chart and peak
// labels against Python's InterfaceBandwidthChart.get_charts(height=8)
// (Interfaces.py:1324-1358). The expected strings were captured by running the
// vendored AsciiChart (nomadnet/vendor/AsciiChart.py) with the same state:
// rx_rates = [0]*59 + [1200], tx_rates = [0]*58 + [800, 0], max_rx_rate=1200,
// max_tx_rate=800, initialisation complete, peak_rx=1200, peak_tx=800, and
// format=RNS.prettyspeed. The chart height of 8 yields 9 rendered rows
// (rows = max2 - min2 = 8, plus the label row); each graph row is 73 columns.
func TestInterfaceBandwidthChartGetCharts(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(60)
	for i := range c.rxRates {
		c.rxRates[i] = 0
		c.txRates[i] = 0
	}
	c.rxRates[59] = 1200
	c.txRates[58] = 800
	c.txRates[59] = 0
	c.maxRXRate = 1200
	c.maxTXRate = 800
	c.initializationComplete = true
	c.peakRXForDisplay = 1200
	c.peakTXForDisplay = 800

	rxChart, txChart, peakRX, peakTX := c.GetCharts(8)

	if peakRX != "9.60 Kbps" {
		t.Errorf("peakRX = %q, want %q", peakRX, "9.60 Kbps")
	}
	if peakTX != "6.40 Kbps" {
		t.Errorf("peakTX = %q, want %q", peakTX, "6.40 Kbps")
	}

	rxLine := func(label, sym string) string { return label + " ┤" + chartSpaces(58) + sym }
	wantRX := strings.Join([]string{
		"   1.32 Kbps ┼",
		rxLine("   1.16 Kbps", "╭"),
		rxLine("     990 bps", "│"),
		rxLine("     825 bps", "│"),
		rxLine("     660 bps", "│"),
		rxLine("     495 bps", "│"),
		rxLine("     330 bps", "│"),
		rxLine("     165 bps", "│"),
		"       0 bps ┼" + strings.Repeat("─", 58) + "╯",
	}, "\n")
	if rxChart != wantRX {
		t.Errorf("rxChart =\n%v\nwant\n%v", rxChart, wantRX)
	}

	txLine := func(label string) string { return label + " ┤" + chartSpaces(57) + "││" }
	wantTX := strings.Join([]string{
		"     880 bps ┼",
		"     770 bps ┤" + chartSpaces(57) + "╭╮",
		txLine("     660 bps"),
		txLine("     550 bps"),
		txLine("     440 bps"),
		txLine("     330 bps"),
		txLine("     220 bps"),
		txLine("     110 bps"),
		"       0 bps ┼" + strings.Repeat("─", 57) + "╯╰",
	}, "\n")
	if txChart != wantTX {
		t.Errorf("txChart =\n%v\nwant\n%v", txChart, wantTX)
	}
}

// TestInterfaceBandwidthChartGetChartsBeforeInit pins the pre-initialisation
// readout: the peak labels read "0 bps" until two real samples have arrived
// (Python: peak_*_for_display is only consulted when initialization_complete).
func TestInterfaceBandwidthChartGetChartsBeforeInit(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(60)
	_, _, peakRX, peakTX := c.GetCharts(8)
	if peakRX != "0 bps" || peakTX != "0 bps" {
		t.Errorf("pre-init peaks = (%q, %q), want (0 bps, 0 bps)", peakRX, peakTX)
	}
}
