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

package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// lossyInterface drops the first `drop` packets of a given context that pass
// through it and forwards everything else untouched. It models the one thing a
// reliable pipe cannot: a packet lost on a real multi-hop path.
//
// The wrapper has to be the interface the transport *receives* on as well as the
// one it sends on. Otherwise the link attaches to the raw pipe underneath and
// the link's own packets bypass the wrapper entirely.
type lossyInterface struct {
	interfaces.Interface

	context int
	drop    *atomic.Int64
	dropped *atomic.Int64
	seen    *atomic.Int64
}

func (l *lossyInterface) Send(data []byte) error {
	l.seen.Add(1)
	p := rns.NewPacketFromRaw(data)
	if p != nil && p.Unpack() == nil && p.Context == l.context && l.drop.Add(-1) >= 0 {
		l.dropped.Add(1)
		return nil
	}
	return l.Interface.Send(data)
}

// newLossyPipes wires a client to a hub through a pipe whose client side is the
// lossy wrapper, attributing received packets to the wrapper so the link
// attaches to it.
func newLossyPipes(t *testing.T, tsHub, tsClient *rns.TransportSystem, drop, dropped, seen *atomic.Int64) (*interfaces.PipeInterface, *lossyInterface, func()) {
	t.Helper()

	var lossy *lossyInterface
	var hubSide *interfaces.PipeInterface
	hubSide = interfaces.NewPipeInterface("hub-side", func(data []byte, _ interfaces.Interface) {
		tsHub.Inbound(data, hubSide)
	})
	clientSide := interfaces.NewPipeInterface("client-side", func(data []byte, _ interfaces.Interface) {
		tsClient.Inbound(data, lossy)
	})
	lossy = &lossyInterface{
		Interface: clientSide,
		context:   rns.ContextResourceReq,
		drop:      drop,
		dropped:   dropped,
		seen:      seen,
	}
	hubSide.SetOther(clientSide)
	clientSide.SetOther(hubSide)

	return hubSide, lossy, func() {
		_ = hubSide.Detach()
		_ = clientSide.Detach()
	}
}

// TestIntegrationFetchPageSurvivesLostResourceRequest pins that a page fetch
// still completes when the receiver's first part requests are lost, and that
// the requests really were dropped.
//
// This covers the recovery half of the appliance's page-serving failure. A
// resource response is advertised, the receiver accepts it and asks for parts;
// on a multi-hop path those requests can be lost, and the sender then
// re-advertises because it believes the transfer never started. The receiver
// must not let the repeat turn one transfer into several — it must recognise
// the advertisement as one it is already running — and the sender must stop
// advertising once it is transferring. The defect itself is pinned
// deterministically by rns.TestAcceptIgnoresRepeatAdvertisement and
// rns.TestResourceRequestStopsReAdvertise, which fail without the fix; a
// reliable in-process pipe recovers from a lost request quickly enough that
// this end-to-end path passes either way.
func TestIntegrationFetchPageSurvivesLostResourceRequest(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsHub, cleanupHub := newStartedTS(t)
	defer cleanupHub()
	tsHub.SetEnabled(true)

	tsNode, cleanupNode := newStartedTS(t)
	defer cleanupNode()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()

	hubToNode, nodeToHub, cleanNodeLink := newBrowserPipes(t, tsHub, tsNode)
	defer cleanNodeLink()

	drop := &atomic.Int64{}
	drop.Store(2)
	dropped := &atomic.Int64{}
	seen := &atomic.Int64{}
	hubToClient, lossyClient, cleanClientLink := newLossyPipes(t, tsHub, tsClient, drop, dropped, seen)
	defer cleanClientLink()

	tsHub.RegisterInterface(hubToNode)
	tsHub.RegisterInterface(hubToClient)
	tsNode.RegisterInterface(nodeToHub)
	tsClient.RegisterInterface(lossyClient)

	baseDir := testutils.TempDir(t, "browser-fetch-lossy")
	pagesDir := filepath.Join(baseDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	page := ">> Lossy Hop\n\n" + strings.Repeat("a micron line of reasonable length\n\n", 1800) + "\nEND\n"
	if err := os.WriteFile(filepath.Join(pagesDir, "index.mu"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}

	n := node.NewNode("LossyNode", pagesDir, baseDir, 720, 0, 0, false)
	if err := n.Start(tsNode, tsNode.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 10*time.Second) {
		t.Fatal("timeout waiting for the node's announce to reach the client through the hub")
	}

	data, err := FetchPage(context.Background(), tsClient, nodeHash, "/page/index.mu", nil,
		45*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("FetchPage with lost part requests failed after %v packets sent by the client: %v", seen.Load(), err)
	}
	if dropped.Load() == 0 {
		t.Fatal("the test dropped no resource request; it did not exercise a lost-request path")
	}
	if string(data) != page {
		t.Errorf("FetchPage with lost part requests returned %v bytes, want %v", len(data), len(page))
	}
}
