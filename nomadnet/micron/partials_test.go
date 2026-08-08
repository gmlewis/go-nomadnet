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

// TestPartialDescriptorAndHash asserts the partial descriptor (components joined
// with "|") and its SHA-256 hex hash match Python parse_partial
// (MicronParser.py:187-189): partial_descriptor = "|".join(partial_components);
// partial_hash = RNS.hexrep(RNS.Identity.full_hash(descriptor.encode("utf-8")),
// delimit=False) — and RNS.Identity.full_hash is SHA-256. Golden values were
// captured from a Python harness mirroring those two lines (no urwid needed).
func TestPartialDescriptorAndHash(t *testing.T) {
	t.Parallel()

	cases := []struct {
		inner      string // text after the leading `{
		descriptor string
		hash       string
	}{
		{"page_name}", "page_name", "46c1eadf8e09ab715661114247bed0fc1fa4b8ee26d2ce3f76d4a4740d41d8c4"},
		{"page_name`5}", "page_name|5", "2a9d0af9b6ef70a12d39fe0927249671535df0af26c824b8b79214acf559a5f9"},
		{"page_name`5`pid=foo}", "page_name|5|pid=foo", "ce642fdd2f99dd6c628f49bd61e913bdfbe5dedb5f064fa452787ab8cc0a34d9"},
		{"page_name`2.5`a|b|pid=bar}", "page_name|2.5|a|b|pid=bar", "4b6d328d3669a69dc418935e68f5577b9507d2cf92ba1ad33352273fc7837a8d"},
		{"sub/page`10}", "sub/page|10", "5484ccc05b51f7ea08dddd7a6e6331af895965e6fda6afe34b1ddf82978552cf"},
	}
	for _, tc := range cases {
		t.Run(tc.inner, func(t *testing.T) {
			t.Parallel()
			nodes := Parse("`{" + tc.inner)
			if len(nodes) != 1 {
				t.Fatalf("len(nodes) = %v, want 1", len(nodes))
			}
			if got := nodes[0].PartialDescriptor; got != tc.descriptor {
				t.Errorf("PartialDescriptor = %q, want %q", got, tc.descriptor)
			}
			if got := PartialHash(nodes[0]); got != tc.hash {
				t.Errorf("PartialHash = %q, want %q", got, tc.hash)
			}
		})
	}
}

// TestPartialRenderPlaceholder asserts a partial renders the ⧖ hourglass
// placeholder span, mirroring Python's urwid.Text("⧖") (MicronParser.py:186).
func TestPartialRenderPlaceholder(t *testing.T) {
	t.Parallel()
	lines := RenderToStyledLines("`{page_name}", ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	if got := lines[0].Spans[0].Text; got != "⧖" {
		t.Errorf("placeholder text = %q, want ⧖", got)
	}
}
