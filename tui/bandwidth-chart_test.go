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
)

func TestBandwidthChartNew(t *testing.T) {
	t.Parallel()

	bc := NewBandwidthChart(60)
	if bc.Length() != 60 {
		t.Errorf("Length() = %d, want 60", bc.Length())
	}
	if bc.Initialized() {
		t.Error("should not be initialized after creation")
	}
}

func TestBandwidthChartFirstUpdate(t *testing.T) {
	t.Parallel()

	bc := NewBandwidthChart(10)
	bc.Update(100, 200)

	if bc.Initialized() {
		t.Error("should not be initialized after first update (baseline only)")
	}
}

func TestBandwidthChartTwoUpdates(t *testing.T) {
	t.Parallel()

	bc := NewBandwidthChart(10)
	bc.Update(0, 0)   // baseline
	bc.Update(100, 50) // delta: 100 bytes RX, 50 bytes TX

	if !bc.Initialized() {
		t.Error("should be initialized after 2 updates")
	}
}

func TestBandwidthChartRates(t *testing.T) {
	t.Parallel()

	bc := NewBandwidthChart(10)
	bc.Update(0, 0)
	bc.Update(1000, 500)

	rxRates := bc.RXRates()
	txRates := bc.TXRates()

	// With 1 second elapsed: RX=1000 bytes → 8000 bits, TX=500 bytes → 4000 bits
	// All other entries should be 0
	totalRX := 0.0
	for _, v := range rxRates {
		totalRX += v
	}
	totalTX := 0.0
	for _, v := range txRates {
		totalTX += v
	}

	if totalRX <= 0 {
		t.Errorf("total RX rate should be > 0, got %v", totalRX)
	}
	if totalTX <= 0 {
		t.Errorf("total TX rate should be > 0, got %v", totalTX)
	}
}

func TestBandwidthChartSlidingWindow(t *testing.T) {
t.Parallel()

	bc := NewBandwidthChart(5) // 5 samples max

	// Fill the buffer
	for i := 0; i < 10; i++ {
		bc.Update(float64(i*100), float64(i*50))
	}

	rxRates := bc.RXRates()
	if len(rxRates) != 5 {
		t.Errorf("RXRates length = %d, want 5", len(rxRates))
	}
}

func TestBandwidthChartPeaks(t *testing.T) {
	t.Parallel()

	bc := NewBandwidthChart(10)
	bc.Update(0, 0)
	bc.Update(1000, 200)
	bc.Update(500, 800)

	peakRX := bc.PeakRX()
	peakTX := bc.PeakTX()

	if peakRX <= 0 {
		t.Errorf("PeakRX = %v, want > 0", peakRX)
	}
	if peakTX <= 0 {
		t.Errorf("PeakTX = %v, want > 0", peakTX)
	}
}

func TestBandwidthChartMaxRate(t *testing.T) {
	t.Parallel()

	bc := NewBandwidthChart(10)
	bc.Update(0, 0)

	for i := 1; i <= 5; i++ {
		bc.Update(float64(i*1000), float64(i*500))
	}

	maxRX := bc.MaxRX()
	maxTX := bc.MaxTX()

	if maxRX <= 0 {
		t.Errorf("MaxRX = %v, want > 0", maxRX)
	}
	if maxTX <= 0 {
		t.Errorf("MaxTX = %v, want > 0", maxTX)
	}
	// Max should be >= peak
	if maxRX < bc.PeakRX() {
		t.Errorf("MaxRX (%v) < PeakRX (%v)", maxRX, bc.PeakRX())
	}
}
