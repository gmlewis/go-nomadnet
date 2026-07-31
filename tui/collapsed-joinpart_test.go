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

// TestCollapsedJoinPartLabelPythonParity verifies CollapsedJoinPartLabel
// against Python's _collapsed_join_part_widget label (Channels.py:1251):
//
//	"  ⋯  " + str(n) + " join/leave event" + ("" if n == 1 else "s") + "  ⋯"
//
// The ellipsis is U+22EF (MIDLINE HORIZONTAL ELLIPSIS). Expected values were
// captured from /tmp/collapsed_ref.py.
func TestCollapsedJoinPartLabelPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, "  ⋯  0 join/leave events  ⋯"},
		{"one singular", 1, "  ⋯  1 join/leave event  ⋯"},
		{"two plural", 2, "  ⋯  2 join/leave events  ⋯"},
		{"three", 3, "  ⋯  3 join/leave events  ⋯"},
		{"five", 5, "  ⋯  5 join/leave events  ⋯"},
		{"ten", 10, "  ⋯  10 join/leave events  ⋯"},
		{"hundred", 100, "  ⋯  100 join/leave events  ⋯"},
		{"thousand", 1000, "  ⋯  1000 join/leave events  ⋯"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CollapsedJoinPartLabel(tt.n)
			if got != tt.want {
				t.Errorf("CollapsedJoinPartLabel(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
