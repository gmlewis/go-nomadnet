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
	"sync"
	"time"
)

// BandwidthChart tracks RX/TX traffic rates over a sliding window.
// Matches Python's InterfaceBandwidthChart.
type BandwidthChart struct {
	mu              sync.Mutex
	historyLength   int
	rxRates         []float64
	txRates         []float64
	prevRX          float64
	prevTX          float64
	prevTime        time.Time
	maxRXRate       float64
	maxTXRate       float64
	peakRX          float64
	peakTX          float64
	initialized     bool
	stabilization   int
	initialComplete bool
}

// NewBandwidthChart creates a chart with the given number of history samples.
func NewBandwidthChart(historyLength int) *BandwidthChart {
	return &BandwidthChart{
		historyLength: historyLength,
		rxRates:       make([]float64, historyLength),
		txRates:       make([]float64, historyLength),
		prevTime:      time.Now(),
		maxRXRate:     1,
		maxTXRate:     1,
	}
}

// Update records a new sample from cumulative byte counters.
// Matches Python's InterfaceBandwidthChart.update().
func (bc *BandwidthChart) Update(rxBytes, txBytes float64) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	now := time.Now()
	timeDelta := now.Sub(bc.prevTime).Seconds()
	if timeDelta < 0.1 {
		timeDelta = 0.1
	}

	if bc.prevTime.IsZero() || bc.prevRX == 0 && bc.prevTX == 0 && !bc.initialized && bc.stabilization == 0 {
		bc.prevRX = rxBytes
		bc.prevTX = txBytes
		bc.prevTime = now
		bc.stabilization++
		return
	}

	rxDelta := rxBytes - bc.prevRX
	if rxDelta < 0 {
		rxDelta = 0
	}
	txDelta := txBytes - bc.prevTX
	if txDelta < 0 {
		txDelta = 0
	}

	rxRate := rxDelta / timeDelta * 8 // bytes/sec to bits/sec
	txRate := txDelta / timeDelta * 8

	// Shift and append (ring buffer)
	copy(bc.rxRates, bc.rxRates[1:])
	bc.rxRates[bc.historyLength-1] = rxRate
	copy(bc.txRates, bc.txRates[1:])
	bc.txRates[bc.historyLength-1] = txRate

	bc.prevRX = rxBytes
	bc.prevTX = txBytes
	bc.prevTime = now

	bc.stabilization++
	if bc.stabilization >= 2 {
		bc.initialComplete = true
	}

	if bc.initialComplete {
		if rxRate > bc.peakRX {
			bc.peakRX = rxRate
		}
		if txRate > bc.peakTX {
			bc.peakTX = txRate
		}
		if bc.peakRX > bc.maxRXRate {
			bc.maxRXRate = bc.peakRX
		}
		if bc.peakTX > bc.maxTXRate {
			bc.maxTXRate = bc.peakTX
		}
	}

	bc.initialized = true
}

// RXRates returns a copy of the RX rate history.
func (bc *BandwidthChart) RXRates() []float64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	out := make([]float64, len(bc.rxRates))
	copy(out, bc.rxRates)
	return out
}

// TXRates returns a copy of the TX rate history.
func (bc *BandwidthChart) TXRates() []float64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	out := make([]float64, len(bc.txRates))
	copy(out, bc.txRates)
	return out
}

// PeakRX returns the peak RX rate in bits/sec.
func (bc *BandwidthChart) PeakRX() float64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.peakRX
}

// PeakTX returns the peak TX rate in bits/sec.
func (bc *BandwidthChart) PeakTX() float64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.peakTX
}

// MaxRX returns the max RX rate used for chart scaling (with headroom).
func (bc *BandwidthChart) MaxRX() float64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.maxRXRate * 1.1
}

// MaxTX returns the max TX rate used for chart scaling (with headroom).
func (bc *BandwidthChart) MaxTX() float64 {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.maxTXRate * 1.1
}

// Length returns the history buffer size.
func (bc *BandwidthChart) Length() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.historyLength
}

// Initialized returns true if at least 2 updates have been received.
func (bc *BandwidthChart) Initialized() bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.initialComplete
}
