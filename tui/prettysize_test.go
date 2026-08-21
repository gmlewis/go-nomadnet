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
	"strconv"
	"testing"
)

// TestPrettysizePythonParity is a LIVE cross-implementation check: it execs
// Python's real RNS.prettysize (RNS/__init__.py), the base-1000 byte-size
// formatter used by NodeStorage stats and elsewhere, and derives the expected
// string freshly on every run. Unlike FormatBytes/FormatSize (base-1024,
// matching NomadNet's format_bytes), RNS.prettysize divides by 1000 and prints
// 2 decimal places for K/M/G/... and 0 decimals for plain bytes. Go owns the
// input battery; Python owns the reference behavior. The test SKIPs, not
// fails, when the Python reference is not importable.
func TestPrettysizePythonParity(t *testing.T) {
	t.Parallel()

	nums := []float64{
		0, 1, 500, 999, 1000, 1024, 1500, 999999, 1000000, 1500000,
		1000000000, 1073741824, 2147483648, 1099511627776, 500000,
		500000000, 500000000000, 225, 3750, 10000, -500, -1500, 1234567890,
		// Boundary cases for the unit-stepping loop: values at and just past
		// each 1000x threshold, plus very large values exceeding all listed
		// units (exercises the final 'Y' fallback branch).
		999.999999, 1000.000001, 999999999999999, 1e18, 1e21, 1e24, 1e27,
	}

	const script = `
import sys, json
import RNS
nums = json.load(sys.stdin)
json.dump([RNS.prettysize(n) for n in nums], sys.stdout)
`

	var want []string
	runPythonNomadnet(t, nums, script, &want)

	for i, n := range nums {
		t.Run(prettysizeCaseName(n), func(t *testing.T) {
			t.Parallel()
			got := Prettysize(n)
			if got != want[i] {
				t.Errorf("Prettysize(%v) = %q, want %q (Python)", n, got, want[i])
			}
		})
	}
}

func prettysizeCaseName(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
