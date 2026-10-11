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
	"strconv"
	"strings"
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
// calculate_rnode_parameters() at Interfaces.py:178. All parameters are
// explicit: Python's function defaults (noise_floor=6, transmit_power=17) are
// supplied by the caller, since the only caller — RNodeCalculator.
// update_calculation — always passes the values it parsed (Interfaces.py:
// 296-366), and a zero noise floor is a meaningful value there, not "unset".
func CalculateRNodeParameters(bandwidth float64, spreadingFactor, codingRate int, opts RNodeParams) RNodeCalcResult {
	crN := codingRateN[codingRate]
	sfNoise := spreadingFactorNoise[spreadingFactor]

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

// RNodeCalculatorDefaultBandwidth is the bandwidth (Hz) the on-air calculator
// falls back to when the form's bandwidth field is missing or unparseable
// (Python Interfaces.py:301-306).
const RNodeCalculatorDefaultBandwidth = 125000

// RNodeCalculatorReadout mirrors Python's RNodeCalculator.update_calculation
// (Interfaces.py:296-366): it reads the parent form's bandwidth,
// spreadingfactor, codingrate and txpower fields (via fieldValue), plus the
// calculator's own noise-floor and antenna-gain edit texts, applying Python's
// per-field defaults whenever a value is missing or unparseable, and returns the
// three readout lines ("Data Rate: …", "Link Budget: …", "Sensitivity: …").
//
// The coding-rate field holds a "4:5"-style display value, which Python's int()
// rejects (a ValueError that its except clause turns into the default 5), so a
// colon-form coding rate resolves to 5 exactly as Python does.
func RNodeCalculatorReadout(fieldValue func(key string) (string, bool), noiseFloorText, antennaGainText string) (dataRate, linkBudget, sensitivity string) {
	atoiOr := func(s string, def int) int {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return def
		}
		return n
	}
	field := func(key string, def int) int {
		if fieldValue == nil {
			return def
		}
		v, ok := fieldValue(key)
		if !ok {
			return def
		}
		return atoiOr(v, def)
	}

	bandwidth := field("bandwidth", RNodeCalculatorDefaultBandwidth)
	sf := field("spreadingfactor", 7)
	cr := field("codingrate", 5)
	txpower := field("txpower", 17)
	noiseFloor := atoiOr(noiseFloorText, 0)
	antennaGain := atoiOr(antennaGainText, 0)

	res := CalculateRNodeParameters(float64(bandwidth), sf, cr, RNodeParams{
		NoiseFloor:    float64(noiseFloor),
		AntennaGain:   float64(antennaGain),
		TransmitPower: float64(txpower),
	})
	return "Data Rate: " + res.DataRate,
		"Link Budget: " + res.LinkBudget,
		"Sensitivity: " + res.Sensitivity
}
