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

import "fmt"

// prettysizeUnits are the base-1000 unit prefixes used by RNS.prettysize, in
// ascending order; the empty string denotes plain bytes.
var prettysizeUnits = []string{"", "K", "M", "G", "T", "P", "E", "Z"}

// Prettysize formats a byte count using base-1000 units with a trailing "B"
// suffix, matching RNS.prettysize (RNS/__init__.py:191). Plain bytes are printed
// with 0 decimals; K/M/G/... values are printed with 2 decimals. Values that
// exceed the Z (zetta) prefix roll over to the "Y" (yotta) suffix with no space.
func Prettysize(num float64) string {
	const lastUnit = "Y"
	for _, unit := range prettysizeUnits {
		if absFloat(num) < 1000.0 {
			if unit == "" {
				return fmt.Sprintf("%.0f %s%s", num, unit, "B")
			}
			return fmt.Sprintf("%.2f %s%s", num, unit, "B")
		}
		num /= 1000.0
	}
	return fmt.Sprintf("%.2f%s%s", num, lastUnit, "B")
}

// absFloat returns the absolute value of x without depending on the math
// package (kept local to mirror the Python `abs(num)` control flow).
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
