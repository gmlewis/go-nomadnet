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

func TestCalculateRNodeParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		bandwidth       float64
		sf              int
		cr              int
		noiseFloor      float64
		antennaGain     float64
		txPower         float64
		wantDataRate    string
		wantLinkBudget  string
		wantSensitivity string
	}{
		{
			name:      "7800/7/5 defaults",
			bandwidth: 7800, sf: 7, cr: 5,
			wantDataRate:    "341 bps",
			wantLinkBudget:  "153.6 dB",
			wantSensitivity: "-136.6 dBm",
		},
		{
			name:      "125000/7/5 defaults",
			bandwidth: 125000, sf: 7, cr: 5,
			wantDataRate:    "5.47 kbps",
			wantLinkBudget:  "141.5 dB",
			wantSensitivity: "-124.5 dBm",
		},
		{
			name:      "62500/9/5 with custom noise/gain/power",
			bandwidth: 62500, sf: 9, cr: 5,
			noiseFloor: 6, antennaGain: 3, txPower: 20,
			wantDataRate:    "879 bps",
			wantLinkBudget:  "155.5 dB",
			wantSensitivity: "-132.5 dBm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := CalculateRNodeParameters(tt.bandwidth, tt.sf, tt.cr,
				RNodeParams{NoiseFloor: tt.noiseFloor, AntennaGain: tt.antennaGain, TransmitPower: tt.txPower})

			if result.DataRate != tt.wantDataRate {
				t.Errorf("DataRate = %q, want %q", result.DataRate, tt.wantDataRate)
			}
			if result.LinkBudget != tt.wantLinkBudget {
				t.Errorf("LinkBudget = %q, want %q", result.LinkBudget, tt.wantLinkBudget)
			}
			if result.Sensitivity != tt.wantSensitivity {
				t.Errorf("Sensitivity = %q, want %q", result.Sensitivity, tt.wantSensitivity)
			}
		})
	}
}

func TestCalculateRNodeParametersHighBandwidth(t *testing.T) {
	t.Parallel()

	result := CalculateRNodeParameters(500001, 7, 5, RNodeParams{})

	if result.Sensitivity == "" {
		t.Error("should produce sensitivity for bandwidth > 500000")
	}
}

func TestCalculateRNodeParametersCodingRates(t *testing.T) {
	t.Parallel()

	for _, cr := range []int{5, 6, 7, 8} {
		result := CalculateRNodeParameters(125000, 7, cr, RNodeParams{})
		if result.DataRate == "" {
			t.Errorf("coding rate %v produced empty data rate", cr)
		}
	}
}

func TestCalculateRNodeParametersSpreadingFactors(t *testing.T) {
	t.Parallel()

	for _, sf := range []int{5, 6, 7, 8, 9, 10, 11, 12} {
		result := CalculateRNodeParameters(125000, sf, 5, RNodeParams{})
		if result.DataRate == "" {
			t.Errorf("spreading factor %v produced empty data rate", sf)
		}
	}
}

// TestCalculateRNodeParametersPythonParity is a LIVE cross-implementation
// check: it execs Python's real Interfaces.calculate_rnode_parameters
// (nomadnet.ui.textui.Interfaces) — a module-level function needing no
// app/urwid instance — and derives the expected (data_rate, link_budget,
// sensitivity) strings freshly on every run. Go owns the input battery;
// Python owns the reference behavior. The test SKIPs, not fails, when the
// Python reference is not importable.
//
// The battery covers: the bps (<1000) and kbps (>=1000) data-rate branches;
// the high-bandwidth sensitivity override (>500000 Hz) and the 203125/406250
// special cases; all spreading factors 5..12; coding rates 5..8; and custom
// noise_floor / antenna_gain / transmit_power. The "useGoDefault" cases pass
// Go RNodeParams{} (zero) — which Go defaults to noise_floor=6,
// antenna_gain=0, transmit_power=17 — and pass those same explicit defaults
// to Python, exercising Go's sentinel-defaulting path against Python's
// keyword defaults. Custom-param cases pass the same explicit values to both.
//
// Note: Go treats NoiseFloor==0 and TransmitPower==0 as "unset" and
// substitutes the Python defaults (6 and 17), so a noise_floor of exactly 0
// is not representable in Go; the battery therefore uses non-zero noise
// floors (the realistic range), which is the only region where the two
// implementations are required to agree.
func TestCalculateRNodeParametersPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		bandwidth    float64
		sf           int
		cr           int
		noiseFloor   float64
		antennaGain  float64
		txPower      float64
		useGoDefault bool // true: Go gets RNodeParams{}, Python gets (6,0,17)
	}{
		{"7800 sf7 cr5 default", 7800, 7, 5, 6, 0, 17, true},
		{"125000 sf7 cr5 default", 125000, 7, 5, 6, 0, 17, true},
		{"500001 high bw override", 500001, 7, 5, 6, 0, 17, true},
		{"203125 special case", 203125, 7, 5, 6, 0, 17, true},
		{"406250 special case", 406250, 7, 5, 6, 0, 17, true},
		{"125000 cr6", 125000, 7, 6, 6, 0, 17, true},
		{"125000 cr8", 125000, 7, 8, 6, 0, 17, true},
		{"125000 sf12", 125000, 12, 5, 6, 0, 17, true},
		{"125000 sf5", 125000, 5, 5, 6, 0, 17, true},
		{"125000 sf6", 125000, 6, 5, 6, 0, 17, true},
		{"125000 sf10", 125000, 10, 5, 6, 0, 17, true},
		{"125000 sf11", 125000, 11, 5, 6, 0, 17, true},
		{"125000 sf8 cr7", 125000, 8, 7, 6, 0, 17, true},
		{"62500 sf9 custom noise/gain/power", 62500, 9, 5, 6, 3, 20, false},
		{"250000 sf7 custom params", 250000, 7, 5, 10, 2, 25, false},
		{"10000 sf7 cr5 custom", 10000, 7, 5, 8, 5, 13, false},
	}

	type rnodeInput struct {
		Bandwidth   float64 `json:"bandwidth"`
		SF          int     `json:"sf"`
		CR          int     `json:"cr"`
		NoiseFloor  float64 `json:"noise_floor"`
		AntennaGain float64 `json:"antenna_gain"`
		TxPower     float64 `json:"tx_power"`
	}
	inputs := make([]rnodeInput, len(tests))
	for i, tt := range tests {
		nf, ag, tp := tt.noiseFloor, tt.antennaGain, tt.txPower
		inputs[i] = rnodeInput{Bandwidth: tt.bandwidth, SF: tt.sf, CR: tt.cr,
			NoiseFloor: nf, AntennaGain: ag, TxPower: tp}
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Interfaces as I
cases = json.load(sys.stdin)
out = []
for c in cases:
    r = I.calculate_rnode_parameters(c["bandwidth"], c["sf"], c["cr"],
                                     c["noise_floor"], c["antenna_gain"], c["tx_power"])
    out.append({"data_rate": r["data_rate"], "link_budget": r["link_budget"], "sensitivity": r["sensitivity"]})
json.dump(out, sys.stdout)
`

	var want []rnodeLiveWant
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := RNodeParams{NoiseFloor: tt.noiseFloor, AntennaGain: tt.antennaGain, TransmitPower: tt.txPower}
			if tt.useGoDefault {
				opts = RNodeParams{}
			}
			result := CalculateRNodeParameters(tt.bandwidth, tt.sf, tt.cr, opts)
			w := want[i]
			if result.DataRate != w.DataRate {
				t.Errorf("DataRate = %q, want %q (Python)", result.DataRate, w.DataRate)
			}
			if result.LinkBudget != w.LinkBudget {
				t.Errorf("LinkBudget = %q, want %q (Python)", result.LinkBudget, w.LinkBudget)
			}
			if result.Sensitivity != w.Sensitivity {
				t.Errorf("Sensitivity = %q, want %q (Python)", result.Sensitivity, w.Sensitivity)
			}
		})
	}
}

type rnodeLiveWant struct {
	DataRate    string `json:"data_rate"`
	LinkBudget  string `json:"link_budget"`
	Sensitivity string `json:"sensitivity"`
}
