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

package app

import (
	"encoding/hex"
	"os"
	"testing"
)

// TestApplyIgnoredDestinationsSeedsRouter verifies that applyIgnoredDestinations
// replays every loaded ignored destination hash into the LXMF router's ignored
// list, mirroring Python NomadNetworkApp (NomadNetworkApp.py:351-352) which
// calls message_router.ignore_destination(destination_hash) for each hash in
// the ignored list right after creating the router.
func TestApplyIgnoredDestinationsSeedsRouter(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()

	h1 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	h2 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}
	content := hex.EncodeToString(h1) + "\n" + hex.EncodeToString(h2) + "\n"
	if err := os.WriteFile(a.IgnoredPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a.loadIgnoredList()
	if len(a.IgnoredList) != 2 {
		t.Fatalf("IgnoredList len = %v, want 2", len(a.IgnoredList))
	}

	a.Router = newRouterForTest(t)
	a.applyIgnoredDestinations()

	if !a.Router.IsIgnored(h1) {
		t.Error("first ignored hash was not seeded into the LXMF router")
	}
	if !a.Router.IsIgnored(h2) {
		t.Error("second ignored hash was not seeded into the LXMF router")
	}

	// Unblocking must also lift the router-side ignore (Python
	// unblock_destination → message_router.unignore_destination,
	// NomadNetworkApp.py:592-594).
	a.UnblockDestination(h1)
	if a.Router.IsIgnored(h1) {
		t.Error("router still ignores the hash after UnblockDestination")
	}
}

// TestApplyIgnoredDestinationsNoRouter verifies the no-router case is a safe
// no-op (the list may be applied before the router exists).
func TestApplyIgnoredDestinationsNoRouter(t *testing.T) {
	t.Parallel()
	a := NewApp(tempDir(t), "", false, false)
	a.setupPaths()
	a.applyIgnoredDestinations() // must not panic
}
