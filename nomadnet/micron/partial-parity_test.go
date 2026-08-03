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

import (
	"reflect"
	"testing"
)

// Expected values captured from Python MicronParser.parse_partial
// (MicronParser.py:149-195), extracted and run in /tmp/parse_partial.py
// against the upstream source. The markup passed to Parse is "`{" + inner,
// where inner already includes the closing brace, matching the Python call
// parse_partial(line[2:]).
func TestParsePartialParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		inner      string // text after the leading `{
		wantNodes  int
		url        string
		hasRefresh bool
		refresh    float64
		fields     []string
		partialID  string
	}{
		{
			name:      "url_only",
			inner:     "page_name}",
			wantNodes: 1,
			url:       "page_name",
			fields:    []string{""},
		},
		{
			name:       "url_and_refresh",
			inner:      "page_name`5}",
			wantNodes:  1,
			url:        "page_name",
			hasRefresh: true,
			refresh:    5.0,
			fields:     []string{""},
		},
		{
			name:       "url_refresh_pid",
			inner:      "page_name`5`pid=foo}",
			wantNodes:  1,
			url:        "page_name",
			hasRefresh: true,
			refresh:    5.0,
			fields:     []string{"pid=foo"},
			partialID:  "foo",
		},
		{
			name:      "refresh_below_one_dropped",
			inner:     "page_name`0.5}",
			wantNodes: 1,
			url:       "page_name",
			fields:    []string{""},
		},
		{
			name:       "fields_with_pid",
			inner:      "page_name`2`field1|pid=abc}",
			wantNodes:  1,
			url:        "page_name",
			hasRefresh: true,
			refresh:    2.0,
			fields:     []string{"field1", "pid=abc"},
			partialID:  "abc",
		},
		{
			name:       "multi_fields_pid_last",
			inner:      "page_name`10`a|b|pid=xyz}",
			wantNodes:  1,
			url:        "page_name",
			hasRefresh: true,
			refresh:    10.0,
			fields:     []string{"a", "b", "pid=xyz"},
			partialID:  "xyz",
		},
		{
			name:      "empty_url_no_node",
			inner:     "}",
			wantNodes: 0,
		},
		{
			name:      "too_many_components_no_node",
			inner:     "a`b`c`d}",
			wantNodes: 0,
		},
		{
			name:       "pid_only_field",
			inner:      "page_name`2`pid=bar}",
			wantNodes:  1,
			url:        "page_name",
			hasRefresh: true,
			refresh:    2.0,
			fields:     []string{"pid=bar"},
			partialID:  "bar",
		},
		{
			name:      "non_numeric_refresh_no_node",
			inner:     "page_name`abc}",
			wantNodes: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nodes := Parse("`{" + tc.inner)
			if len(nodes) != tc.wantNodes {
				t.Fatalf("Parse partial len = %v, want %v", len(nodes), tc.wantNodes)
			}
			if tc.wantNodes == 0 {
				return
			}
			n := nodes[0]
			if n.Type != NodePartial {
				t.Fatalf("node type = %v, want %v", n.Type, NodePartial)
			}
			if n.PartialURL != tc.url {
				t.Errorf("partial url = %q, want %q", n.PartialURL, tc.url)
			}
			if n.HasRefresh != tc.hasRefresh {
				t.Errorf("has refresh = %v, want %v", n.HasRefresh, tc.hasRefresh)
			}
			if n.HasRefresh && n.PartialRefresh != tc.refresh {
				t.Errorf("partial refresh = %v, want %v", n.PartialRefresh, tc.refresh)
			}
			if !reflect.DeepEqual(n.PartialFields, tc.fields) {
				t.Errorf("partial fields = %v, want %v", n.PartialFields, tc.fields)
			}
			if n.PartialID != tc.partialID {
				t.Errorf("partial id = %q, want %q", n.PartialID, tc.partialID)
			}
		})
	}
}
