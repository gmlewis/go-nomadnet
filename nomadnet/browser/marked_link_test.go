// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package browser

import "testing"

// TestMarkedLinkTargetGolden pins Python Browser.marked_link's link-target
// construction (Browser.py:173-176): when link_fields is a non-empty list, the
// fields are joined with "|" and appended to the target after a backtick
// ("<target>`<f1|f2>"). A nil/empty fields list leaves the target unchanged
// (Python `if link_fields:` is falsy for None and []). Golden values captured
// from the installed Python nomadnet.
func TestMarkedLinkTargetGolden(t *testing.T) {
	t.Parallel()
	base := "aabb11223344556677889900aabb112233445566:/page/x.mu"
	cases := []struct {
		name   string
		target string
		fields []string
		want   string
	}{
		{"nil fields", base, nil, base},
		{"empty fields", base, []string{}, base},
		{"single field", base, []string{"x=1"}, base + "`x=1"},
		{"two fields", base, []string{"x=1", "y=2"}, base + "`x=1|y=2"},
		{"link field no equals", base, []string{"name"}, base + "`name"},
		{"relative three fields", "rel", []string{"a=1", "b=2", "c=3"}, "rel`a=1|b=2|c=3"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := MarkedLinkTarget(c.target, c.fields); got != c.want {
				t.Errorf("MarkedLinkTarget(%q, %v) = %q, want %q", c.target, c.fields, got, c.want)
			}
		})
	}
}
