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

package main

import "testing"

// TestFreeLoopbackPortIPv6 verifies a bare IPv6 literal bind address is accepted.
// The helper joins the address and port itself, so an unbracketed literal such
// as "::1" became "::1:0" and net.Listen rejected it with
// "too many colons in address", making -bind unusable with IPv6.
func TestFreeLoopbackPortIPv6(t *testing.T) {
	t.Parallel()

	port, err := freeLoopbackPort("::1")
	if err != nil {
		t.Fatalf("freeLoopbackPort(%q) = %v, want a usable port", "::1", err)
	}
	if port <= 0 {
		t.Fatalf("freeLoopbackPort(%q) = %v, want a positive port", "::1", port)
	}
}
