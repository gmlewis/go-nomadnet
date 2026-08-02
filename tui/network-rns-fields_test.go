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

package tui

import (
	"testing"
)

// TestFormatHopsStr pins the KnownNodeInfo "Distance" string (Network.py:791-
// 800): "1 hop" for a single hop, "<N> hops" otherwise, and "Unknown" when the
// path is unknown (hops == PATHFINDER_M, surfaced by the app as a nil pointer).
func TestFormatHopsStr(t *testing.T) {
	t.Parallel()
	one := 1
	three := 3
	zero := 0
	tests := []struct {
		name string
		hops *int
		want string
	}{
		{"nil unknown", nil, "Unknown"},
		{"one hop", &one, "1 hop"},
		{"three hops", &three, "3 hops"},
		{"zero hops", &zero, "0 hops"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatHopsStr(tc.hops); got != tc.want {
				t.Errorf("FormatHopsStr(%v) = %q, want %q", tc.hops, got, tc.want)
			}
		})
	}
}

// TestFormatLXMFAddrStr pins the KnownNodeInfo centered PN-address line
// (Network.py:629-634): when the node identity is known it renders
// `g["sent"] + " LXMF Propagation Node Address is " + prettyhexrep(pn_hash)`;
// when the identity could not be recalled it is the literal
// "No associated Propagation Node known".
func TestFormatLXMFAddrStr(t *testing.T) {
	t.Parallel()
	sent := "↑"
	pnHash := []byte{0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89}
	want := sent + " LXMF Propagation Node Address is <abcdef0123456789>"
	if got := FormatLXMFAddrStr(pnHash, sent); got != want {
		t.Errorf("known identity:\ngot  %q\nwant %q", got, want)
	}
	if got := FormatLXMFAddrStr(nil, sent); got != "No associated Propagation Node known" {
		t.Errorf("nil pnHash = %q, want %q", got, "No associated Propagation Node known")
	}
}
