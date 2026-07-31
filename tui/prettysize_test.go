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

// TestPrettysizePythonParity verifies Prettysize against RNS.prettysize
// (RNS/__init__.py:191), the base-1000 byte-size formatter used by NodeStorage
// stats and elsewhere. Unlike FormatBytes/FormatSize (base-1024, matching
// NomadNet's format_bytes), RNS.prettysize divides by 1000 and prints 2 decimal
// places for K/M/G/... and 0 decimals for plain bytes. Expected values were
// captured from /tmp/prettysize_ref.py.
func TestPrettysizePythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    float64
		want string
	}{
		{"zero", 0, "0 B"},
		{"one", 1, "1 B"},
		{"500 bytes", 500, "500 B"},
		{"999 bytes", 999, "999 B"},
		{"1000 -> 1.00 KB", 1000, "1.00 KB"},
		{"1024 -> 1.02 KB", 1024, "1.02 KB"},
		{"1500 -> 1.50 KB", 1500, "1.50 KB"},
		{"999999 -> 1000.00 KB", 999999, "1000.00 KB"},
		{"1000000 -> 1.00 MB", 1000000, "1.00 MB"},
		{"1500000 -> 1.50 MB", 1500000, "1.50 MB"},
		{"1e9 -> 1.00 GB", 1000000000, "1.00 GB"},
		{"1073741824 -> 1.07 GB", 1073741824, "1.07 GB"},
		{"2147483648 -> 2.15 GB", 2147483648, "2.15 GB"},
		{"1099511627776 -> 1.10 TB", 1099511627776, "1.10 TB"},
		{"500000 KB", 500000, "500.00 KB"},
		{"500000000 MB", 500000000, "500.00 MB"},
		{"500000000000 GB", 500000000000, "500.00 GB"},
		{"225 bytes", 225, "225 B"},
		{"3750 -> 3.75 KB", 3750, "3.75 KB"},
		{"10000 -> 10.00 KB", 10000, "10.00 KB"},
		{"negative bytes", -500, "-500 B"},
		{"negative KB", -1500, "-1.50 KB"},
		{"1234567890 -> 1.23 GB", 1234567890, "1.23 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Prettysize(tt.n)
			if got != tt.want {
				t.Errorf("Prettysize(%v) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
