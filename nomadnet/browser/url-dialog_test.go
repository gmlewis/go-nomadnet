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

package browser

import "testing"

// TestNormalizeEnteredURLGolden pins Python Browser.url_dialog's input
// normalization (Browser.py:1144-1146): when the entered text contains no
// backtick and contains a colon, the FIRST "|" (at pos>0) is rewritten to a
// backtick so a user can type "hash:path|x=1|y=2" instead of "hash:path`x=1|y=2".
// Golden values captured from the installed Python nomadnet.
func TestNormalizeEnteredURLGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no colon no pipe", "abc123", "abc123"},
		{"colon no pipe", "a:b", "a:b"},
		{"single pipe", "a:b|x=1", "a:b`x=1"},
		{"only first pipe", "a:b|x=1|y=2", "a:b`x=1|y=2"},
		{"backtick present skip", "a:b`x=1", "a:b`x=1"},
		{"pipe after colon", "a:|x", "a:`x"},
		{"no colon", "abc", "abc"},
		{"pipe at pos 0 no transform", "|x=1", "|x=1"},
		{"real url field merge", "hash12345678901234567890123456789012:/page/index.mu|f=v",
			"hash12345678901234567890123456789012:/page/index.mu`f=v"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeEnteredURL(c.in); got != c.want {
				t.Errorf("NormalizeEnteredURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
