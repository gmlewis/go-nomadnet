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
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
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
		t.Errorf("JobInterval = %d, want 5", a.JobInterval)
	}
	if a.DeferJobs != 90 {
		t.Errorf("DeferJobs = %d, want 90", a.DeferJobs)
	}
	if a.AnnounceInterval != 21600 {
		t.Errorf("AnnounceInterval = %d, want 21600", a.AnnounceInterval)
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
		t.Errorf("LXMFSyncInterval = %d, want 21600", a.LXMFSyncInterval)
	}
	if a.LXMFSyncLimit != 8 {
		t.Errorf("LXMFSyncLimit = %d, want 8", a.LXMFSyncLimit)
	}
	if a.NodePropagationCost != 16 {
		t.Errorf("NodePropagationCost = %d, want 16", a.NodePropagationCost)
	}
	if a.RRCHistoryPerRoomCap != 500 {
		t.Errorf("RRCHistoryPerRoomCap = %d, want 500", a.RRCHistoryPerRoomCap)
	}
	if !a.RRCFilterLoadedHistory {
		t.Error("RRCFilterLoadedHistory = false, want true")
	}
	if a.RRCEphemeralNotices != 600 {
		t.Errorf("RRCEphemeralNotices = %d, want 600", a.RRCEphemeralNotices)
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
	// Not parallel — Init() modifies globalApp

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
			t.Errorf("directory %s not created", d)
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
		t.Errorf("AnnounceInterval = %d, want %d", a.AnnounceInterval, 120*60)
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
	// Not parallel — Init() modifies globalApp

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

func TestSharedInstance(t *testing.T) {
	// Not parallel — tests share globalApp

	// Reset global state
	globalMu.Lock()
	globalApp = nil
	appOnce = sync.Once{}
	globalMu.Unlock()

	if SharedInstance() != nil {
		t.Error("SharedInstance() = non-nil before Init")
	}

	dir := tempDir(t)
	a := NewApp(dir, "", false, false)
	if err := a.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if SharedInstance() != a {
		t.Error("SharedInstance() != a after Init")
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
		t.Errorf("UINone = %d, want 0", UINone)
	}
	if UIText != 1 {
		t.Errorf("UIText = %d, want 1", UIText)
	}
	if UIGraphical != 2 {
		t.Errorf("UIGraphical = %d, want 2", UIGraphical)
	}
	if UIWeb != 3 {
		t.Errorf("UIWeb = %d, want 3", UIWeb)
	}
	if UIMenu != 4 {
		t.Errorf("UIMenu = %d, want 4", UIMenu)
	}
}

func TestAppWithConfigFile(t *testing.T) {
	// Not parallel — Init() modifies globalApp

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

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-app-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
