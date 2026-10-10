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
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestIntegrationFetchPageOverTwoHops reproduces the appliance's page-serving
// failure from the outside: a node on one end of a two-hop path, a client on
// the other, and a transport in between.
//
// The tablet's node announces, the mesh has a path, and an inbound link from
// outside establishes and goes ACTIVE — but the /page/… request served as an
// RNS resource never completes, so the requestor sees nothing at all and no
// error. A single-process, single-hop PipeInterface fetch cannot see this: it
// never crosses a transport that has to forward the packet, and it never has
// to carry a multi-part resource across one.
//
// The page is deliberately larger than one link MDU (the tablet serves a
// ~60 KB index.mu), so the response travels as an RNS resource with dozens of
// parts rather than as a single ContextResponse packet.
func TestIntegrationFetchPageOverTwoHops(t *testing.T) {
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
	hubToClient, clientToHub, cleanClientLink := newBrowserPipes(t, tsHub, tsClient)
	defer cleanClientLink()
	tsHub.RegisterInterface(hubToNode)
	tsHub.RegisterInterface(hubToClient)
	tsNode.RegisterInterface(nodeToHub)
	tsClient.RegisterInterface(clientToHub)

	baseDir := testutils.TempDir(t, "browser-fetch-2hop")
	pagesDir := filepath.Join(baseDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The tablet's served page is nomadnet/app/default-index.mu (~60 KB), which
	// RNS carries as a resource of dozens of parts.
	page := ">> Two Hop\n\n" + strings.Repeat("micron line of reasonable length\n\n", 1800) + "\nEND\n"
	if err := os.WriteFile(filepath.Join(pagesDir, "index.mu"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}

	n := node.NewNode("TwoHopNode", pagesDir, baseDir, 720, 0, 0, false)
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

	var lastProgress float64
	data, err := FetchPage(context.Background(), tsClient, nodeHash, "/page/index.mu", nil,
		45*time.Second, func(p float64) { lastProgress = p }, nil)
	if err != nil {
		t.Fatalf("FetchPage over two hops failed at progress %v of 1: %v", lastProgress, err)
	}
	if string(data) != page {
		t.Errorf("FetchPage over two hops returned %v bytes, want %v", len(data), len(page))
	}
}
