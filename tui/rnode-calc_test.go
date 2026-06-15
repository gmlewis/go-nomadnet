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

func TestRNodeCalcDefaults(t *testing.T) {
	t.Parallel()

	r := CalcRNodeParams(125000, 7, 5, 6, 0, 17)
	assertClose(t, r.RawDataRate, 5468.75, 0.01, "data rate")
	assertClose(t, r.RawSensitivity, -124.5309, 0.001, "sensitivity")
	assertClose(t, r.RawLinkBudget, 141.5309, 0.001, "link budget")
	if r.DataRate != "5.47 kbps" {
		t.Errorf("DataRate = %q, want %q", r.DataRate, "5.47 kbps")
	}
}

func TestRNodeCalcSF12(t *testing.T) {
	t.Parallel()

	r := CalcRNodeParams(125000, 12, 5, 6, 0, 17)
	assertClose(t, r.RawDataRate, 292.968750, 0.01, "data rate")
	assertClose(t, r.RawSensitivity, -137.0309, 0.001, "sensitivity")
	assertClose(t, r.RawLinkBudget, 154.0309, 0.001, "link budget")
	if r.DataRate != "293 bps" {
		t.Errorf("DataRate = %q, want %q", r.DataRate, "293 bps")
	}
}

func TestRNodeCalcFast(t *testing.T) {
	t.Parallel()

	r := CalcRNodeParams(500000, 7, 5, 6, 0, 17)
	assertClose(t, r.RawDataRate, 21875.0, 0.1, "data rate")
	assertClose(t, r.RawSensitivity, -118.5103, 0.001, "sensitivity")
	if r.DataRate != "21.88 kbps" {
		t.Errorf("DataRate = %q, want %q", r.DataRate, "21.88 kbps")
	}
}

func TestRNodeCalcBW203125(t *testing.T) {
	t.Parallel()

	r := CalcRNodeParams(203125, 7, 5, 6, 0, 17)
	assertClose(t, r.RawSensitivity, -114.022366, 0.001, "sensitivity")
	assertClose(t, r.RawDataRate, 8886.718750, 0.1, "data rate")
}

func TestRNodeCalcMinBW(t *testing.T) {
	t.Parallel()

	r := CalcRNodeParams(7800, 7, 5, 6, 0, 17)
	assertClose(t, r.RawDataRate, 341.25, 0.01, "data rate")
	assertClose(t, r.RawSensitivity, -136.579054, 0.001, "sensitivity")
	if r.DataRate != "341 bps" {
		t.Errorf("DataRate = %q, want %q", r.DataRate, "341 bps")
	}
}

func TestRNodeCalcCustomParams(t *testing.T) {
	t.Parallel()

	r := CalcRNodeParams(125000, 10, 8, 3, 6, 20)
	assertClose(t, r.RawDataRate, 610.351562, 0.01, "data rate")
	assertClose(t, r.RawSensitivity, -135.0309, 0.001, "sensitivity")
	assertClose(t, r.RawLinkBudget, 161.0309, 0.001, "link budget")
}

func TestRNodeCalcCR6(t *testing.T) {
	t.Parallel()

	// coding_rate=6 → n=2, CR_eff=4/6
	r := CalcRNodeParams(125000, 7, 6, 6, 0, 17)
	assertClose(t, r.RawDataRate, 4557.291667, 0.1, "data rate")
}

func TestRNodeCalcCR8(t *testing.T) {
	t.Parallel()

	// coding_rate=8 → n=4, CR_eff=4/8
	r := CalcRNodeParams(125000, 7, 8, 6, 0, 17)
	assertClose(t, r.RawDataRate, 3417.96875, 0.01, "data rate")
}

func TestRNodeCalcSensitivityBW406250(t *testing.T) {
	t.Parallel()

	// BW=406250 uses alternate thermal noise floor (-165.6)
	r := CalcRNodeParams(406250, 7, 5, 6, 0, 17)
	assertClose(t, r.RawSensitivity, -111.0121, 0.001, "sensitivity")
}

func TestRNodeCalcAntennaGain(t *testing.T) {
	t.Parallel()

	r1 := CalcRNodeParams(125000, 7, 5, 6, 0, 17)
	r2 := CalcRNodeParams(125000, 7, 5, 6, 6, 17)
	// Link budget should differ by exactly the antenna gain (6 dB)
	diff := r2.RawLinkBudget - r1.RawLinkBudget
	assertClose(t, diff, 6.0, 0.001, "antenna gain effect")
}

func TestRNodeCalcTransmitPower(t *testing.T) {
	t.Parallel()

	r1 := CalcRNodeParams(125000, 7, 5, 6, 0, 14)
	r2 := CalcRNodeParams(125000, 7, 5, 6, 0, 20)
	diff := r2.RawLinkBudget - r1.RawLinkBudget
	assertClose(t, diff, 6.0, 0.001, "transmit power effect")
}

func TestRNodeCalcNoiseFloor(t *testing.T) {
	t.Parallel()

	r1 := CalcRNodeParams(125000, 7, 5, 6, 0, 17)
	r2 := CalcRNodeParams(125000, 7, 5, 3, 0, 17)
	// Lower noise floor → lower sensitivity → higher link budget
	diff := r2.RawLinkBudget - r1.RawLinkBudget
	assertClose(t, diff, 3.0, 0.001, "noise floor effect")
}

func assertClose(t *testing.T, got, want, tolerance float64, label string) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("%s = %v, want %v (tolerance %v)", label, got, want, tolerance)
	}
}
