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

// RNodeParams holds the calculated RNode link parameters.
type RNodeParams struct {
	DataRate       string  // formatted data rate string
	Sensitivity    string  // formatted sensitivity string
	LinkBudget     string  // formatted link budget string
	RawDataRate    float64 // data rate in bps
	RawSensitivity float64 // sensitivity in dBm
	RawLinkBudget  float64 // link budget in dB
}

// crn maps coding rate denominator to numerator offset.
// CR 4:5→1, 4:6→2, 4:7→3, 4:8→4.
var crn = map[int]int{5: 1, 6: 2, 7: 3, 8: 4}

// sfn maps spreading factor to noise margin offset in dB.
var sfn = map[int]float64{
	5: -2.5, 6: -5, 7: -7.5, 8: -10,
	9: -12.5, 10: -15, 11: -17.5, 12: -20,
}

// CalcRNodeParams computes data rate, sensitivity, and link budget
// for an RNode LoRa interface. Matches Python's
// calculate_rnode_parameters exactly.
func CalcRNodeParams(bandwidth, spreadingFactor, codingRate int,
	noiseFloor, antennaGain, transmitPower float64) RNodeParams {

	codingRateN := crn[codingRate]
	if codingRateN == 0 {
		codingRateN = 1
	}

	crEff := 4.0 / float64(4+codingRateN)
	dataRate := float64(spreadingFactor) * (crEff / (math.Pow(2, float64(spreadingFactor)) / (float64(bandwidth) / 1000.0))) * 1000.0

	// Alternate thermal noise floor for wider bandwidths
	var thermalNoise float64
	if bandwidth == 203125 || bandwidth == 406250 || bandwidth > 500000 {
		thermalNoise = -165.6
	} else {
		thermalNoise = -174.0
	}

	sensitivity := thermalNoise + 10*math.Log10(float64(bandwidth)) + noiseFloor + sfn[spreadingFactor]
	linkBudget := (transmitPower - sensitivity) + antennaGain

	var drStr string
	if dataRate < 1000 {
		drStr = fmt.Sprintf("%.0f bps", dataRate)
	} else {
		drStr = fmt.Sprintf("%.2f kbps", dataRate/1000)
	}

	return RNodeParams{
		DataRate:       drStr,
		Sensitivity:    fmt.Sprintf("%.1f dBm", sensitivity),
		LinkBudget:     fmt.Sprintf("%.1f dB", linkBudget),
		RawDataRate:    dataRate,
		RawSensitivity: sensitivity,
		RawLinkBudget:  linkBudget,
	}
}
