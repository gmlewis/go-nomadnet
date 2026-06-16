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
	"fmt"
	"math"
	"time"
)

// InterfaceBandwidthChart tracks bandwidth history for an interface
// and provides rate calculation for charting. Matches Python's
// InterfaceBandwidthChart at Interfaces.py:1240.
type InterfaceBandwidthChart struct {
	historyLength int
	rxRates       []float64
	txRates       []float64

	prevRX   *float64
	prevTX   *float64
	prevTime *time.Time

	maxRXRate float64
	maxTXRate float64

	firstUpdate            bool
	initializationComplete bool
	stabilizationUpdates   int
	updateCount            int

	peakRXForDisplay float64
	peakTXForDisplay float64
}

// NewInterfaceBandwidthChart creates a new bandwidth chart tracker.
func NewInterfaceBandwidthChart(historyLength int) *InterfaceBandwidthChart {
	if historyLength <= 0 {
		historyLength = 60
	}
	return &InterfaceBandwidthChart{
		historyLength:        historyLength,
		rxRates:              make([]float64, historyLength),
		txRates:              make([]float64, historyLength),
		firstUpdate:          true,
		stabilizationUpdates: 2,
	}
}

// Update records a new bandwidth sample. The first call initializes
// baseline values; subsequent calls compute rates based on deltas.
// rxBytes and txBytes are cumulative byte counters.
// now is the current time for rate calculation.
// Matches Python's update() at Interfaces.py:1258.
func (c *InterfaceBandwidthChart) Update(rxBytes, txBytes float64, now time.Time) {
	if c.firstUpdate {
		c.prevRX = &rxBytes
		c.prevTX = &txBytes
		c.prevTime = &now
		c.firstUpdate = false
		return
	}

	timeDelta := now.Sub(*c.prevTime).Seconds()
	if timeDelta < 0.1 {
		timeDelta = 0.1
	}

	rxDelta := math.Max(0, rxBytes-*c.prevRX) / timeDelta
	txDelta := math.Max(0, txBytes-*c.prevTX) / timeDelta

	c.prevRX = &rxBytes
	c.prevTX = &txBytes
	c.prevTime = &now

	c.updateCount++

	c.rxRates = append(c.rxRates[1:], rxDelta*8)
	c.txRates = append(c.txRates[1:], txDelta*8)

	if c.updateCount >= c.stabilizationUpdates {
		c.initializationComplete = true

		c.peakRXForDisplay = math.Max(c.peakRXForDisplay, rxDelta)
		c.peakTXForDisplay = math.Max(c.peakTXForDisplay, txDelta)

		currentRXMax := maxSlice(c.rxRates)
		currentTXMax := maxSlice(c.txRates)

		c.maxRXRate = math.Max(1, currentRXMax)
		c.maxTXRate = math.Max(1, currentTXMax)
	}
}

// Rates returns the current RX and TX rate history slices.
// Values are in bits per second.
func (c *InterfaceBandwidthChart) Rates() (rx, tx []float64) {
	rx = make([]float64, len(c.rxRates))
	tx = make([]float64, len(c.txRates))
	copy(rx, c.rxRates)
	copy(tx, c.txRates)
	return rx, tx
}

// Peaks returns the peak RX and TX rates as formatted strings.
// Returns "0 bps" if initialization is not yet complete.
func (c *InterfaceBandwidthChart) Peaks() (rxStr, txStr string) {
	if !c.initializationComplete {
		return "0 bps", "0 bps"
	}
	return PrettySpeed(c.peakRXForDisplay * 8), PrettySpeed(c.peakTXForDisplay * 8)
}

// MaxRates returns the current maximum RX and TX rates.
func (c *InterfaceBandwidthChart) MaxRates() (rxMax, txMax float64) {
	return c.maxRXRate, c.maxTXRate
}

// maxSlice returns the maximum value in a float64 slice.
func maxSlice(s []float64) float64 {
	m := s[0]
	for _, v := range s[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// PrettySpeed formats a rate in bits per second as a human-readable
// string. Matches Python's RNS.prettyspeed().
func PrettySpeed(bps float64) string {
	units := []string{"bps", "Kbps", "Mbps", "Gbps", "Tbps"}
	unitIndex := 0
	for bps >= 1000.0 && unitIndex < len(units)-1 {
		bps /= 1000.0
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", int64(bps), units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", bps, units[unitIndex])
}
