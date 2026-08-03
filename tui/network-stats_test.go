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
	"time"
)

// TestNetworkStatLabelsPythonParity verifies the seven NetworkDisplay
// stat-widget value methods against Python (Network.py:1026-1244):
// AnnounceTime, NodeAnnounceTime, NodeActiveConnections, NodeStorageStats,
// NodeTotalConnections, NodeTotalPages, NodeTotalFiles. Each Python widget
// builds a "label : value" line; the value depends on app state (node present,
// peer_settings timestamps, router storage). The Go label functions take the
// raw inputs and reproduce the exact strings, including the fixed-width label
// spacing Python hardcodes ("Announced : ", "Last Announce  : ",
// "Connected Now  : ", "LXMF Storage   : ", "Total Connects : ",
// "Served Pages   : ", "Served Files   : "). Expected values were captured
// from /tmp/stat_ref.py with a fixed now of 2026-07-31 12:00:00 UTC, so the
// timestamp-based labels are driven through prettyDateAt with that same now.
func TestNetworkStatLabelsPythonParity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	sec := func(offset time.Duration) int64 {
		return now.Add(-offset).Unix()
	}

	t.Run("AnnounceTime", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			ts   *int64
			want string
		}{
			{"never", nil, "Announced : Never"},
			{"a minute ago", pInt64(sec(90 * time.Second)), "Announced : a minute ago"},
			{"hours ago", pInt64(sec(5 * time.Hour)), "Announced : 5 hours ago"},
			{"days ago", pInt64(sec(48 * time.Hour)), "Announced : 2 days ago"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := AnnounceTimeLabel(tt.ts, now)
				if got != tt.want {
					t.Errorf("AnnounceTimeLabel(%v) = %q, want %q", tt.ts, got, tt.want)
				}
			})
		}
	})

	t.Run("NodeAnnounceTime", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			ts   *int64
			want string
		}{
			{"never", nil, "Last Announce  : Never"},
			{"a minute ago", pInt64(sec(90 * time.Second)), "Last Announce  : a minute ago"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := NodeAnnounceTimeLabel(tt.ts, now)
				if got != tt.want {
					t.Errorf("NodeAnnounceTimeLabel(%v) = %q, want %q", tt.ts, got, tt.want)
				}
			})
		}
	})

	t.Run("NodeActiveConnections", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			links   int
			hasNode bool
			want    string
		}{
			{"three", 3, true, "Connected Now  : 3"},
			{"zero", 0, true, "Connected Now  : 0"},
			{"no node", 0, false, "Connected Now  : None"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := NodeActiveConnectionsLabel(tt.links, tt.hasNode)
				if got != tt.want {
					t.Errorf("NodeActiveConnectionsLabel(%v,%v) = %q, want %q",
						tt.links, tt.hasNode, got, tt.want)
				}
			})
		}
	})

	t.Run("NodeStorageStats", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			used    *int64
			limit   *int64
			hasNode bool
			propOn  bool
			want    string
		}{
			{"50pct", pInt64(500000), pInt64(1000000), true, true,
				"LXMF Storage   : 50.0%, 500.00 KB of 1.00 MB"},
			{"zero used", pInt64(0), pInt64(1000000), true, true,
				"LXMF Storage   : 0.0%, 0 B of 1.00 MB"},
			{"no limit", pInt64(512), nil, true, true,
				"LXMF Storage   : 512 B"},
			{"bankers pct", pInt64(225), pInt64(10000), true, true,
				"LXMF Storage   : 2.2%, 225 B of 10.00 KB"},
			{"37.5 pct", pInt64(3750), pInt64(10000), true, true,
				"LXMF Storage   : 37.5%, 3.75 KB of 10.00 KB"},
			{"propagation disabled", pInt64(500), pInt64(1000), true, false,
				"LXMF Storage   : None"},
			{"no node", pInt64(500), pInt64(1000), false, true,
				"LXMF Storage   : None"},
			{"big GB", pInt64(1073741824), pInt64(2147483648), true, true,
				"LXMF Storage   : 50.0%, 1.07 GB of 2.15 GB"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := NodeStorageStatsLabel(tt.used, tt.limit, tt.hasNode, tt.propOn)
				if got != tt.want {
					t.Errorf("NodeStorageStatsLabel(%v,%v,%v,%v) = %q, want %q",
						tt.used, tt.limit, tt.hasNode, tt.propOn, got, tt.want)
				}
			})
		}
	})

	t.Run("NodeTotalConnections", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			n       int
			hasNode bool
			want    string
		}{
			{"42", 42, true, "Total Connects : 42"},
			{"zero", 0, true, "Total Connects : 0"},
			{"no node", 0, false, "Total Connects : None"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := NodeTotalConnectionsLabel(tt.n, tt.hasNode)
				if got != tt.want {
					t.Errorf("NodeTotalConnectionsLabel(%v,%v) = %q, want %q",
						tt.n, tt.hasNode, got, tt.want)
				}
			})
		}
	})

	t.Run("NodeTotalPages", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			n       int
			hasNode bool
			want    string
		}{
			{"7", 7, true, "Served Pages   : 7"},
			{"no node", 0, false, "Served Pages   : None"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := NodeTotalPagesLabel(tt.n, tt.hasNode)
				if got != tt.want {
					t.Errorf("NodeTotalPagesLabel(%v,%v) = %q, want %q",
						tt.n, tt.hasNode, got, tt.want)
				}
			})
		}
	})

	t.Run("NodeTotalFiles", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			n       int
			hasNode bool
			want    string
		}{
			{"13", 13, true, "Served Files   : 13"},
			{"no node", 0, false, "Served Files   : None"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := NodeTotalFilesLabel(tt.n, tt.hasNode)
				if got != tt.want {
					t.Errorf("NodeTotalFilesLabel(%v,%v) = %q, want %q",
						tt.n, tt.hasNode, got, tt.want)
				}
			})
		}
	})
}

// pInt64 returns a pointer to v (helper for optional int64 inputs).
func pInt64(v int64) *int64 { return &v }
