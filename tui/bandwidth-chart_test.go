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

func TestPrettySpeed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input float64
		want  string
	}{
		{"zero", 0, "0 bps"},
		{"small bps", 500, "500 bps"},
		{"1 Kbps", 1000, "1.0 Kbps"},
		{"1.5 Kbps", 1500, "1.5 Kbps"},
		{"1 Mbps", 1000000, "1.0 Mbps"},
		{"1 Gbps", 1000000000, "1.0 Gbps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := PrettySpeed(tt.input)
			if got != tt.want {
				t.Errorf("PrettySpeed(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBandwidthChartFirstUpdate(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(5)
	now := time.Now()
	c.Update(1000, 2000, now)

	if c.updateCount != 0 {
		t.Errorf("updateCount after first update = %v, want 0", c.updateCount)
	}
	if c.initializationComplete {
		t.Error("should not be initialized after first update")
	}
}

func TestBandwidthChartSubsequentUpdates(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(5)
	base := time.Now()

	c.Update(1000, 2000, base)
	c.Update(11000, 22000, base.Add(time.Second))

	if c.updateCount != 1 {
		t.Errorf("updateCount = %v, want 1", c.updateCount)
	}
	if c.initializationComplete {
		t.Error("should not be initialized after only 1 real update (stabilization=2)")
	}

	c.Update(21000, 42000, base.Add(2*time.Second))
	if !c.initializationComplete {
		t.Error("should be initialized after 2 real updates (stabilization=2)")
	}

	rx, tx := c.Rates()
	lastRX := rx[len(rx)-1]
	lastTX := tx[len(tx)-1]

	if lastRX <= 0 {
		t.Errorf("last rx rate = %v, want > 0", lastRX)
	}
	if lastTX <= 0 {
		t.Errorf("last tx rate = %v, want > 0", lastTX)
	}
}

func TestBandwidthChartRateCalculation(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(3)
	base := time.Now()

	c.Update(0, 0, base)
	c.Update(1000, 5000, base.Add(time.Second))

	rx, tx := c.Rates()
	lastRX := rx[len(rx)-1]
	lastTX := tx[len(tx)-1]

	wantRXBits := 1000.0 * 8
	wantTXBits := 5000.0 * 8

	if lastRX != wantRXBits {
		t.Errorf("rx rate = %v, want %v", lastRX, wantRXBits)
	}
	if lastTX != wantTXBits {
		t.Errorf("tx rate = %v, want %v", lastTX, wantTXBits)
	}
}

func TestBandwidthChartHistoryRolls(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(3)
	base := time.Now()

	c.Update(0, 0, base)
	c.Update(100, 100, base.Add(time.Second))
	c.Update(200, 200, base.Add(2*time.Second))
	c.Update(300, 300, base.Add(3*time.Second))

	rx, _ := c.Rates()
	if len(rx) != 3 {
		t.Errorf("rx history length = %v, want 3", len(rx))
	}
}

func TestBandwidthChartPeaks(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(5)
	base := time.Now()

	rxStr, txStr := c.Peaks()
	if rxStr != "0 bps" || txStr != "0 bps" {
		t.Errorf("peaks before init = (%q, %q), want (0 bps, 0 bps)", rxStr, txStr)
	}

	c.Update(0, 0, base)
	c.Update(1000, 5000, base.Add(time.Second))
	c.Update(2000, 10000, base.Add(2*time.Second))

	rxStr, _ = c.Peaks()
	if rxStr == "0 bps" {
		t.Error("rx peak should not be 0 bps after initialization")
	}
}

func TestBandwidthChartMaxRates(t *testing.T) {
	t.Parallel()

	c := NewInterfaceBandwidthChart(5)
	base := time.Now()

	c.Update(0, 0, base)
	c.Update(1000, 5000, base.Add(time.Second))
	c.Update(2000, 10000, base.Add(2*time.Second))

	rxMax, txMax := c.MaxRates()
	if rxMax < 1 {
		t.Errorf("rx max = %v, want >= 1", rxMax)
	}
	if txMax < 1 {
		t.Errorf("tx max = %v, want >= 1", txMax)
	}
}
