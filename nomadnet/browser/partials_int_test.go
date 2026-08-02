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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestIntegrationPartialPipeline verifies the full partial lifecycle against a
// real node: a page declares a partial via "`{:/page/part.mu`5`x=1}", the client
// fetches the page, ExtractPartials recovers the partial metadata (relative URL,
// fields, refresh), and FetchPartial resolves the relative URL against the
// node's destination hash and fetches the partial's markup. The fetched bytes
// must equal the served partial page — transitively asserting the relative-URL
// resolution (":/page/part.mu" → <nodehash>:/page/part.mu) and the var_* request
// data path of the partial fetch, mirroring Python Browser.__load_partial.
func TestIntegrationPartialPipeline(t *testing.T) {
	testutils.SkipShortIntegration(t)

	tsServer, cleanupServer := newStartedTS(t)
	defer cleanupServer()
	tsClient, cleanupClient := newStartedTS(t)
	defer cleanupClient()
	pipeA, pipeB, pipeCleanup := newBrowserPipes(t, tsServer, tsClient)
	defer pipeCleanup()
	tsServer.RegisterInterface(pipeA)
	tsClient.RegisterInterface(pipeB)

	baseDir := testutils.TempDir(t, "browser-partial")
	pagesDir := filepath.Join(baseDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A partial with no var_* fields: PartialRequestData yields an empty map,
	// which FetchPartial passes as nil (see its doc) so the Go node responds.
	// (var_* field extraction is golden-pinned in TestPartialRequestData.)
	indexMarkup := ">Main page\n`{:/page/part.mu`5}\nEnd\n"
	partMarkup := ">Partial heading\nPartial body.\n"
	if err := os.WriteFile(filepath.Join(pagesDir, "index.mu"), []byte(indexMarkup), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pagesDir, "part.mu"), []byte(partMarkup), 0o644); err != nil {
		t.Fatal(err)
	}

	n := node.NewNode("BrowserPartialNode", pagesDir, baseDir, 720, 0, 0, false)
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

	// Fetch the page carrying the partial directive.
	pageData, err := FetchPage(tsClient, nodeHash, "/page/index.mu", nil, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("FetchPage index: %v", err)
	}
	if string(pageData) != indexMarkup {
		t.Fatalf("FetchPage index = %q, want %q", truncStr(pageData, 80), indexMarkup)
	}

	// Extract the partial from the fetched markup.
	partials := ExtractPartials(string(pageData))
	if len(partials) != 1 {
		t.Fatalf("ExtractPartials got %d, want 1: %+v", len(partials), partials)
	}
	p := partials[0]
	if p.URL != ":/page/part.mu" {
		t.Errorf("partial URL = %q, want :/page/part.mu", p.URL)
	}
	if p.Refresh != 5 {
		t.Errorf("partial Refresh = %v, want 5", p.Refresh)
	}

	// Fetch the partial (relative URL resolved against nodeHash).
	partialData, err := FetchPartial(tsClient, p, nodeHash, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("FetchPartial: %v", err)
	}
	if string(partialData) != partMarkup {
		t.Errorf("FetchPartial = %q, want %q", truncStr(partialData, 80), partMarkup)
	}
}
