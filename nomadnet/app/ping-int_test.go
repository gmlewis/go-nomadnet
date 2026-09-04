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

//go:build integration

package app

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAppPingPeer verifies App.PingPeer end-to-end against Python's
// _ping_peer_from_dialog (Conversations.py:705-768): after appB announces its
// LXMF delivery destination, appA recalls appB's identity, opens an outbound
// lxmf.delivery link, and on establishment reports "Pong in N ms (M hops)" via
// the injected setStatus callback. This is the integration-level counterpart
// to the FormatPongResult golden table: it exercises the real RNS link
// establishment flow over the interconnected test transports.
func TestAppPingPeer(t *testing.T) {
	t.Parallel()
	appA, appB, cleanup := setupTwoNodeApps(t)
	defer cleanup()

	// appB announces its LXMF delivery destination so appA learns appB's
	// identity for appB's LXMFDest.Hash (Python's RNS.Identity.recall path).
	// InitWithTransport does not auto-announce (only the async initRNS path
	// does, gated on PeerAnnounceAtStart), so announce explicitly.
	appB.AnnounceNow()

	// Wait for appA to record appB's LXMF delivery announce as a "peer"
	// announce, which also means appA's transport has a path + the identity.
	peerHash := fmt.Sprintf("%x", appB.LXMFDest.Hash)
	deadline := time.Now().Add(10 * time.Second)
	var saw bool
	for time.Now().Before(deadline) {
		for _, ev := range appA.DirAnnounceEvents() {
			if ev.AnnounceType == "peer" && fmt.Sprintf("%x", ev.SourceHash) == peerHash {
				saw = true
				break
			}
		}
		if saw {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !saw {
		t.Fatalf("appA never received appB's peer announce for %v", peerHash)
	}

	// Collect every setStatus call. PingPeer invokes setStatus from the link's
	// established/closed callbacks (background goroutines), so guard with a
	// mutex and signal each update on a channel.
	var mu sync.Mutex
	var statuses []string
	updated := make(chan string, 16)
	setStatus := func(s string) {
		mu.Lock()
		statuses = append(statuses, s)
		mu.Unlock()
		select {
		case updated <- s:
		default:
		}
	}

	appA.PingPeer(peerHash, setStatus)

	// Wait for a "Pong in …" status. Link establishment over the pipe transport
	// takes a couple of seconds (cf. node-int_test link waits of 10s); allow 15s.
	deadline = time.Now().Add(15 * time.Second)
	var final string
	for time.Now().Before(deadline) {
		select {
		case s := <-updated:
			final = s
			if strings.HasPrefix(s, "Pong in ") {
				break
			}
			continue
		case <-time.After(200 * time.Millisecond):
			continue
		}
		if strings.HasPrefix(final, "Pong in ") {
			break
		}
	}
	if !strings.HasPrefix(final, "Pong in ") {
		mu.Lock()
		all := append([]string(nil), statuses...)
		mu.Unlock()
		t.Fatalf("PingPeer did not report a pong; last status=%q all=%v", final, all)
	}
	if !strings.Contains(final, "ms") {
		t.Errorf("pong status %q missing ms unit", final)
	}
}
