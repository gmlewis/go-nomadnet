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

package app

import "testing"

// TestFormatPongResult pins the ping-outcome string against Python's
// _ping_peer_from_dialog (Conversations.py:734-744):
//
//	hops_str = f" ({hops} hop{'s' if hops != 1 else ''})"  # when hops < PATHFINDER_M
//	f"Pong in {elapsed_ms} ms{hops_str}"
//
// The hop suffix is omitted when hops is unknown (>= PathfinderM). The plural
// form is "s" for every count except 1 (so "0 hops", "1 hop", "2 hops").
func TestFormatPongResult(t *testing.T) {
	t.Parallel()
	cases := []struct {
		elapsedMs int
		hops      int
		want      string
	}{
		{123, 2, "Pong in 123 ms (2 hops)"},
		{50, 1, "Pong in 50 ms (1 hop)"},
		{50, 0, "Pong in 50 ms (0 hops)"}, // 0 != 1 → plural
		{7, 5, "Pong in 7 ms (5 hops)"},   // single-digit ms
		{50, 128, "Pong in 50 ms"},        // == PathfinderM → unknown → no suffix
		{50, 200, "Pong in 50 ms"},        // > PathfinderM → no suffix
		{50, -1, "Pong in 50 ms"},         // negative (sentinel for unknown) → no suffix
		{0, 1, "Pong in 0 ms (1 hop)"},    // zero ms
	}
	for _, c := range cases {
		got := FormatPongResult(c.elapsedMs, c.hops)
		if got != c.want {
			t.Errorf("FormatPongResult(%v, %v) = %q, want %q", c.elapsedMs, c.hops, got, c.want)
		}
	}
}
