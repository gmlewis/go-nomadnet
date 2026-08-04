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

// Package node implements the NomadNet node that serves Micron pages
// and files to remote peers over RNS links.
//
// A Node scans configured directories for .mu page files and regular
// files, registers them as request handlers, and serves content to
// connecting peers. Access control is supported via .allowed files
// that restrict which identity hashes may access specific pages.
package node

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// ErrInvalidHex is returned when a hex string contains invalid characters.
var ErrInvalidHex = errors.New("invalid hex character")

// JobInterval is the interval in seconds between background job runs.
const JobInterval = 5

// StartAnnounceDelay is the delay in seconds before the first announce.
const StartAnnounceDelay = 6

// DefaultIndex is the default Micron home page served when index.mu
// is not found in the pages directory.
const DefaultIndex = `>Default Home Page

This node is serving pages, but the home page file (index.mu) was not found in the page storage directory. This is an auto-generated placeholder.

If you are the node operator, you can define your own home page by creating a file named ` + "`*index.mu`*" + ` in the page storage directory.
`

// DefaultNotAllowed is the response sent when a request is denied.
const DefaultNotAllowed = `>Request Not Allowed

You are not authorised to carry out the request.
`

// Node represents a NomadNet node that serves pages and files.
type Node struct {
	Name string

	PagesPath string
	FilesPath string

	AnnounceInterval    int // minutes
	PageRefreshInterval int // minutes
	FileRefreshInterval int // minutes
	AnnounceAtStart     bool

	ServedPages []string
	ServedFiles []string

	LastAnnounce    time.Time
	LastPageRefresh time.Time
	LastFileRefresh time.Time
	ShouldRunJobs   bool

	ServedPageRequests int
	ServedFileRequests int
	NodeConnects       int
	ActiveLinks        int

	AppData []byte

	// Event callbacks let the owning App persist counters and react to node
	// events without the node importing the app package (one-way dependency).
	// Each mirrors a peer_settings write in Python (Node.py:111-112,194-195,218):
	// OnPeerConnected → node_connects, OnPageServed → served_page_requests,
	// OnFileServed → served_file_requests, OnAnnounced → node_last_announce.
	OnPeerConnected func()
	OnPageServed    func()
	OnFileServed    func()
	OnAnnounced     func()

	destination *rns.Destination
	identity    *rns.Identity
	transport   rns.Transport

	mu sync.Mutex
}

// NewNode creates a new Node with the given configuration.
func NewNode(name, pagesPath, filesPath string, announceInterval, pageRefresh, fileRefresh int, announceAtStart bool) *Node {
	n := &Node{
		Name:                name,
		PagesPath:           pagesPath,
		FilesPath:           filesPath,
		AnnounceInterval:    announceInterval,
		PageRefreshInterval: pageRefresh,
		FileRefreshInterval: fileRefresh,
		AnnounceAtStart:     announceAtStart,
		ShouldRunJobs:       true,
		LastAnnounce:        time.Now(),
		LastPageRefresh:     time.Now(),
		LastFileRefresh:     time.Now(),
	}

	return n
}

// Start registers the node destination on the given transport and sets up
// request handlers for pages and files. This matches Python's Node.__init__
// which creates a Destination and registers link/request callbacks.
func (n *Node) Start(ts rns.Transport, identity *rns.Identity) error {
	dest, err := rns.NewDestination(ts, identity, rns.DestinationIn, rns.DestinationSingle, "nomadnetwork", "node")
	if err != nil {
		return err
	}

	n.destination = dest
	n.identity = identity
	n.transport = ts

	dest.SetLinkEstablishedCallback(n.PeerConnected)

	n.RegisterPages()
	n.RegisterFiles()

	n.registerRequestHandlers()

	return nil
}

// Stop shuts down the node by marking jobs as stopped.
func (n *Node) Stop() {
	n.mu.Lock()
	n.ShouldRunJobs = false
	n.mu.Unlock()
}

// ResetStats zeros the in-memory stat counters (served page/file requests and
// node connects), mirroring Python NodeInfo.stats_query (Network.py:1391-1394)
// which resets peer_settings["node_connects"]/["served_page_requests"]/
// ["served_file_requests"] to 0. The owning App persists the peer-settings
// copy; this resets the node's own bookkeeping.
func (n *Node) ResetStats() {
	n.mu.Lock()
	n.ServedPageRequests = 0
	n.ServedFileRequests = 0
	n.NodeConnects = 0
	n.mu.Unlock()
}

// Destination returns the node's RNS destination, or nil if not started.
func (n *Node) Destination() *rns.Destination {
	return n.destination
}

// ActiveLinkCount returns the number of currently-open peer links under mu, so
// the TUI render goroutine can read it without racing PeerConnected/
// PeerDisconnected, which mutate ActiveLinks from link callbacks.
func (n *Node) ActiveLinkCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ActiveLinks
}

// registerRequestHandlers registers RNS request handlers for all served
// pages and files, plus the default index.
func (n *Node) registerRequestHandlers() {
	if n.destination == nil {
		return
	}

	hasIndex := false
	for _, page := range n.ServedPages {
		if filepath.Base(page) == "index.mu" {
			hasIndex = true
		}
		reqPath := PageRequestPath(page, n.PagesPath)
		if reqPath == "" {
			continue
		}
		n.destination.RegisterRequestHandler(reqPath, n.makePageHandler(page), rns.AllowAll, nil, true)
	}

	if !hasIndex {
		n.destination.RegisterRequestHandler("/page/index.mu", n.defaultIndexHandler, rns.AllowAll, nil, true)
	}

	for _, file := range n.ServedFiles {
		reqPath := FileRequestPath(file, n.FilesPath)
		if reqPath == "" {
			continue
		}
		n.destination.RegisterRequestHandler(reqPath, n.makeFileHandler(file), rns.AllowAll, nil, true)
	}
}

// makePageHandler returns a request handler function for a specific page file.
func (n *Node) makePageHandler(filePath string) func(string, []byte, []byte, []byte, *rns.Identity, time.Time) any {
	return func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *rns.Identity, requestedAt time.Time) any {
		if !isRequestAllowed(filePath, remoteIdentity) {
			return ServeNotAllowed()
		}
		content := ServePage(filePath)
		if content == nil {
			return nil
		}
		n.mu.Lock()
		n.ServedPageRequests++
		n.mu.Unlock()
		if n.OnPageServed != nil {
			n.OnPageServed()
		}
		return content
	}
}

// makeFileHandler returns a request handler function for a specific file.
func (n *Node) makeFileHandler(filePath string) func(string, []byte, []byte, []byte, *rns.Identity, time.Time) any {
	return func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *rns.Identity, requestedAt time.Time) any {
		if !isRequestAllowed(filePath, remoteIdentity) {
			return ServeNotAllowed()
		}
		fileData, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		n.mu.Lock()
		n.ServedFileRequests++
		n.mu.Unlock()
		if n.OnFileServed != nil {
			n.OnFileServed()
		}
		return fileData
	}
}

// defaultIndexHandler serves the default index page.
func (n *Node) defaultIndexHandler(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *rns.Identity, requestedAt time.Time) any {
	return ServeDefaultIndex()
}

// Announce sends an announce for this node on the RNS network with
// the node name as app data. Matches Python's Node.announce().
func (n *Node) Announce() error {
	if n.destination == nil {
		return errors.New("node not started")
	}
	n.mu.Lock()
	n.AppData = []byte(n.Name)
	n.LastAnnounce = time.Now()
	appData := n.AppData
	n.mu.Unlock()
	if n.OnAnnounced != nil {
		n.OnAnnounced()
	}
	return n.destination.Announce(appData)
}

// PeerConnected handles a peer establishing a link to this node, mirroring
// Python Node.peer_connected: it increments the node connection count and
// registers PeerDisconnected as the link-closed callback. Python persists the
// incremented count via app.save_peer_settings; that persistence is applied at
// the app layer via OnPeerConnected when the node is wired into NomadNetworkApp.
// ActiveLinks tracks the currently-open links (Python
// len(node.destination.links)) for the "Connected Now" stat.
func (n *Node) PeerConnected(link *rns.Link) {
	n.mu.Lock()
	n.NodeConnects++
	n.ActiveLinks++
	n.mu.Unlock()
	if n.OnPeerConnected != nil {
		n.OnPeerConnected()
	}
	if link != nil {
		link.SetLinkClosedCallback(n.PeerDisconnected)
	}
}

// PeerDisconnected handles a peer link closing, mirroring Python
// Node.peer_disconnected, whose body only logs (it is a `pass`). It decrements
// the active-link count so the "Connected Now" stat stays accurate.
func (n *Node) PeerDisconnected(link *rns.Link) {
	n.mu.Lock()
	if n.ActiveLinks > 0 {
		n.ActiveLinks--
	}
	n.mu.Unlock()
}

// Jobs runs the background job loop, mirroring Python Node.__jobs. It repeats
// until Stop sets ShouldRunJobs false, sleeping JobInterval seconds between
// passes. Each pass re-announces when the announce interval has elapsed and
// re-scans the page and file directories when their refresh intervals have
// elapsed. Announce errors are logged so the loop keeps running, since Python's
// __jobs likewise has no error handling that would halt the worker.
func (n *Node) Jobs() {
	for {
		n.mu.Lock()
		run := n.ShouldRunJobs
		n.mu.Unlock()
		if !run {
			return
		}
		if err := n.runJobsOnce(time.Now()); err != nil {
			log.Printf("node jobs: %v", err)
		}
		time.Sleep(JobInterval * time.Second)
	}
}

// runJobsOnce performs a single pass of the background job loop as of now,
// mirroring one iteration of Python Node.__jobs. The announce interval is in
// minutes and has no zero-guard (matching Python, which re-announces whenever
// now exceeds last_announce). The refresh intervals are in minutes and only
// fire when greater than zero, matching Python's guards. It returns any error
// from announcing.
func (n *Node) runJobsOnce(now time.Time) error {
	n.mu.Lock()
	lastAnnounce := n.LastAnnounce
	announceInterval := n.AnnounceInterval
	pageRefresh := n.PageRefreshInterval
	lastPageRefresh := n.LastPageRefresh
	fileRefresh := n.FileRefreshInterval
	lastFileRefresh := n.LastFileRefresh
	n.mu.Unlock()

	if now.After(lastAnnounce.Add(time.Duration(announceInterval) * time.Minute)) {
		if err := n.Announce(); err != nil {
			return err
		}
	}
	if pageRefresh > 0 && now.After(lastPageRefresh.Add(time.Duration(pageRefresh)*time.Minute)) {
		n.RegisterPages()
		n.mu.Lock()
		n.LastPageRefresh = now
		n.mu.Unlock()
	}
	if fileRefresh > 0 && now.After(lastFileRefresh.Add(time.Duration(fileRefresh)*time.Minute)) {
		n.RegisterFiles()
		n.mu.Lock()
		n.LastFileRefresh = now
		n.mu.Unlock()
	}
	return nil
}

// ScanPages recursively scans a directory for .mu page files,
// excluding hidden files (starting with .) and .allowed files.
func ScanPages(basePath string) []string {
	var pages []string

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(basePath, name)
		if entry.IsDir() {
			pages = append(pages, ScanPages(fullPath)...)
		} else if !strings.HasSuffix(name, ".allowed") {
			pages = append(pages, fullPath)
		}
	}

	return pages
}

// ScanFiles recursively scans a directory for regular files,
// excluding hidden files (starting with .).
func ScanFiles(basePath string) []string {
	var files []string

	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(basePath, name)
		if entry.IsDir() {
			files = append(files, ScanFiles(fullPath)...)
		} else {
			files = append(files, fullPath)
		}
	}

	return files
}

// RegisterPages scans the pages directory and updates the served pages list.
func (n *Node) RegisterPages() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ServedPages = ScanPages(n.PagesPath)
}

// RegisterFiles scans the files directory and updates the served files list.
func (n *Node) RegisterFiles() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ServedFiles = ScanFiles(n.FilesPath)
}

// PageRequestPath returns the request path for a page file.
// For example, /pages/docs/help.mu becomes /page/docs/help.mu.
func PageRequestPath(filePath, pagesPath string) string {
	rel, err := filepath.Rel(pagesPath, filePath)
	if err != nil {
		return ""
	}
	return "/page/" + filepath.ToSlash(rel)
}

// FileRequestPath returns the request path for a served file.
// For example, /files/docs/readme.txt becomes /file/docs/readme.txt.
func FileRequestPath(filePath, filesPath string) string {
	rel, err := filepath.Rel(filesPath, filePath)
	if err != nil {
		return ""
	}
	return "/file/" + filepath.ToSlash(rel)
}

// ParseAllowedFile reads an .allowed file and returns the list of
// permitted identity hashes. Each line should be a 64-character hex string.
func ParseAllowedFile(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var hashes [][]byte
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Expected: 64 hex chars = 32 bytes
		if len(line) != 64 {
			continue
		}
		hash, err := hexDecode(line)
		if err != nil {
			continue
		}
		hashes = append(hashes, hash)
	}

	return hashes, nil
}

// isRequestAllowed checks if a request from the given identity is allowed
// to access filePath. If a .allowed file exists and the remote identity is
// nil (unidentified), access is denied. If no .allowed file exists, access
// is granted to everyone.
func isRequestAllowed(filePath string, remoteIdentity *rns.Identity) bool {
	allowedPath := filePath + ".allowed"
	if _, err := os.Stat(allowedPath); os.IsNotExist(err) {
		return true
	}
	if remoteIdentity == nil {
		return false
	}
	return IsAllowed(filePath, remoteIdentity.Hash)
}

// IsAllowed checks if a remote identity hash is in the allowed list.
// If the allowed file doesn't exist (no access control), returns true.
func IsAllowed(filePath string, remoteIdentityHash []byte) bool {
	allowedPath := filePath + ".allowed"

	if _, err := os.Stat(allowedPath); os.IsNotExist(err) {
		// No access control file — allow everyone
		return true
	}

	allowedList, err := ParseAllowedFile(allowedPath)
	if err != nil {
		return false
	}

	for _, hash := range allowedList {
		if bytesEqual(hash, remoteIdentityHash) {
			return true
		}
	}

	return false
}

// ServePage reads a page file and returns its content.
// Returns nil if the file cannot be read.
func ServePage(filePath string) []byte {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	return data
}

// ServeFile opens a file for streaming. Returns the file handle and
// the filename for the response header.
func ServeFile(filePath, filesPath string) (*os.File, string, error) {
	rel, err := filepath.Rel(filesPath, filePath)
	if err != nil {
		return nil, "", err
	}
	fileName := filepath.Base(rel)

	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}

	return f, fileName, nil
}

// ServeDefaultIndex returns the default index page content.
func ServeDefaultIndex() []byte {
	return []byte(DefaultIndex)
}

// ServeNotAllowed returns the "not allowed" response content.
func ServeNotAllowed() []byte {
	return []byte(DefaultNotAllowed)
}

// SortPages sorts a page list for deterministic ordering.
func SortPages(pages []string) []string {
	sorted := make([]string, len(pages))
	copy(sorted, pages)
	sort.Strings(sorted)
	return sorted
}

// SortFiles sorts a file list for deterministic ordering.
func SortFiles(files []string) []string {
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	return sorted
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi := hexDigit(s[i])
		lo := hexDigit(s[i+1])
		if hi < 0 || lo < 0 {
			return nil, ErrInvalidHex
		}
		b[i/2] = byte(hi<<4 | lo)
	}
	return b, nil
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
