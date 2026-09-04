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
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestIntegrationDownloadFile verifies the download backend end-to-end against
// a real nomadnetwork node serving a file over the in-memory PipeInterface
// transport. The client establishes a link, requests /file/report.txt, and
// saves the response to a downloads directory. The saved name, size, and
// contents must match the served file — transitively asserting the request
// path bytes selected the file handler (the node only returns file bytes for
// the registered /file/<name> path), exercising DownloadFile's
// fetchBytes + SaveDownload path. Mirrors Python Browser.download_file +
// file_received.
func TestIntegrationDownloadFile(t *testing.T) {
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

	baseDir := testutils.TempDir(t, "browser-download")
	pagesDir := filepath.Join(baseDir, "pages")
	filesDir := filepath.Join(baseDir, "files")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fileContent := "The quick brown fox.\nLine two.\n"
	if err := os.WriteFile(filepath.Join(filesDir, "report.txt"), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	n := node.NewNode("BrowserDownloadNode", pagesDir, filesDir, 720, 0, 0, false)
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

	downloadsDir := filepath.Join(baseDir, "downloads")
	savedName, savedSize, err := DownloadFile(context.Background(), tsClient, nodeHash, "/file/report.txt", nil, downloadsDir, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if savedName != "report.txt" {
		t.Errorf("DownloadFile savedName = %q, want report.txt", savedName)
	}
	if savedSize != len(fileContent) {
		t.Errorf("DownloadFile savedSize = %v, want %v", savedSize, len(fileContent))
	}
	got, err := os.ReadFile(filepath.Join(downloadsDir, savedName))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != fileContent {
		t.Errorf("DownloadFile content = %q, want %q", truncStr(got, 80), fileContent)
	}

	// A second download of the same file collides → report.txt.1, with the
	// same contents (mirrors Python file_received's collision counter).
	savedName2, _, err := DownloadFile(context.Background(), tsClient, nodeHash, "/file/report.txt", nil, downloadsDir, 15*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("DownloadFile 2: %v", err)
	}
	if savedName2 != "report.txt.1" {
		t.Errorf("DownloadFile 2 savedName = %q, want report.txt.1", savedName2)
	}
	got2, err := os.ReadFile(filepath.Join(downloadsDir, savedName2))
	if err != nil {
		t.Fatalf("read second saved file: %v", err)
	}
	if string(got2) != fileContent {
		t.Errorf("DownloadFile 2 content = %q, want %q", truncStr(got2, 80), fileContent)
	}
}
