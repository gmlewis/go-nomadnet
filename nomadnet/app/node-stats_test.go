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
	"bytes"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestNodeStatsCountersAndReset pins the hosted-node stat counters end to end: the
// per-event increments the node's callbacks make, the snapshot the Local Node Info panel
// renders ("Connected Now" / "Served Pages" / "Served Files"), and ResetNodeStats zeroing
// and persisting them (Python NodeInfo.stats_query, Network.py:1391-1394). Nothing
// covered these counters, so a counter that incremented the wrong field or a reset that
// did not persist would show the user a number that came back on the next start.
func TestNodeStatsCountersAndReset(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	a := newPersistApp(t, dir)

	if _, _, _, ok := a.NodeStats(); !ok {
		t.Fatal("precondition: the app has no peer settings to count into")
	}

	a.onNodePeerConnected()
	a.onNodePageServed()
	a.onNodePageServed()
	a.onNodeFileServed()

	connects, pages, files, ok := a.NodeStats()
	if !ok {
		t.Fatal("NodeStats reported no settings")
	}
	if connects != 1 || pages != 2 || files != 1 {
		t.Errorf("counters = connects %v, pages %v, files %v; want 1, 2, 1", connects, pages, files)
	}

	a.ResetNodeStats()

	if connects, pages, files, _ = a.NodeStats(); connects != 0 || pages != 0 || files != 0 {
		t.Errorf("counters after reset = %v, %v, %v; want all zero", connects, pages, files)
	}

	// The reset must reach the file: a second app over the same directory reads zeros,
	// not the pre-reset totals.
	second := newPersistApp(t, dir)
	if connects, pages, files, _ := second.NodeStats(); connects != 0 || pages != 0 || files != 0 {
		t.Errorf("counters after a restart = %v, %v, %v; want the reset values", connects, pages, files)
	}
}

// TestNodeLastAnnounceRoundTrip pins the persisted node-announce timestamp: the node's
// announce callback records it (Python Node.announce sets
// peer_settings["node_last_announce"], Node.py:218) and the Local Node Info panel reads
// it back as "Last Announce". Without the round trip the panel would show "Never" after
// every restart, which is what the go-nomadnet workaround for a nil value had to paper
// over.
func TestNodeLastAnnounceRoundTrip(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeTestNomadNetConfig(t, dir)

	a := newPersistApp(t, dir)
	a.onNodeAnnounced()

	got, ok := a.NodeLastAnnounceSetting()
	if !ok {
		t.Fatal("NodeLastAnnounceSetting reported no settings")
	}
	stamp, isInt := got.(int64)
	if !isInt {
		t.Fatalf("node last announce = %#v (%T), want an int64 unix timestamp", got, got)
	}
	if stamp <= 0 {
		t.Errorf("node last announce = %v, want a positive unix timestamp", stamp)
	}

	second := newPersistApp(t, dir)
	again, ok := second.NodeLastAnnounceSetting()
	if !ok {
		t.Fatal("the second app reported no settings")
	}
	if again != got {
		t.Errorf("node last announce after a restart = %v, want %v", again, got)
	}
}

// TestNodeDestinationHash pins the address the hosted node announces and the Network
// page shows. It must be the nomadnetwork.node destination derived from the app's
// identity — checked against a destination built through the ordinary constructor rather
// than by recomputing the same hash call — and it must be absent before the identity is
// loaded, which is the state the UI has to cope with.
func TestNodeDestinationHash(t *testing.T) {
	t.Parallel()

	a := NewApp(tempDir(t), "", false, false)
	if got := a.NodeDestinationHash(); got != nil {
		t.Errorf("NodeDestinationHash = %x before an identity is loaded, want nil", got)
	}

	a = newPersistApp(t, tempDir(t))
	hash := a.NodeDestinationHash()
	if len(hash) != rns.TruncatedHashLength/8 {
		t.Fatalf("NodeDestinationHash = %x (%v bytes), want %v", hash, len(hash), rns.TruncatedHashLength/8)
	}

	dest, err := rns.NewDestination(a.Transport, a.Identity, rns.DestinationIn, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		t.Fatalf("building the node destination: %v", err)
	}
	if !bytes.Equal(hash, dest.Hash) {
		t.Errorf("NodeDestinationHash = %x, want the nomadnetwork.node destination %x", hash, dest.Hash)
	}
}
