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

package node

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

func TestScanPages(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)

	// Create page files
	writeFile(t, filepath.Join(dir, "index.mu"), "index content")
	writeFile(t, filepath.Join(dir, "about.mu"), "about content")

	// Create subdirectory with pages
	subDir := filepath.Join(dir, "docs")
	mkdir(t, subDir)
	writeFile(t, filepath.Join(subDir, "help.mu"), "help content")

	// Create files that should be excluded
	writeFile(t, filepath.Join(dir, ".hidden"), "hidden")
	writeFile(t, filepath.Join(dir, "readme.txt"), "readme")
	writeFile(t, filepath.Join(dir, "index.mu.allowed"), "allowed list")

	pages := ScanPages(dir)
	sorted := SortPages(pages)

	// Python scan_pages includes ALL non-hidden, non-.allowed files
	if len(sorted) != 4 {
		t.Fatalf("ScanPages len = %v, want 4: %v", len(sorted), sorted)
	}

	// Check that expected files are found (full paths)
	found := make(map[string]bool)
	for _, p := range sorted {
		found[filepath.Base(p)] = true
	}

	if !found["index.mu"] {
		t.Error("index.mu not found")
	}
	if !found["about.mu"] {
		t.Error("about.mu not found")
	}
	if !found["help.mu"] {
		t.Error("help.mu not found")
	}
	if !found["readme.txt"] {
		t.Error("readme.txt not found (scan_pages includes all non-hidden files)")
	}

	// Check exclusions
	for _, p := range sorted {
		base := filepath.Base(p)
		if base == ".hidden" || base == "index.mu.allowed" {
			t.Errorf("Should not include %v", base)
		}
	}
}

func TestScanPagesEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	pages := ScanPages(dir)
	if len(pages) != 0 {
		t.Errorf("ScanPages on empty dir = %v, want 0", len(pages))
	}
}

func TestScanPagesMissing(t *testing.T) {
	t.Parallel()

	pages := ScanPages("/nonexistent/path")
	if len(pages) != 0 {
		t.Errorf("ScanPages on missing dir = %v, want 0", len(pages))
	}
}

func TestScanFiles(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)

	// Create files
	writeFile(t, filepath.Join(dir, "readme.txt"), "readme")
	writeFile(t, filepath.Join(dir, "data.json"), "{}")

	// Create subdirectory with files
	subDir := filepath.Join(dir, "docs")
	mkdir(t, subDir)
	writeFile(t, filepath.Join(subDir, "guide.txt"), "guide")

	// Create hidden files (should be excluded)
	writeFile(t, filepath.Join(dir, ".hidden"), "hidden")

	files := ScanFiles(dir)
	sorted := SortFiles(files)

	if len(sorted) != 3 {
		t.Fatalf("ScanFiles len = %v, want 3: %v", len(sorted), sorted)
	}

	found := make(map[string]bool)
	for _, f := range sorted {
		found[filepath.Base(f)] = true
	}

	if !found["readme.txt"] {
		t.Error("readme.txt not found")
	}
	if !found["data.json"] {
		t.Error("data.json not found")
	}
	if !found["guide.txt"] {
		t.Error("guide.txt not found")
	}

	// Check exclusion
	for _, f := range sorted {
		if filepath.Base(f) == ".hidden" {
			t.Error("Should not include .hidden")
		}
	}
}

func TestScanFilesEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	files := ScanFiles(dir)
	if len(files) != 0 {
		t.Errorf("ScanFiles on empty dir = %v, want 0", len(files))
	}
}

func TestScanFilesMissing(t *testing.T) {
	t.Parallel()

	files := ScanFiles("/nonexistent/path")
	if len(files) != 0 {
		t.Errorf("ScanFiles on missing dir = %v, want 0", len(files))
	}
}

func TestPageRequestPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		filePath, pagesPath, want string
	}{
		{"/pages/index.mu", "/pages", "/page/index.mu"},
		{"/pages/docs/help.mu", "/pages", "/page/docs/help.mu"},
		{"/pages/a/b/c.mu", "/pages", "/page/a/b/c.mu"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := PageRequestPath(tt.filePath, tt.pagesPath)
			if got != tt.want {
				t.Errorf("PageRequestPath(%q, %q) = %q, want %q", tt.filePath, tt.pagesPath, got, tt.want)
			}
		})
	}
}

func TestFileRequestPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		filePath, filesPath, want string
	}{
		{"/files/readme.txt", "/files", "/file/readme.txt"},
		{"/files/docs/guide.txt", "/files", "/file/docs/guide.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := FileRequestPath(tt.filePath, tt.filesPath)
			if got != tt.want {
				t.Errorf("FileRequestPath(%q, %q) = %q, want %q", tt.filePath, tt.filesPath, got, tt.want)
			}
		})
	}
}

func TestParseAllowedFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)

	// Create an allowed file with 3 valid hashes
	hash1 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hash2 := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	content := hash1 + "\n" + hash2 + "\n# comment\n\n" + hash2 + "\n"
	path := filepath.Join(dir, "test.allowed")
	writeFile(t, path, content)

	hashes, err := ParseAllowedFile(path)
	if err != nil {
		t.Fatalf("ParseAllowedFile: %v", err)
	}

	// Should have 3 entries (hash2 appears twice)
	if len(hashes) != 3 {
		t.Fatalf("ParseAllowedFile len = %v, want 3", len(hashes))
	}

	// Verify first hash
	if len(hashes[0]) != 32 {
		t.Errorf("hash[0] len = %v, want 32", len(hashes[0]))
	}
}

func TestParseAllowedFileInvalidLength(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)

	// Create an allowed file with invalid hash lengths
	content := "0123456789abcdef\n" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"
	path := filepath.Join(dir, "test.allowed")
	writeFile(t, path, content)

	hashes, err := ParseAllowedFile(path)
	if err != nil {
		t.Fatalf("ParseAllowedFile: %v", err)
	}

	// Only the 64-char hash should be included
	if len(hashes) != 1 {
		t.Errorf("ParseAllowedFile len = %v, want 1", len(hashes))
	}
}

func TestIsAllowedNoFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	pagePath := filepath.Join(dir, "page.mu")
	writeFile(t, pagePath, "content")

	// No .allowed file — should allow everyone
	if !IsAllowed(pagePath, []byte{0x01}) {
		t.Error("IsAllowed should return true when no .allowed file exists")
	}
}

func TestIsAllowedWithFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	pagePath := filepath.Join(dir, "page.mu")
	writeFile(t, pagePath, "content")

	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	allowedPath := pagePath + ".allowed"
	writeFile(t, allowedPath, hash+"\n")

	// Allowed hash
	allowedHash, _ := hexDecode(hash)
	if !IsAllowed(pagePath, allowedHash) {
		t.Error("IsAllowed should return true for listed hash")
	}

	// Not-allowed hash
	if IsAllowed(pagePath, []byte{0xFF, 0xFE}) {
		t.Error("IsAllowed should return false for unlisted hash")
	}
}

func TestServePage(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	pagePath := filepath.Join(dir, "test.mu")
	writeFile(t, pagePath, "page content here")

	data := ServePage(pagePath)
	if string(data) != "page content here" {
		t.Errorf("ServePage = %q, want %q", string(data), "page content here")
	}
}

func TestServePageMissing(t *testing.T) {
	t.Parallel()

	data := ServePage("/nonexistent/page.mu")
	if data != nil {
		t.Errorf("ServePage for missing file = %v, want nil", data)
	}
}

func TestServeFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeFile(t, filepath.Join(dir, "test.txt"), "file content")

	f, name, err := ServeFile(filepath.Join(dir, "test.txt"), dir)
	if err != nil {
		t.Fatalf("ServeFile: %v", err)
	}
	defer func() { _ = f.Close() }()

	if name != "test.txt" {
		t.Errorf("filename = %q, want %q", name, "test.txt")
	}

	buf := make([]byte, 100)
	n, _ := f.Read(buf)
	if string(buf[:n]) != "file content" {
		t.Errorf("file content = %q, want %q", string(buf[:n]), "file content")
	}
}

func TestServeDefaultIndex(t *testing.T) {
	t.Parallel()

	data := ServeDefaultIndex()
	if len(data) == 0 {
		t.Error("ServeDefaultIndex returned empty")
	}
	s := string(data)
	if s != DefaultIndex {
		t.Error("ServeDefaultIndex does not match DefaultIndex constant")
	}
}

func TestServeNotAllowed(t *testing.T) {
	t.Parallel()

	data := ServeNotAllowed()
	if len(data) == 0 {
		t.Error("ServeNotAllowed returned empty")
	}
	s := string(data)
	if s != DefaultNotAllowed {
		t.Error("ServeNotAllowed does not match DefaultNotAllowed constant")
	}
}

// TestServeDefaultIndexPythonParity verifies that the default index and
// not-allowed markup bytes match the Python NomadNet Node.DEFAULT_INDEX and
// Node.DEFAULT_NOTALLOWED constants exactly. The reference bytes were
// captured from the Python source into testdata.
func TestServeDefaultIndexPythonParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  []byte
		fix  string
	}{
		{"index", ServeDefaultIndex(), filepath.Join("testdata", "py_default_index.mu")},
		{"notallowed", ServeNotAllowed(), filepath.Join("testdata", "py_default_notallowed.mu")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(tc.fix)
			if err != nil {
				t.Fatal(err)
			}
			if string(tc.got) != string(want) {
				t.Errorf("%v markup differs from Python reference:\n got  (%v bytes): %q\n want (%v bytes): %q",
					tc.name, len(tc.got), string(tc.got), len(want), string(want))
			}
		})
	}
}

func TestNewNode(t *testing.T) {
	t.Parallel()

	n := NewNode("TestNode", "/pages", "/files", 60, 30, 15, true)

	if n.Name != "TestNode" {
		t.Errorf("Name = %q, want %q", n.Name, "TestNode")
	}
	if n.PagesPath != "/pages" {
		t.Errorf("PagesPath = %q, want %q", n.PagesPath, "/pages")
	}
	if n.FilesPath != "/files" {
		t.Errorf("FilesPath = %q, want %q", n.FilesPath, "/files")
	}
	if n.AnnounceInterval != 60 {
		t.Errorf("AnnounceInterval = %v, want 60", n.AnnounceInterval)
	}
	if n.PageRefreshInterval != 30 {
		t.Errorf("PageRefreshInterval = %v, want 30", n.PageRefreshInterval)
	}
	if n.FileRefreshInterval != 15 {
		t.Errorf("FileRefreshInterval = %v, want 15", n.FileRefreshInterval)
	}
	if !n.AnnounceAtStart {
		t.Error("AnnounceAtStart = false, want true")
	}
	if !n.ShouldRunJobs {
		t.Error("ShouldRunJobs = false, want true")
	}
}

func TestNodeRegisterPages(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	pagesDir := filepath.Join(dir, "pages")
	mkdir(t, pagesDir)
	writeFile(t, filepath.Join(pagesDir, "index.mu"), "index")
	writeFile(t, filepath.Join(pagesDir, "about.mu"), "about")

	n := NewNode("Test", pagesDir, "", 60, 0, 0, false)
	n.RegisterPages()

	if len(n.ServedPages) != 2 {
		t.Errorf("ServedPages len = %v, want 2", len(n.ServedPages))
	}
}

func TestNodeRegisterFiles(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	filesDir := filepath.Join(dir, "files")
	mkdir(t, filesDir)
	writeFile(t, filepath.Join(filesDir, "readme.txt"), "readme")
	writeFile(t, filepath.Join(filesDir, "data.json"), "{}")

	n := NewNode("Test", "", filesDir, 60, 0, 0, false)
	n.RegisterFiles()

	if len(n.ServedFiles) != 2 {
		t.Errorf("ServedFiles len = %v, want 2", len(n.ServedFiles))
	}
}

func TestNodeConstants(t *testing.T) {
	t.Parallel()

	if JobInterval != 5 {
		t.Errorf("JobInterval = %v, want 5", JobInterval)
	}
	if StartAnnounceDelay != 6 {
		t.Errorf("StartAnnounceDelay = %v, want 6", StartAnnounceDelay)
	}
}

func TestSortPages(t *testing.T) {
	t.Parallel()

	pages := []string{"/c.mu", "/a.mu", "/b.mu"}
	sorted := SortPages(pages)

	if len(sorted) != 3 {
		t.Fatalf("SortPages len = %v, want 3", len(sorted))
	}
	if sorted[0] != "/a.mu" || sorted[1] != "/b.mu" || sorted[2] != "/c.mu" {
		t.Errorf("SortPages = %v, want [/a.mu /b.mu /c.mu]", sorted)
	}

	// Original should not be modified
	if pages[0] != "/c.mu" {
		t.Error("SortPages modified original slice")
	}
}

func TestSortFiles(t *testing.T) {
	t.Parallel()

	files := []string{"/z.txt", "/a.txt", "/m.txt"}
	sorted := SortFiles(files)

	if sorted[0] != "/a.txt" || sorted[1] != "/m.txt" || sorted[2] != "/z.txt" {
		t.Errorf("SortFiles = %v, want [/a.txt /m.txt /z.txt]", sorted)
	}
}

func TestHexDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  []byte
		err   bool
	}{
		{"01020304", []byte{1, 2, 3, 4}, false},
		{"abcdef", []byte{0xab, 0xcd, 0xef}, false},
		{"ABCDEF", []byte{0xab, 0xcd, 0xef}, false},
		{"00", []byte{0}, false},
		{"xyz", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := hexDecode(tt.input)
			if (err != nil) != tt.err {
				t.Errorf("hexDecode(%q) error = %v, wantErr %v", tt.input, err, tt.err)
				return
			}
			if !tt.err && !bytesEqual(got, tt.want) {
				t.Errorf("hexDecode(%q) = %x, want %x", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultIndexContent(t *testing.T) {
	t.Parallel()

	if DefaultIndex == "" {
		t.Error("DefaultIndex is empty")
	}
}

func TestDefaultNotAllowedContent(t *testing.T) {
	t.Parallel()

	if DefaultNotAllowed == "" {
		t.Error("DefaultNotAllowed is empty")
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-node-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestStartCreatesDestination(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := tempDir(t)
	n := NewNode("TestNode", dir, dir, 720, 0, 0, false)

	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if n.destination == nil {
		t.Error("destination should be set after Start")
	}
	if n.destination.Type != rns.DestinationSingle {
		t.Errorf("destination type = %v, want DestinationSingle", n.destination.Type)
	}
}

func TestStartRegistersRequestHandlers(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	pagesDir := tempDir(t)
	writeFile(t, filepath.Join(pagesDir, "index.mu"), ">Home Page\nWelcome!")

	n := NewNode("TestNode", pagesDir, pagesDir, 720, 0, 0, false)

	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if n.destination == nil {
		t.Fatal("destination not set after Start")
	}

	if len(n.ServedPages) == 0 {
		t.Error("ServedPages should not be empty after Start")
	}
}

func TestAnnounceSetsAppData(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := tempDir(t)
	n := NewNode("TestNode", dir, dir, 720, 0, 0, false)

	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer n.Stop()

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce error: %v", err)
	}

	if n.AppData == nil {
		t.Error("AppData should be set after Announce")
	}
	if string(n.AppData) != "TestNode" {
		t.Errorf("AppData = %q, want %q", string(n.AppData), "TestNode")
	}
}

// TestAnnounceAppDataParity verifies that Node.Announce encodes the announce
// app_data exactly as Python Node.announce does: self.app_data =
// self.name.encode("utf-8"), passed as app_data to destination.announce. A
// non-ASCII name exercises UTF-8 encoding parity. The announce packet's data
// payload is pubkey + nameHash + randomHash + ratchet + signature + app_data,
// so the packet data must end with the name's UTF-8 bytes.
func TestAnnounceAppDataParity(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	const name = "TëstNödë ☕"
	dir := tempDir(t)
	n := NewNode(name, dir, dir, 720, 0, 0, false)

	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer n.Stop()

	if err := n.Announce(); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	wantAppData := []byte(name)
	if !bytes.Equal(n.AppData, wantAppData) {
		t.Errorf("AppData = %x, want %x (name UTF-8)", n.AppData, wantAppData)
	}

	pkt, err := n.destination.BuildAnnouncePacket(wantAppData)
	if err != nil {
		t.Fatalf("BuildAnnouncePacket: %v", err)
	}
	if !bytes.HasSuffix(pkt.Data, wantAppData) {
		t.Errorf("announce packet data does not end with app_data:\n data = %x\n want suffix = %x", pkt.Data, wantAppData)
	}
}

// TestPeerConnectedIncrementsConnects verifies that PeerConnected, the
// link-established callback, increments the node connection count — matching
// Python Node.peer_connected which does app.peer_settings["node_connects"] += 1.
func TestPeerConnectedIncrementsConnects(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := tempDir(t)
	n := NewNode("N", dir, dir, 720, 0, 0, false)
	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer n.Stop()

	if n.NodeConnects != 0 {
		t.Fatalf("initial NodeConnects = %v, want 0", n.NodeConnects)
	}

	n.PeerConnected(&rns.Link{})
	if n.NodeConnects != 1 {
		t.Errorf("after 1 connect, NodeConnects = %v, want 1", n.NodeConnects)
	}

	n.PeerConnected(&rns.Link{})
	if n.NodeConnects != 2 {
		t.Errorf("after 2 connects, NodeConnects = %v, want 2", n.NodeConnects)
	}
}

// TestPeerDisconnectedNoop verifies that PeerDisconnected, the link-closed
// callback, does not alter the connection count — matching Python
// Node.peer_disconnected which only logs (its body is `pass`).
func TestPeerDisconnectedNoop(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := tempDir(t)
	n := NewNode("N", dir, dir, 720, 0, 0, false)
	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer n.Stop()

	n.PeerConnected(&rns.Link{})
	before := n.NodeConnects
	n.PeerDisconnected(&rns.Link{})
	if n.NodeConnects != before {
		t.Errorf("PeerDisconnected changed NodeConnects from %v to %v", before, n.NodeConnects)
	}
}

// TestRunJobsOnce verifies a single pass of the background job loop mirrors
// Python Node.__jobs: it re-announces when now exceeds last_announce plus the
// announce interval (minutes, no zero-guard), and re-scans pages/files when
// now exceeds the last refresh plus the refresh interval (minutes, only when
// the interval is greater than zero).
func TestRunJobsOnce(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name           string
		announceMin    int
		pageRefreshMin int
		fileRefreshMin int
		lastAnnounce   time.Time
		lastPage       time.Time
		lastFile       time.Time
		now            time.Time
		wantAnnounce   bool
		wantPage       bool
		wantFile       bool
	}{
		{
			name: "announce_due", announceMin: 30, pageRefreshMin: 0, fileRefreshMin: 0,
			lastAnnounce: base.Add(-31 * time.Minute), lastPage: base, lastFile: base, now: base,
			wantAnnounce: true,
		},
		{
			name: "announce_not_due", announceMin: 30, pageRefreshMin: 0, fileRefreshMin: 0,
			lastAnnounce: base.Add(-10 * time.Minute), lastPage: base, lastFile: base, now: base,
			wantAnnounce: false,
		},
		{
			name: "page_refresh_due", announceMin: 9999, pageRefreshMin: 10, fileRefreshMin: 0,
			lastAnnounce: base, lastPage: base.Add(-11 * time.Minute), lastFile: base, now: base,
			wantPage: true,
		},
		{
			name: "page_refresh_disabled", announceMin: 9999, pageRefreshMin: 0, fileRefreshMin: 0,
			lastAnnounce: base, lastPage: base.Add(-11 * time.Minute), lastFile: base, now: base,
			wantPage: false,
		},
		{
			name: "file_refresh_due", announceMin: 9999, pageRefreshMin: 0, fileRefreshMin: 10,
			lastAnnounce: base, lastPage: base, lastFile: base.Add(-11 * time.Minute), now: base,
			wantFile: true,
		},
		{
			name: "file_refresh_disabled", announceMin: 9999, pageRefreshMin: 0, fileRefreshMin: 0,
			lastAnnounce: base, lastPage: base, lastFile: base.Add(-11 * time.Minute), now: base,
			wantFile: false,
		},
		{
			name: "all_due", announceMin: 30, pageRefreshMin: 10, fileRefreshMin: 10,
			lastAnnounce: base.Add(-31 * time.Minute), lastPage: base.Add(-11 * time.Minute),
			lastFile: base.Add(-11 * time.Minute), now: base,
			wantAnnounce: true, wantPage: true, wantFile: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := rns.NewTransportSystem(nil)
			id, err := rns.NewIdentity(true, nil)
			if err != nil {
				t.Fatal(err)
			}
			dir := tempDir(t)
			n := NewNode("N", dir, dir, tc.announceMin, tc.pageRefreshMin, tc.fileRefreshMin, false)
			if err := n.Start(ts, id); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer n.Stop()

			n.LastAnnounce = tc.lastAnnounce
			n.LastPageRefresh = tc.lastPage
			n.LastFileRefresh = tc.lastFile

			if err := n.runJobsOnce(tc.now); err != nil {
				t.Fatalf("runJobsOnce: %v", err)
			}

			if tc.wantAnnounce {
				if string(n.AppData) != "N" {
					t.Errorf("announce should have fired; AppData = %q, want %q", string(n.AppData), "N")
				}
			} else {
				if n.AppData != nil {
					t.Errorf("announce should not have fired; AppData = %q, want nil", string(n.AppData))
				}
			}

			wantPageRefresh := tc.lastPage
			if tc.wantPage {
				wantPageRefresh = tc.now
			}
			if !n.LastPageRefresh.Equal(wantPageRefresh) {
				t.Errorf("LastPageRefresh = %v, want %v", n.LastPageRefresh, wantPageRefresh)
			}

			wantFileRefresh := tc.lastFile
			if tc.wantFile {
				wantFileRefresh = tc.now
			}
			if !n.LastFileRefresh.Equal(wantFileRefresh) {
				t.Errorf("LastFileRefresh = %v, want %v", n.LastFileRefresh, wantFileRefresh)
			}
		})
	}
}

// TestJobsReturnsWhenStopped verifies that Jobs exits its loop promptly when
// ShouldRunJobs is false, rather than blocking on the sleep between passes.
func TestJobsReturnsWhenStopped(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := tempDir(t)
	n := NewNode("N", dir, dir, 9999, 0, 0, false)
	if err := n.Start(ts, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	n.Stop()

	done := make(chan struct{})
	go func() {
		n.Jobs()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Jobs did not return after Stop")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
