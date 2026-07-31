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

// Expected values captured from Python MicronParser.make_output's "[" branch
// (MicronParser.py:763-822), extracted via /tmp/micron_inline.py with the
// urwid-dependent style helpers stubbed. In micron, links are entered from
// formatting mode with the `[ prefix. link_data is split on the backtick
// into 1 (url only), 2 (label+url), or 3 (label+url+fields) components; any
// other count, or an empty url, yields no link. An empty label falls back to
// the url. Go stores the raw fields string (pipe-separated) on LinkFields.
func TestParseLinkParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		markup     string
		wantURL    string
		wantLabel  string
		wantFields string
		wantNoLink bool
	}{
		{
			name: "label_url_fields", markup: "`[label`url`f1|f2]",
			wantURL: "url", wantLabel: "label", wantFields: "f1|f2",
		},
		{
			name: "url_only", markup: "`[urlonly]",
			wantURL: "urlonly", wantLabel: "urlonly", wantFields: "",
		},
		{
			name: "empty_label", markup: "`[`url]",
			wantURL: "url", wantLabel: "url", wantFields: "",
		},
		{
			name: "label_url", markup: "`[label`url]",
			wantURL: "url", wantLabel: "label", wantFields: "",
		},
		{
			name: "fields_all", markup: "`[Submit`:/page.mu`*]",
			wantURL: ":/page.mu", wantLabel: "Submit", wantFields: "*",
		},
		{
			// 4+ components → empty url/label/fields, no link emitted.
			name: "too_many_components", markup: "`[a`b`c`d]", wantNoLink: true,
		},
		{
			// Empty url → no link emitted; the closing ] is consumed.
			name: "empty_url", markup: "`[]", wantNoLink: true,
		},
		{
			// No closing ]: the `[ prefix is consumed and the rest is text.
			name: "no_close", markup: "`[noclose", wantNoLink: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nodes := Parse(tc.markup)
			if tc.wantNoLink {
				for _, n := range nodes {
					if n.Type == NodeLink {
						t.Fatalf("Parse(%q) produced a link node, want none", tc.markup)
					}
				}
				return
			}
			var n *Node
			for _, cand := range nodes {
				if cand.Type == NodeLink {
					n = cand
					break
				}
			}
			if n == nil {
				t.Fatalf("Parse(%q) produced no link node", tc.markup)
			}
			if n.LinkURL != tc.wantURL {
				t.Errorf("LinkURL = %q, want %q", n.LinkURL, tc.wantURL)
			}
			if n.LinkLabel != tc.wantLabel {
				t.Errorf("LinkLabel = %q, want %q", n.LinkLabel, tc.wantLabel)
			}
			if n.LinkFields != tc.wantFields {
				t.Errorf("LinkFields = %q, want %q", n.LinkFields, tc.wantFields)
			}
		})
	}
}
