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

package micron

import "testing"

// Expected values captured from Python MicronParser.parse_line
// (MicronParser.py:220-416), extracted via /tmp/micron_parseline.py.
// Covers dividers (custom char only when the line is exactly two chars),
// comments (no output), and section-reset ("<" resets depth to 0 and
// re-parses the rest of the line, recursively for multiple "<").
func TestParseLineLevelParity(t *testing.T) {
	t.Parallel()

	t.Run("divider", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			markup string
			want   string
		}{
			{"-", "─"},
			{"-x", "x"},
			{"---", "─"},
			{"-=", "="},
			{"------", "─"},
		}
		for _, tc := range cases {
			t.Run(tc.markup, func(t *testing.T) {
				t.Parallel()
				nodes := Parse(tc.markup)
				var d *Node
				for _, n := range nodes {
					if n.Type == NodeDivider {
						d = n
						break
					}
				}
				if d == nil {
					t.Fatalf("Parse(%q) produced no divider", tc.markup)
				}
				if d.Text != tc.want {
					t.Errorf("divider char = %q, want %q", d.Text, tc.want)
				}
			})
		}
	})

	t.Run("comment", func(t *testing.T) {
		t.Parallel()
		for _, markup := range []string{"# comment", "#"} {
			if nodes := Parse(markup); len(nodes) != 0 {
				t.Errorf("Parse(%q) = %v nodes, want 0", markup, len(nodes))
			}
		}
	})

	t.Run("section_reset", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			markup string
			want   string
		}{
			{"<after heading", "after heading"},
			{"<<deep reset", "deep reset"},
		}
		for _, tc := range cases {
			t.Run(tc.markup, func(t *testing.T) {
				t.Parallel()
				nodes := Parse(tc.markup)
				// Section reset re-parses the remainder as inline text at
				// depth 0; concatenate the text nodes.
				got := ""
				for _, n := range nodes {
					if n.Type == NodeText {
						got += n.Text
					}
					if n.Depth != 0 {
						t.Errorf("node %v Depth = %v, want 0", n.Type, n.Depth)
					}
				}
				if got != tc.want {
					t.Errorf("section-reset text = %q, want %q", got, tc.want)
				}
			})
		}
	})
}
