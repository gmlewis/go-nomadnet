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

//go:build integration

package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestNodeOperatorDisplayAndPropagationHash verifies the RNS-derived Network
// page fields (Network.py:138-144, 629-634, 681-688): after appA announces its
// LXMF delivery identity, appB can recall that identity and NodeOperatorDisplay
// derives the same lxmf.delivery hash back (proving recall + CalculateHash),
// while NodePropagationHash derives the lxmf.propagation hash. An unannounced
// hash yields "Unknown" / nil.
func TestNodeOperatorDisplayAndPropagationHash(t *testing.T) {
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	// Announce both LXMF delivery identities so each side can recall the other.
	if err := appA.Router.Announce(appA.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce A: %v", err)
	}
	if err := appB.Router.Announce(appB.LXMFDest.Hash); err != nil {
		t.Fatalf("Announce B: %v", err)
	}
	waitForAnnounce(t, appA, "peer", 5*time.Second)
	waitForAnnounce(t, appB, "peer", 5*time.Second)

	// Give the announce path a moment to persist identities for recall.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if rns.RecallIdentity(appB.Transport, appA.LXMFDest.Hash) != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if rns.RecallIdentity(appB.Transport, appA.LXMFDest.Hash) == nil {
		t.Fatalf("appB could not recall appA's LXMF identity")
	}

	// NodeOperatorDisplay(appA's lxmf.delivery hash) recalls appA's identity and
	// re-derives the lxmf.delivery hash, which equals appA.LXMFDest.Hash. With no
	// directory entry for it, simplest_display_str returns the "<hex>" form.
	wantOp := "<" + fmt.Sprintf("%x", appA.LXMFDest.Hash) + ">"
	if got := appB.NodeOperatorDisplay(appA.LXMFDest.Hash); got != wantOp {
		t.Errorf("NodeOperatorDisplay = %q, want %q", got, wantOp)
	}

	// NodePropagationHash derives the lxmf.propagation hash for the same identity.
	pn := appB.NodePropagationHash(appA.LXMFDest.Hash)
	if len(pn) != len(appA.LXMFDest.Hash) {
		t.Errorf("NodePropagationHash len = %v, want %v", len(pn), len(appA.LXMFDest.Hash))
	}
	wantPN := rns.CalculateHash(appA.Identity, "lxmf", "propagation")
	if fmt.Sprintf("%x", pn) != fmt.Sprintf("%x", wantPN) {
		t.Errorf("NodePropagationHash = %x, want %x", pn, wantPN)
	}

	// An unannounced hash cannot be recalled → "Unknown" / nil.
	unknown := make([]byte, len(appA.LXMFDest.Hash))
	unknown[0] = 0xDE
	unknown[1] = 0xAD
	if got := appB.NodeOperatorDisplay(unknown); got != "Unknown" {
		t.Errorf("NodeOperatorDisplay(unknown) = %q, want %q", got, "Unknown")
	}
	if got := appB.NodePropagationHash(unknown); got != nil {
		t.Errorf("NodePropagationHash(unknown) = %x, want nil", got)
	}
}
