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

// TestCalculateRNodeParametersPythonParity locks the exact on-air parameter
// strings against Python's calculate_rnode_parameters (Interfaces.py:185),
// captured by inlining the Python function (the module import pulls in urwid).
// It covers the high-bandwidth sensitivity override (>500000 Hz and the
// 203125/406250 special cases) and the spreading-factor / coding-rate
// variants, all with the Python defaults noise_floor=6, antenna_gain=0,
// transmit_power=17 (which the Go zero-value RNodeParams{} maps to).
func TestCalculateRNodeParametersPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		bandwidth       float64
		sf              int
		cr              int
		wantDataRate    string
		wantLinkBudget  string
		wantSensitivity string
	}{
		{"500001 high bw override", 500001, 7, 5, "21.88 kbps", "127.1 dB", "-110.1 dBm"},
		{"203125 special case", 203125, 7, 5, "8.89 kbps", "131.0 dB", "-114.0 dBm"},
		{"406250 special case", 406250, 7, 5, "17.77 kbps", "128.0 dB", "-111.0 dBm"},
		{"125000 coding rate 6", 125000, 7, 6, "4.56 kbps", "141.5 dB", "-124.5 dBm"},
		{"125000 coding rate 8", 125000, 7, 8, "3.42 kbps", "141.5 dB", "-124.5 dBm"},
		{"125000 spreading factor 12", 125000, 12, 5, "293 bps", "154.0 dB", "-137.0 dBm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := CalculateRNodeParameters(tt.bandwidth, tt.sf, tt.cr, RNodeParams{})
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
