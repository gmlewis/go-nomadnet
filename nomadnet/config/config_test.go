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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()

	// Logging defaults
	if c.Logging.LogLevel != 4 {
		t.Errorf("LogLevel = %d, want 4", c.Logging.LogLevel)
	}
	if c.Logging.Destination != "file" {
		t.Errorf("Destination = %q, want %q", c.Logging.Destination, "file")
	}

	// Client defaults
	if !c.Client.EnableClient {
		t.Error("EnableClient = false, want true")
	}
	if c.Client.UserInterface != "text" {
		t.Errorf("UserInterface = %q, want %q", c.Client.UserInterface, "text")
	}
	if c.Client.DownloadsPath != "~/Downloads" {
		t.Errorf("DownloadsPath = %q, want %q", c.Client.DownloadsPath, "~/Downloads")
	}
	if c.Client.AnnounceInterval != 360*60 {
		t.Errorf("AnnounceInterval = %d, want %d", c.Client.AnnounceInterval, 360*60)
	}
	if c.Client.LXMFSyncLimit != 8 {
		t.Errorf("LXMFSyncLimit = %d, want 8", c.Client.LXMFSyncLimit)
	}
	if c.Client.RequiredStampCost != nil {
		t.Errorf("RequiredStampCost = %v, want nil", *c.Client.RequiredStampCost)
	}
	if c.Client.MaxAcceptedSize != 500 {
		t.Errorf("MaxAcceptedSize = %f, want 500", c.Client.MaxAcceptedSize)
	}

	// TextUI defaults
	if c.TextUI.IntroTime != 1 {
		t.Errorf("IntroTime = %f, want 1", c.TextUI.IntroTime)
	}
	if c.TextUI.Theme != "dark" {
		t.Errorf("Theme = %q, want %q", c.TextUI.Theme, "dark")
	}
	if c.TextUI.ColorMode != "24bit" {
		t.Errorf("ColorMode = %q, want %q", c.TextUI.ColorMode, "24bit")
	}
	if c.TextUI.Glyphs != "nerdfont" {
		t.Errorf("Glyphs = %q, want %q", c.TextUI.Glyphs, "nerdfont")
	}
	if !c.TextUI.MouseEnabled {
		t.Error("MouseEnabled = false, want true")
	}
	if c.TextUI.Editor != "nano" {
		t.Errorf("Editor = %q, want %q", c.TextUI.Editor, "nano")
	}

	// RRC defaults
	if c.RRC.HistoryPerRoomCap != 500 {
		t.Errorf("HistoryPerRoomCap = %d, want 500", c.RRC.HistoryPerRoomCap)
	}
	if c.RRC.FilterLoadedHistory != true {
		t.Error("FilterLoadedHistory = false, want true")
	}
	if c.RRC.EphemeralNotices != 10 {
		t.Errorf("EphemeralNotices = %f, want 10", c.RRC.EphemeralNotices)
	}
	if !c.RRC.NickColors {
		t.Error("NickColors = false, want true")
	}
	if !c.RRC.JustifyMsgs {
		t.Error("JustifyMsgs = false, want true")
	}
	if c.RRC.SpaceMsgs {
		t.Error("SpaceMsgs = true, want false")
	}

	// Node defaults
	if c.Node.EnableNode {
		t.Error("EnableNode = true, want false")
	}
	if c.Node.AnnounceInterval != 360*60 {
		t.Errorf("Node.AnnounceInterval = %d, want %d", c.Node.AnnounceInterval, 360*60)
	}
	if c.Node.PropagationCost != 16 {
		t.Errorf("PropagationCost = %d, want 16", c.Node.PropagationCost)
	}
	if c.Node.MaxTransferSize != 256 {
		t.Errorf("MaxTransferSize = %f, want 256", c.Node.MaxTransferSize)
	}
	if c.Node.MaxSyncSize != 10240 {
		t.Errorf("MaxSyncSize = %f, want 10240", c.Node.MaxSyncSize)
	}
	if c.Node.MessageStorageLimit != 2000 {
		t.Errorf("MessageStorageLimit = %f, want 2000", c.Node.MessageStorageLimit)
	}

	// Printing defaults
	if c.Printing.PrintMessages {
		t.Error("PrintMessages = true, want false")
	}
	if c.Printing.PrintCommand != "lp" {
		t.Errorf("PrintCommand = %q, want %q", c.Printing.PrintCommand, "lp")
	}
}

func TestAsBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"true", true},
		{"True", true},
		{"1", true},
		{"no", false},
		{"No", false},
		{"false", false},
		{"0", false},
		{"", false},
		{" maybe ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := asBool(tt.input)
			if got != tt.want {
				t.Errorf("asBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAsInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"-1", -1, false},
		{" 10 ", 10, false},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := asInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("asInt(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("asInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestAsFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"3.14", 3.14, false},
		{"500", 500, false},
		{" 2.5 ", 2.5, false},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := asFloat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("asFloat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("asFloat(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestAsList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a, b, c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"a,,b,", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := asList(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("asList(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("asList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestApplyCustomValues(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()
	c.Raw = map[string]map[string]string{
		"client": {
			"enable_client":     "no",
			"announce_interval": "60",
			"required_stamp_cost": "42",
			"max_accepted_size": "1024.5",
		},
		"textui": {
			"theme":    "light",
			"glyphs":   "unicode",
			"editor":   "vim",
		},
		"node": {
			"enable_node":       "yes",
			"node_name":         "MyNode",
			"propagation_cost":  "20",
			"max_peers":         "10",
		},
	}

	c.Apply()

	if c.Client.EnableClient {
		t.Error("EnableClient = true, want false")
	}
	if c.Client.AnnounceInterval != 60*60 {
		t.Errorf("AnnounceInterval = %d, want %d", c.Client.AnnounceInterval, 60*60)
	}
	if c.Client.RequiredStampCost == nil || *c.Client.RequiredStampCost != 42 {
		t.Error("RequiredStampCost != 42")
	}
	if c.Client.MaxAcceptedSize != 1024.5 {
		t.Errorf("MaxAcceptedSize = %f, want 1024.5", c.Client.MaxAcceptedSize)
	}

	if c.TextUI.Theme != "light" {
		t.Errorf("Theme = %q, want %q", c.TextUI.Theme, "light")
	}
	if c.TextUI.Glyphs != "unicode" {
		t.Errorf("Glyphs = %q, want %q", c.TextUI.Glyphs, "unicode")
	}
	if c.TextUI.Editor != "vim" {
		t.Errorf("Editor = %q, want %q", c.TextUI.Editor, "vim")
	}

	if !c.Node.EnableNode {
		t.Error("EnableNode = false, want true")
	}
	if c.Node.NodeName != "MyNode" {
		t.Errorf("NodeName = %q, want %q", c.Node.NodeName, "MyNode")
	}
	if c.Node.PropagationCost != 20 {
		t.Errorf("PropagationCost = %d, want 20", c.Node.PropagationCost)
	}
	if c.Node.MaxPeers == nil || *c.Node.MaxPeers != 10 {
		t.Error("MaxPeers != 10")
	}
}

func TestApplyLogLevelClamp(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()
	c.Raw = map[string]map[string]string{
		"logging": {"loglevel": "99"},
	}
	c.Apply()
	if c.Logging.LogLevel != 7 {
		t.Errorf("LogLevel = %d, want 7 (clamped)", c.Logging.LogLevel)
	}

	c2 := DefaultConfig()
	c2.Raw = map[string]map[string]string{
		"logging": {"loglevel": "-5"},
	}
	c2.Apply()
	if c2.Logging.LogLevel != 0 {
		t.Errorf("LogLevel = %d, want 0 (clamped)", c2.Logging.LogLevel)
	}
}

func TestApplyAnnounceIntervalMin(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()
	c.Raw = map[string]map[string]string{
		"client": {"announce_interval": "10"},
	}
	c.Apply()
	if c.Client.AnnounceInterval != 30*60 {
		t.Errorf("AnnounceInterval = %d, want %d (min clamped)", c.Client.AnnounceInterval, 30*60)
	}
}

func TestApplyNodePropagationCostMin(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()
	c.Raw = map[string]map[string]string{
		"node": {"propagation_cost": "5"},
	}
	c.Apply()
	if c.Node.PropagationCost != 13 {
		t.Errorf("PropagationCost = %d, want 13 (min clamped)", c.Node.PropagationCost)
	}
}

func TestApplyNodeNameNone(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()
	c.Raw = map[string]map[string]string{
		"node": {"node_name": "None"},
	}
	c.Apply()
	if c.Node.NodeName != "" {
		t.Errorf("NodeName = %q, want empty", c.Node.NodeName)
	}
}

func TestParseINI(t *testing.T) {
	t.Parallel()

	content := `[logging]
loglevel = 6
destination = stdout

[client]
enable_client = yes
announce_interval = 120
# this is a comment

[node]
enable_node = no
`
	dir := tempDir(t)
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	raw := parseINI(f)

	if raw["logging"]["loglevel"] != "6" {
		t.Errorf("logging.loglevel = %q, want %q", raw["logging"]["loglevel"], "6")
	}
	if raw["logging"]["destination"] != "stdout" {
		t.Errorf("logging.destination = %q, want %q", raw["logging"]["destination"], "stdout")
	}
	if raw["client"]["enable_client"] != "yes" {
		t.Errorf("client.enable_client = %q, want %q", raw["client"]["enable_client"], "yes")
	}
	if raw["client"]["announce_interval"] != "120" {
		t.Errorf("client.announce_interval = %q, want %q", raw["client"]["announce_interval"], "120")
	}
	if raw["node"]["enable_node"] != "no" {
		t.Errorf("node.enable_node = %q, want %q", raw["node"]["enable_node"], "no")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	configPath := filepath.Join(dir, "config")

	// Create a custom config
	c := DefaultConfig()
	c.Client.EnableClient = false
	c.TextUI.Theme = "light"
	c.Node.EnableNode = true
	c.Node.NodeName = "TestNode"

	// Save it
	if err := Save(c, configPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load it back
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Client.EnableClient {
		t.Error("EnableClient = true after round-trip, want false")
	}
	if loaded.TextUI.Theme != "light" {
		t.Errorf("Theme = %q after round-trip, want %q", loaded.TextUI.Theme, "light")
	}
	if !loaded.Node.EnableNode {
		t.Error("EnableNode = false after round-trip, want true")
	}
	if loaded.Node.NodeName != "TestNode" {
		t.Errorf("NodeName = %q after round-trip, want %q", loaded.Node.NodeName, "TestNode")
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	configPath := filepath.Join(dir, "nonexistent", "config")

	c, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}

	// Should return default config
	if !c.Client.EnableClient {
		t.Error("Missing file should return default config with EnableClient=true")
	}
}

func TestRequiredStampCostNone(t *testing.T) {
	t.Parallel()

	c := DefaultConfig()
	c.Raw = map[string]map[string]string{
		"client": {"required_stamp_cost": "None"},
	}
	c.Apply()
	if c.Client.RequiredStampCost != nil {
		t.Errorf("RequiredStampCost = %v, want nil", c.Client.RequiredStampCost)
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-config-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
