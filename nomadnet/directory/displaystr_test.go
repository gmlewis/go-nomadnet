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

package directory

import "testing"

func TestSimplestDisplayStrSanitize(t *testing.T) {
	t.Parallel()
	hash := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x10}
	tests := []struct {
		name          string
		sanitizeNames bool
		san           bool
		trust         byte
		displayName   string
		want          string
	}{
		{"strip_keeps_emoji", false, true, TrustTrusted, "Alice😀", "Alice😀"},
		{"sanitize_strips_emoji", true, true, TrustTrusted, "Alice😀", "Alice"},
		{"san_false_uses_strip", true, false, TrustTrusted, "Alice😀", "Alice😀"},
		{"warning_appends_hash", true, true, TrustWarning, "Bob", "Bob <" + hexString(hash) + ">"},
		{"unknown_empty_prettyhex", true, true, TrustUnknown, "", "<" + hexString(hash) + ">"},
		{"no_entry", true, true, TrustUnknown, "x", "<" + hexString(hash) + ">"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := New()
			d.SanitizeNames = tt.sanitizeNames
			if tt.displayName != "x" || tt.name == "no_entry" {
				if tt.name != "no_entry" {
					d.Remember(&Entry{SourceHash: hash, DisplayName: tt.displayName, TrustLevel: tt.trust})
				}
			}
			got := d.SimplestDisplayStrSan(hash, tt.san)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
