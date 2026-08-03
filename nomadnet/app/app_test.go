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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-reticulum/rns"
)

func TestNewAppDefaults(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	if a.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", a.Version, "0.1.0")
	}
	if a.ConfigDir != dir {
		t.Errorf("ConfigDir = %q, want %q", a.ConfigDir, dir)
	}
	if !a.EnableClient {
		t.Error("EnableClient = false, want true")
	}
	if a.ShouldRunJobs != true {
		t.Error("ShouldRunJobs = false, want true")
	}
	if a.JobInterval != 5 {
		t.Errorf("JobInterval = %v, want 5", a.JobInterval)
	}
	if a.DeferJobs != 90 {
		t.Errorf("DeferJobs = %v, want 90", a.DeferJobs)
	}
	if a.AnnounceInterval != 21600 {
		t.Errorf("AnnounceInterval = %v, want 21600", a.AnnounceInterval)
	}
	if !a.PeerAnnounceAtStart {
		t.Error("PeerAnnounceAtStart = false, want true")
	}
	if !a.TryPropagationOnFail {
		t.Error("TryPropagationOnFail = false, want true")
	}
	if !a.DisablePropagation {
		t.Error("DisablePropagation = false, want true")
	}
	if !a.NotifyOnNewMessage {
		t.Error("NotifyOnNewMessage = false, want true")
	}
	if !a.ComposeMarkdown {
		t.Error("ComposeMarkdown = false, want true")
	}
	if !a.PeriodicLXMFSync {
		t.Error("PeriodicLXMFSync = false, want true")
	}
	if a.LXMFSyncInterval != 21600 {
		t.Errorf("LXMFSyncInterval = %v, want 21600", a.LXMFSyncInterval)
	}
	if a.LXMFSyncLimit != 8 {
		t.Errorf("LXMFSyncLimit = %v, want 8", a.LXMFSyncLimit)
	}
	if a.NodePropagationCost != 16 {
		t.Errorf("NodePropagationCost = %v, want 16", a.NodePropagationCost)
	}
	if a.RRCHistoryPerRoomCap != 500 {
		t.Errorf("RRCHistoryPerRoomCap = %v, want 500", a.RRCHistoryPerRoomCap)
	}
	if !a.RRCFilterLoadedHistory {
		t.Error("RRCFilterLoadedHistory = false, want true")
	}
	if a.RRCEphemeralNotices != 600 {
		t.Errorf("RRCEphemeralNotices = %v, want 600", a.RRCEphemeralNotices)
	}
	if !a.RRCNickColors {
		t.Error("RRCNickColors = false, want true")
	}
	if !a.RRCColorMentionTimestamps {
		t.Error("RRCColorMentionTimestamps = false, want true")
	}
	if !a.RRCUIJustifyMsgs {
		t.Error("RRCUIJustifyMsgs = false, want true")
	}
	if !a.RRCUIRenderMarkdown {
		t.Error("RRCUIRenderMarkdown = false, want true")
	}
	if !a.RRCUIRenderMicron {
		t.Error("RRCUIRenderMicron = false, want true")
	}
	if a.PrintCommand != "lp" {
		t.Errorf("PrintCommand = %q, want %q", a.PrintCommand, "lp")
	}
}

func TestNewAppPaths(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	if a.ConfigPath != filepath.Join(dir, "config") {
		t.Errorf("ConfigPath = %q", a.ConfigPath)
	}
	if a.StoragePath != filepath.Join(dir, "storage") {
		t.Errorf("StoragePath = %q", a.StoragePath)
	}
	if a.IdentityPath != filepath.Join(dir, "storage", "identity") {
		t.Errorf("IdentityPath = %q", a.IdentityPath)
	}
	if a.ConversationPath != filepath.Join(dir, "storage", "conversations") {
		t.Errorf("ConversationPath = %q", a.ConversationPath)
	}
	if a.DirectoryPath != filepath.Join(dir, "storage", "directory") {
		t.Errorf("DirectoryPath = %q", a.DirectoryPath)
	}
	if a.PeerSettingsPath != filepath.Join(dir, "storage", "peersettings") {
		t.Errorf("PeerSettingsPath = %q", a.PeerSettingsPath)
	}
	if a.PagesPath != filepath.Join(dir, "storage", "pages") {
		t.Errorf("PagesPath = %q", a.PagesPath)
	}
	if a.FilesPath != filepath.Join(dir, "storage", "files") {
		t.Errorf("FilesPath = %q", a.FilesPath)
	}
	if a.AttachmentPath != filepath.Join(dir, "storage", "attachments") {
		t.Errorf("AttachmentPath = %q", a.AttachmentPath)
	}
	if a.ExamplesPath != filepath.Join(dir, "examples") {
		t.Errorf("ExamplesPath = %q", a.ExamplesPath)
	}
}

func TestAppInit(t *testing.T) {
	// Not parallel — Init() starts real RNS/LXMF subsystems.

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify directories were created
	for _, d := range []string{
		a.StoragePath, a.CachePath, a.ResourcePath,
		a.ConversationPath, a.PagesPath, a.FilesPath,
		a.TmpFilesPath, a.AttachmentPath,
	} {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			t.Errorf("directory %v not created", d)
		}
	}

	// Verify subsystems
	if a.Config == nil {
		t.Error("Config is nil after Init")
	}
	if a.Dir == nil {
		t.Error("Dir is nil after Init")
	}
	if a.RRC == nil {
		t.Error("RRC is nil after Init")
	}
}

func TestAppApplyConfig(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	cfg := config.DefaultConfig()
	cfg.Client.EnableClient = false
	cfg.Client.AnnounceInterval = 120 * 60
	cfg.Client.NotifyOnNewMessage = false
	cfg.TextUI.Theme = "light"
	cfg.RRC.NickColors = false
	cfg.Node.EnableNode = true
	cfg.Node.NodeName = "MyNode"
	cfg.Printing.PrintMessages = true

	a.applyConfig(cfg)

	if a.EnableClient {
		t.Error("EnableClient = true after applying config with false")
	}
	if a.AnnounceInterval != 120*60 {
		t.Errorf("AnnounceInterval = %v, want %v", a.AnnounceInterval, 120*60)
	}
	if a.NotifyOnNewMessage {
		t.Error("NotifyOnNewMessage = true after applying config with false")
	}
	if a.RRCNickColors {
		t.Error("RRCNickColors = true after applying config with false")
	}
	if !a.EnableNode {
		t.Error("EnableNode = false after applying config with true")
	}
	if a.NodeName != "MyNode" {
		t.Errorf("NodeName = %q, want %q", a.NodeName, "MyNode")
	}
	if !a.PrintMessages {
		t.Error("PrintMessages = false after applying config with true")
	}
}

func TestAppShutdown(t *testing.T) {
	// Not parallel — Init() starts real RNS/LXMF subsystems.

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)
	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	a.Shutdown()

	if a.ShouldRunJobs {
		t.Error("ShouldRunJobs = true after Shutdown")
	}
}

func TestAppRequestLXMFSync(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	before := time.Now()
	a.RequestLXMFSync(8)

	if a.LastLXMFSync.Before(before) {
		t.Error("LastLXMFSync not updated")
	}
}

func TestAppAnnounceNow(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	before := time.Now()
	a.AnnounceNow()

	if a.LastAnnounce.Before(before) {
		t.Error("LastAnnounce not updated")
	}
}

func TestAppConversationListEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)

	list := a.ConversationList()
	if list != nil {
		t.Errorf("ConversationList = %v, want nil", list)
	}
}

func TestExpandUser(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	tests := []struct {
		input, want string
	}{
		{"~/Downloads", filepath.Join(home, "Downloads")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := expandUser(tt.input)
			if got != tt.want {
				t.Errorf("expandUser(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAppUIModeConstants(t *testing.T) {
	t.Parallel()

	if UINone != 0 {
		t.Errorf("UINone = %v, want 0", UINone)
	}
	if UIText != 1 {
		t.Errorf("UIText = %v, want 1", UIText)
	}
	if UIGraphical != 2 {
		t.Errorf("UIGraphical = %v, want 2", UIGraphical)
	}
	if UIWeb != 3 {
		t.Errorf("UIWeb = %v, want 3", UIWeb)
	}
	if UIMenu != 4 {
		t.Errorf("UIMenu = %v, want 4", UIMenu)
	}
}

func TestAppWithConfigFile(t *testing.T) {
	// Not parallel — Init() starts real RNS/LXMF subsystems.

	dir := tempDir(t)
	configPath := filepath.Join(dir, "config")

	cfg := config.DefaultConfig()
	cfg.Client.EnableClient = false
	cfg.Node.EnableNode = true
	cfg.Node.NodeName = "TestNode"
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	a := NewApp(dir, "", false, false)
	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if a.EnableClient {
		t.Error("EnableClient = true, want false (loaded from config)")
	}
	if !a.EnableNode {
		t.Error("EnableNode = false, want true (loaded from config)")
	}
	if a.NodeName != "TestNode" {
		t.Errorf("NodeName = %q, want %q", a.NodeName, "TestNode")
	}
}

func TestNewAppWithTransportWithInjectedTransport(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))

	if a.Transport != ts {
		t.Error("Transport not set from option")
	}
	if a.Identity != id {
		t.Error("Identity not set from option")
	}
	if a.ConfigDir != dir {
		t.Errorf("ConfigDir = %q, want %q", a.ConfigDir, dir)
	}
}

func TestNewAppWithTransportWithoutOptionsMatchesNewApp(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	a := NewAppWithTransport(dir)

	if a.ConfigDir != dir {
		t.Errorf("ConfigDir = %q, want %q", a.ConfigDir, dir)
	}
	if a.Transport != nil {
		t.Error("Transport should be nil without option")
	}
	if a.Identity != nil {
		t.Error("Identity should be nil without option")
	}
	if a.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", a.Version, "0.1.0")
	}
}

func TestInitWithTransportCreatesRouter(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	defer a.Shutdown()

	if a.Router == nil {
		t.Error("Router should be created by InitWithTransport")
	}
	if a.LXMFDest == nil {
		t.Error("LXMFDest should be registered by InitWithTransport")
	}
	if a.Dir == nil {
		t.Error("Dir should be initialized by InitWithTransport")
	}
}

func TestInitWithTransportReceivesAnnounces(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	defer a.Shutdown()

	if a.AnnounceCount() != 0 {
		t.Errorf("AnnounceCount = %v, want 0", a.AnnounceCount())
	}

	// Simulate receiving a peer announce directly
	a.handleLXMFAnnounce(id.Hash, id, []byte("test"), false)

	if a.AnnounceCount() != 1 {
		t.Errorf("AnnounceCount = %v, want 1 after announce", a.AnnounceCount())
	}
}

// TestAnnounceStreamNewestFirst pins Python's announce-stream ordering
// (Directory.py: the per-type lists use list.insert(0, ...), so each type is
// newest-first; announce_stream = _node_announces+_peer_announces+_pn_announces).
// The Network panel's AnnounceStream filters by tab, so within each type the
// most-recently-received announce must come first. The app-level Announces list
// feeds the TUI via GetAnnounces, so it must preserve newest-first per type
// (a prepend, matching the directory), not oldest-first (an append).
func TestAnnounceStreamNewestFirst(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	defer a.Shutdown()

	// Receive node announces oldest -> newest (distinct source hashes so they
	// are not compacted away).
	old := []byte{0xaa, 0xaa, 0xaa, 0xaa}
	newer := []byte{0xbb, 0xbb, 0xbb, 0xbb}
	a.handleNodeAnnounce(old, nil, []byte("OldNode"), false)
	a.handleNodeAnnounce(newer, nil, []byte("NewNode"), false)

	got := a.GetAnnounces()
	if len(got) != 2 {
		t.Fatalf("GetAnnounces len = %v, want 2", len(got))
	}
	// Newest-first: the second-received ("NewNode") must be at index 0.
	if string(got[0].AppData) != "NewNode" || string(got[1].AppData) != "OldNode" {
		t.Errorf("GetAnnounces order = %q, %q; want newest-first %q, %q",
			got[0].AppData, got[1].AppData, "NewNode", "OldNode")
	}

	// Per-type newest-first must hold across mixed types: a peer announce
	// received between two node announces must not displace the node ordering
	// when filtered to the node tab (Python groups by type, each newest-first).
	peerHash := []byte{0xcc, 0xcc, 0xcc, 0xcc}
	// Raw UTF-8 app_data is the original LXMF announce format
	// (DisplayNameFromAppData's non-msgpack branch returns it verbatim).
	a.handleLXMFAnnounce(peerHash, nil, []byte("PeerOne"), false)

	got = a.GetAnnounces()
	// Nodes newest-first among node entries, peer newest among peer entries.
	var nodes, peers []string
	for _, ev := range got {
		switch ev.AnnounceType {
		case "node":
			nodes = append(nodes, string(ev.AppData))
		case "peer":
			peers = append(peers, string(ev.AppData))
		}
	}
	if len(nodes) != 2 || nodes[0] != "NewNode" || nodes[1] != "OldNode" {
		t.Errorf("node entries = %v, want newest-first [NewNode OldNode]", nodes)
	}
	if len(peers) != 1 || peers[0] != "PeerOne" {
		t.Errorf("peer entries = %v, want [PeerOne]", peers)
	}
}

// TestInitWithTransportSetsRRCIdentity verifies the App wires its identity
// into the RRC manager, mirroring Python RRCManager.identity (a @property that
// returns self.app.identity). After init, a.RRC.Identity() must be the very
// same identity the app holds.
func TestInitWithTransportSetsRRCIdentity(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	defer a.Shutdown()

	if a.RRC == nil {
		t.Fatal("RRC is nil after InitWithTransport")
	}
	if a.Identity == nil {
		t.Fatal("App Identity is nil after InitWithTransport")
	}
	if a.RRC.Identity() != a.Identity {
		t.Error("RRC.Identity() does not match App Identity after InitWithTransport")
	}
}

// TestDirAnnounceEventsCrossRunRetention pins the parity fix for "gonomadnet
// doesn't retain node history across runs": the Network panel must read the
// persisted directory announce stream (a.DirAnnounceEvents, mirroring Python's
// AnnounceStream widget iterating app.directory.announce_stream, Network.py:489)
// so it populates at boot from the previous run's discovered nodes, instead of
// the ephemeral a.Announces feed which is empty until a live announce arrives.
// AppA receives announces, shuts down (persisting the stream); AppB loads the
// same storage dir and must see the announces — the "same list of live nodes
// from history" the user needs for parity capture.
func TestDirAnnounceEventsCrossRunRetention(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// App A: receive a node announce and a peer announce.
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport error: %v", err)
	}
	nodeHash := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	peerHash := []byte{0x11, 0x22, 0x33, 0x44}
	a.handleNodeAnnounce(nodeHash, nil, []byte("MyNode"), false)
	// Raw UTF-8 app_data is the original LXMF announce format; the peer's
	// display name derives from it verbatim via DisplayNameFromAppData.
	a.handleLXMFAnnounce(peerHash, id, []byte("Alice"), false)

	// DirAnnounceEvents must reflect the directory stream with names derived
	// per type (node = raw app_data; peer = LXMF app_data name).
	got := a.DirAnnounceEvents()
	var nodeNames, peerNames []string
	for _, ev := range got {
		switch ev.AnnounceType {
		case "node":
			nodeNames = append(nodeNames, ev.DisplayName)
		case "peer":
			peerNames = append(peerNames, ev.DisplayName)
		}
	}
	if len(nodeNames) != 1 || nodeNames[0] != "MyNode" {
		t.Errorf("node announce display names = %v, want [MyNode]", nodeNames)
	}
	if len(peerNames) != 1 || peerNames[0] != "Alice" {
		t.Errorf("peer announce display names = %v, want [Alice]", peerNames)
	}

	// Persist (Shutdown saves entries + announce stream) and reload into a
	// fresh App over the same storage dir, simulating the next run.
	a.Shutdown()

	ts2 := rns.NewTransportSystem(nil)
	id2, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := NewAppWithTransport(dir, WithTransport(ts2), WithIdentity(id2))
	if err := b.InitWithTransport(ts2, id2); err != nil {
		t.Fatalf("AppB InitWithTransport: %v", err)
	}
	defer b.Shutdown()

	// AppB's Network panel must show the persisted announces at boot — the
	// core retention fix — without any new live announce having arrived.
	reloaded := b.DirAnnounceEvents()
	var rNode, rPeer int
	for _, ev := range reloaded {
		switch ev.AnnounceType {
		case "node":
			if ev.DisplayName == "MyNode" && string(ev.SourceHash) == string(nodeHash) {
				rNode++
			}
		case "peer":
			if ev.DisplayName == "Alice" {
				rPeer++
			}
		}
	}
	if rNode != 1 {
		t.Errorf("AppB reloaded node announce count = %v, want 1 (history not retained?)", rNode)
	}
	if rPeer != 1 {
		t.Errorf("AppB reloaded peer announce count = %v, want 1 (history not retained?)", rPeer)
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-app-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
