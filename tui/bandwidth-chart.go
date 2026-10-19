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

	"github.com/gmlewis/go-nomadnet/nomadnet/asciichart"
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

	// glyphSet selects the chart glyph set (Python InterfaceBandwidthChart
	// glyphset, "unicode" by default).
	glyphSet string
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
		maxRXRate:            1,
		maxTXRate:            1,
		firstUpdate:          true,
		stabilizationUpdates: 2,
		glyphSet:             "unicode",
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

// UpdateNow records a bandwidth sample using the current wall clock, matching
// Python's InterfaceBandwidthChart.update (which reads time.time() itself,
// Interfaces.py:1259). Tests that need a deterministic rate use Update with an
// explicit clock instead.
func (c *InterfaceBandwidthChart) UpdateNow(rxBytes, txBytes float64) {
	c.Update(rxBytes, txBytes, time.Now())
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

// SetGlyphSet sets the chart glyph set ("unicode" or "plain"), mirroring
// Python's InterfaceBandwidthChart(glyphset=...).
func (c *InterfaceBandwidthChart) SetGlyphSet(g string) {
	if g != "" {
		c.glyphSet = g
	}
}

// GetCharts renders the RX and TX bandwidth charts plus their peak labels,
// matching Python's InterfaceBandwidthChart.get_charts (Interfaces.py:1324-1358):
// each rate history is plotted at the given height with Y-axis labels formatted
// by RNS.prettyspeed, the min fixed at 0 and the max at 1.1× the running max
// rate; the peak labels read "0 bps" until the chart's initialization
// completes. The rate histories are in bits per second.
func (c *InterfaceBandwidthChart) GetCharts(height int) (rxChart, txChart, peakRX, peakTX string) {
	var peakRXVal, peakTXVal float64
	if c.initializationComplete {
		peakRXVal = c.peakRXForDisplay
		peakTXVal = c.peakTXForDisplay
	}
	peakRX = PrettySpeed(peakRXVal * 8)
	peakTX = PrettySpeed(peakTXVal * 8)
	rxChart = c.plotRates(c.rxRates, c.maxRXRate, height)
	txChart = c.plotRates(c.txRates, c.maxTXRate, height)
	return rxChart, txChart, peakRX, peakTX
}

// plotRates plots one rate history as an ASCII chart with the get_charts
// configuration (min 0, max 1.1× the max rate, prettyspeed labels).
func (c *InterfaceBandwidthChart) plotRates(rates []float64, maxRate float64, height int) string {
	glyphSet := c.glyphSet
	if glyphSet == "" {
		glyphSet = "unicode"
	}
	chart := asciichart.New(glyphSet)
	chart.Height = height
	zero := 0.0
	chart.Min = &zero
	upper := maxRate * 1.1
	chart.Max = &upper
	chart.FormatFunc = PrettySpeed
	out, err := chart.PlotSingle(rates)
	if err != nil {
		return ""
	}
	return out
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

// PrettySpeed formats a rate in bits per second as a human-readable string,
// matching Python's RNS.prettyspeed (RNS/__init__.py:190):
// prettysize(num/8, suffix="b") + "ps". The /8 and the suffix-"b" ×8 cancel, so
// the base-1000 ladder runs over the bits-per-second value itself with the "b"
// suffix, and plain values print with 0 decimals while K/M/G/... print with 2
// (e.g. 1200 → "1.20 Kbps", 150 → "150 bps").
func PrettySpeed(bps float64) string {
	const lastUnit = "Y"
	num := bps
	for _, unit := range prettysizeUnits {
		if absFloat(num) < 1000.0 {
			if unit == "" {
				return fmt.Sprintf("%.0f %v%v", num, unit, "bps")
			}
			return fmt.Sprintf("%.2f %v%v", num, unit, "bps")
		}
		num /= 1000.0
	}
	return fmt.Sprintf("%.2f%v%v", num, lastUnit, "bps")
}
