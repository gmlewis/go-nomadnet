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
)

// RNodeParams holds optional parameters for RNode on-air calculations.
type RNodeParams struct {
	NoiseFloor    float64
	AntennaGain   float64
	TransmitPower float64
}

// RNodeCalcResult holds the results of RNode parameter calculations.
type RNodeCalcResult struct {
	DataRate       string
	LinkBudget     string
	Sensitivity    string
	RawDataRate    float64
	RawLinkBudget  float64
	RawSensitivity float64
}

// codingRateN maps coding rate values to their numerical
// representation used in the data rate formula.
var codingRateN = map[int]float64{
	5: 1,
	6: 2,
	7: 3,
	8: 4,
}

// spreadingFactorNoise maps spreading factor values to their noise
// floor offset in dB. Matches Python's sfn dict.
var spreadingFactorNoise = map[int]float64{
	5:  -2.5,
	6:  -5,
	7:  -7.5,
	8:  -10,
	9:  -12.5,
	10: -15,
	11: -17.5,
	12: -20,
}

// CalculateRNodeParameters computes on-air parameters for LoRa RNode
// interfaces: data rate, sensitivity, and link budget. Matches Python's
// calculate_rnode_parameters() at Interfaces.py:178.
func CalculateRNodeParameters(bandwidth float64, spreadingFactor, codingRate int, opts RNodeParams) RNodeCalcResult {
	crN := codingRateN[codingRate]
	sfNoise := spreadingFactorNoise[spreadingFactor]

	if opts.NoiseFloor == 0 {
		opts.NoiseFloor = 6
	}
	if opts.TransmitPower == 0 {
		opts.TransmitPower = 17
	}

	dataRate := float64(spreadingFactor) *
		(4.0 / (4.0 + crN)) /
		(math.Pow(2, float64(spreadingFactor)) / (bandwidth / 1000.0)) * 1000.0

	sensitivity := -174.0 + 10.0*math.Log10(bandwidth) + opts.NoiseFloor + sfNoise

	if bandwidth == 203125 || bandwidth == 406250 || bandwidth > 500000 {
		sensitivity = -165.6 + 10.0*math.Log10(bandwidth) + opts.NoiseFloor + sfNoise
	}

	linkBudget := (opts.TransmitPower - sensitivity) + opts.AntennaGain

	var dataRateStr string
	if dataRate < 1000 {
		dataRateStr = fmt.Sprintf("%.0f bps", dataRate)
	} else {
		dataRateStr = fmt.Sprintf("%.2f kbps", dataRate/1000.0)
	}

	return RNodeCalcResult{
		DataRate:       dataRateStr,
		LinkBudget:     fmt.Sprintf("%.1f dB", linkBudget),
		Sensitivity:    fmt.Sprintf("%.1f dBm", sensitivity),
		RawDataRate:    dataRate,
		RawLinkBudget:  linkBudget,
		RawSensitivity: sensitivity,
	}
}
