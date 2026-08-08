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
// (MicronParser.py:220-416), extracted via /tmp/micron_parseline.py with
// urwid widget constructors stubbed. The heading level is the unbounded
// count of leading ">" characters (Python does not clamp it). Heading lines
// that contain a field (`<) lose their heading status: the leading ">"s are
// stripped and the line is reclassified (MicronParser.py:233-236).
func TestParseHeadingParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		markup    string
		wantLevel int
		wantText  string
		wantNone  bool
		wantField bool
	}{
		{
			name: "h1", markup: ">Heading one",
			wantLevel: 1, wantText: "Heading one",
		},
		{
			name: "h2", markup: ">>Heading two",
			wantLevel: 2, wantText: "Heading two",
		},
		{
			name: "h3", markup: ">>>Heading three",
			wantLevel: 3, wantText: "Heading three",
		},
		{
			name: "h4", markup: ">>>>Heading four",
			wantLevel: 4, wantText: "Heading four",
		},
		{
			name: "h5_unbounded", markup: ">>>>>Heading five",
			wantLevel: 5, wantText: "Heading five",
		},
		{
			// ">" with no content yields no node.
			name: "empty", markup: ">", wantNone: true,
		},
		{
			// A leading "!" is literal text (no backtick precedes it), so the
			// heading text is "!Bold heading"; the trailing `! toggles bold
			// but emits no text.
			name: "leading_bang_literal", markup: ">!Bold heading`!",
			wantLevel: 1, wantText: "!Bold heading",
		},
		{
			// Heading sanitization: a ">"-prefixed line containing `< is
			// stripped of its ">"s and reclassified, so it produces a field
			// at depth 0, not a heading.
			name: "field_sanitized", markup: ">>`<field`data>",
			wantNone: true, wantField: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nodes := Parse(tc.markup)

			var heading *Node
			for _, n := range nodes {
				if n.Type == NodeHeading {
					heading = n
					break
				}
			}
			if tc.wantNone {
				if heading != nil {
					t.Fatalf("Parse(%q) produced a heading, want none", tc.markup)
				}
				if tc.wantField {
					found := false
					for _, n := range nodes {
						if n.Type == NodeField {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Parse(%q) produced no field node, want one", tc.markup)
					}
				}
				return
			}
			if heading == nil {
				t.Fatalf("Parse(%q) produced no heading", tc.markup)
			}
			if heading.Level != tc.wantLevel {
				t.Errorf("Level = %v, want %v", heading.Level, tc.wantLevel)
			}
			// Concatenate the text of text-node children to compare against
			// the Python heading content (toggle nodes emit no text).
			got := ""
			for _, c := range heading.Children {
				if c.Type == NodeText {
					got += c.Text
				}
			}
			if got != tc.wantText {
				t.Errorf("heading text = %q, want %q", got, tc.wantText)
			}
		})
	}
}
