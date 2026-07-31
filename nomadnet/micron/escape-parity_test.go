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

// Expected values captured from Python MicronParser.make_output's escape
// handling (MicronParser.py:829-846), extracted via /tmp/micron_inline.py.
// A backslash escapes the next character (making ` and \ literal); a
// trailing backslash contributes nothing. A leading "\<" / "\#" line is
// not a section-reset or comment — the backslash escapes the first char so
// the line renders as literal text.
func TestParseEscapeParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		markup string
		want   string
	}{
		{"a \\`!b", "a `!b"},
		{"a \\\\b", "a \\b"},
		{"trailing\\", "trailing"},
		{"\\>not heading", ">not heading"},
		{"\\#not comment", "#not comment"},
		{"mix \\`!bold\\`!", "mix `!bold`!"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.markup, func(t *testing.T) {
			t.Parallel()
			nodes := Parse(tc.markup)
			got := ""
			for _, n := range nodes {
				if n.Type == NodeText {
					got += n.Text
				}
			}
			if got != tc.want {
				t.Errorf("Parse(%q) text = %q, want %q", tc.markup, got, tc.want)
			}
		})
	}
}
