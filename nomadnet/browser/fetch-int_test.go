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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/interfaces"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestIntegrationFetchPageFromNode verifies the browser fetch backend end-to-end
// against a real nomadnetwork node over an in-memory PipeInterface "mocked
// transport" (the same harness node-int_test.go uses). The client recalls the
// node identity, establishes an RNS link, and issues link.Request("/page/index.mu").
// The response bytes must equal node.DefaultIndex — which transitively asserts
// the client requested the correct path (the node only returns DefaultIndex for
// /page/index.mu when no index.mu exists), exercising ParseURL's path defaulting
// and FetchPage's link-establish + request + response path.
func TestIntegrationFetchPageFromNode(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	dir := testutils.TempDir(t, "browser-fetch-node")
	n := node.NewNode("BrowserFetchNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 5*time.Second) {
		t.Fatal("timeout waiting for path to node")
	}

	data, err := FetchPage(context.Background(), tsClient, nodeHash, "/page/index.mu", nil, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if string(data) != node.DefaultIndex {
		t.Errorf("FetchPage default index = %q, want node.DefaultIndex (len %v)", truncStr(data, 60), len(node.DefaultIndex))
	}
}

// TestIntegrationFetchPageCustomPath verifies FetchPage returns the node's actual
// served page (not DefaultIndex) when index.mu exists — pinning that the request
// path bytes select the right server-side handler.
func TestIntegrationFetchPageCustomPath(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	baseDir := testutils.TempDir(t, "browser-fetch-custom")
	pagesDir := filepath.Join(baseDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pageContent := ">Custom Page\n\nFetched over RNS.\n"
	if err := os.WriteFile(filepath.Join(pagesDir, "index.mu"), []byte(pageContent), 0o644); err != nil {
		t.Fatal(err)
	}

	n := node.NewNode("BrowserCustomNode", pagesDir, baseDir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 5*time.Second) {
		t.Fatal("timeout waiting for path to node")
	}

	data, err := FetchPage(context.Background(), tsClient, nodeHash, "/page/index.mu", nil, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if string(data) != pageContent {
		t.Errorf("FetchPage custom page = %q, want %q", truncStr(data, 80), pageContent)
	}
}

// TestIntegrationFetchPageViaParseURL wires ParseURL → FetchPage together: parse
// a "<hash>:/page/index.mu" URL, then fetch with the parsed hash+path. The
// response must equal DefaultIndex. This pins the full retrieveURL pipeline
// (parse the RNS address → establish a link → request the page → handle the
// response).
func TestIntegrationFetchPageViaParseURL(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	dir := testutils.TempDir(t, "browser-fetch-parseurl")
	n := node.NewNode("BrowserParseNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 5*time.Second) {
		t.Fatal("timeout waiting for path to node")
	}

	// A bare 32-hex hash URL → path defaults to /page/index.mu (ParseURL).
	url := hexEncode(nodeHash)
	dest, path, rd, err := ParseURL(url, nil, nil)
	if err != nil {
		t.Fatalf("ParseURL(%q): %v", url, err)
	}
	if path != DefaultPath {
		t.Errorf("ParseURL path = %q, want %q", path, DefaultPath)
	}
	if rd != nil {
		t.Errorf("ParseURL requestData = %v, want nil", rd)
	}

	data, err := FetchPage(context.Background(), tsClient, dest, path, rd, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if string(data) != node.DefaultIndex {
		t.Errorf("FetchPage via ParseURL = %q, want DefaultIndex", truncStr(data, 60))
	}
}

// TestIntegrationFetchPageOnLinkEstablished pins the onLinkEstablished hook
// (Python Browser.link_established, Browser.py:1454-1459): when supplied, it is
// invoked once the link is ACTIVE and before the page response arrives — the
// point at which Python identifies to the remote node. The hook must observe an
// ACTIVE link, and the fetch must still complete with the page bytes.
func TestIntegrationFetchPageOnLinkEstablished(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	dir := testutils.TempDir(t, "browser-fetch-hook")
	n := node.NewNode("BrowserHookNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}

	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 5*time.Second) {
		t.Fatal("timeout waiting for path to node")
	}

	var hookStatus int
	var hookCalled bool
	hook := func(link *rns.Link) {
		hookStatus = link.GetStatus()
		hookCalled = true
	}
	data, err := FetchPage(context.Background(), tsClient, nodeHash, "/page/index.mu", nil, 15*time.Second, nil, hook)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if !hookCalled {
		t.Fatal("onLinkEstablished hook was not invoked")
	}
	if hookStatus != rns.LinkActive {
		t.Errorf("hook observed link status %#x, want LinkActive (%#x)", hookStatus, rns.LinkActive)
	}
	if string(data) != node.DefaultIndex {
		t.Errorf("FetchPage = %q, want DefaultIndex", truncStr(data, 60))
	}
}

// TestIntegrationFetchPagePreCancelledCtx pins the context-cancellation guard
// at the top of fetchBytes: a fetch made with an ALREADY-cancelled ctx must
// return context.Canceled immediately, without touching the network. This is
// the backend half of the "stale Connect overtakes the current page" fix — a
// superseding Connect cancels the prior fetch's ctx, so when the prior fetch
// reaches the top guard it bails out deterministically (before any link or
// select with random ordering). The render-gate half is unit-tested in
// tui/browser_request_test.go.
func TestIntegrationFetchPagePreCancelledCtx(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	dir := testutils.TempDir(t, "browser-fetch-ctx")
	n := node.NewNode("BrowserCtxNode", dir, dir, 720, 0, 0, false)
	if err := n.Start(tsServer, tsServer.Identity()); err != nil {
		t.Fatalf("node Start: %v", err)
	}
	defer n.Stop()
	if err := n.Announce(); err != nil {
		t.Fatalf("node Announce: %v", err)
	}
	nodeHash := n.Destination().Hash
	if !waitForPath(tsClient, nodeHash, 5*time.Second) {
		t.Fatal("timeout waiting for path to node")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := FetchPage(ctx, tsClient, nodeHash, "/page/index.mu", nil, 15*time.Second, nil, nil)
	if err == nil {
		t.Fatal("FetchPage with pre-cancelled ctx returned nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("FetchPage err = %v, want context.Canceled", err)
	}
}

// --- harness helpers (in-memory two-node RNS over a PipeInterface pair). ---

func newStartedTS(t *testing.T) (*rns.TransportSystem, func()) {
	t.Helper()
	dir := testutils.TempDir(t, "browser-fetch-ts")
	ts := rns.NewTransportSystem(nil)
	if err := ts.Start(filepath.Join(dir, "storage")); err != nil {
		t.Fatalf("ts.Start: %v", err)
	}
	return ts, func() { ts.Stop() }
}

func newBrowserPipes(t *testing.T, tsA, tsB *rns.TransportSystem) (*interfaces.PipeInterface, *interfaces.PipeInterface, func()) {
	t.Helper()
	pipeA := interfaces.NewPipeInterface("a", func(data []byte, iface interfaces.Interface) {
		tsA.Inbound(data, iface)
	})
	pipeB := interfaces.NewPipeInterface("b", func(data []byte, iface interfaces.Interface) {
		tsB.Inbound(data, iface)
	})
	pipeA.SetOther(pipeB)
	pipeB.SetOther(pipeA)
	cleanup := func() {
		_ = pipeA.Detach()
		_ = pipeB.Detach()
	}
	return pipeA, pipeB, cleanup
}

func waitForPath(ts *rns.TransportSystem, destHash []byte, timeout time.Duration) bool {
	return testutils.PollUntil(timeout, func() bool {
		return ts.HasPath(destHash)
	})
}

func truncStr(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
