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

// Package config provides configuration parsing and management for NomadNet.
//
// NomadNet uses ConfigObj (INI-like) config files with sections for
// logging, client, textui, rrc, node, and printing settings.
package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all NomadNet configuration settings parsed from the
// INI-style config file.
type Config struct {
	Logging  LoggingConfig
	Client   ClientConfig
	TextUI   TextUIConfig
	RRC      RRCConfig
	Node     NodeConfig
	Printing PrintingConfig

	// Raw stores the parsed key-value pairs from the config file.
	Raw map[string]map[string]string
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	LogLevel    int
	Destination string
	LogFile     string
}

// ClientConfig holds client/application settings.
type ClientConfig struct {
	EnableClient             bool
	UserInterface            string
	DownloadsPath            string
	AttachmentSavePath       string
	NotifyOnNewMessage       bool
	AnnounceAtStart          bool
	AnnounceInterval         int // seconds
	TryPropagationOnSendFail bool
	PeriodicLXMFSync         bool
	LXMFSyncInterval         int // seconds
	LXMFSyncLimit            int
	RequiredStampCost        *int
	AcceptInvalidStamps      bool
	MaxAcceptedSize          float64 // kilobytes
	CompactAnnounceStream    bool
	ComposeInMarkdown        bool
}

// TextUIConfig holds text UI settings.
type TextUIConfig struct {
	IntroTime     float64
	Theme         string
	ColorMode     string
	Glyphs        string
	MouseEnabled  bool
	Editor        string
	HideGuide     bool
	SanitizeNames bool
	ClipboardCopy bool
}

// RRCConfig holds Reticulum Relay Chat settings.
type RRCConfig struct {
	HistoryPerRoomCap      int
	FilterLoadedHistory    bool
	EphemeralNotices       float64 // minutes
	ColorMentionTimestamps bool
	RenderMarkdown         bool
	RenderMicron           bool
	NickColors             bool
	JustifyMsgs            bool
	SpaceMsgs              bool
	ShowGutters            bool
	MentionColor           string
	NickColorsTheme        []string
	EnableEsoterics        bool
}

// NodeConfig holds node settings.
type NodeConfig struct {
	EnableNode             bool
	NodeName               string
	AnnounceInterval       int // seconds
	AnnounceAtStart        bool
	DisablePropagation     bool
	PropagationCost        int
	MaxTransferSize        float64 // KB
	MaxSyncSize            float64 // KB
	PagesPath              string
	PageRefreshInterval    int
	FilesPath              string
	FileRefreshInterval    int
	PrioritiseDestinations []string
	StaticPeers            []string
	MaxPeers               *int
	MessageStorageLimit    float64 // MB
}

// PrintingConfig holds printing settings.
type PrintingConfig struct {
	PrintMessages   bool
	PrintCommand    string
	PrintFrom       string
	MessageTemplate string
}

// DefaultConfig returns a Config with all default values set,
// matching the Python NomadNet defaults.
func DefaultConfig() *Config {
	return &Config{
		Logging: LoggingConfig{
			LogLevel:    4,
			Destination: "file",
		},
		Client: ClientConfig{
			EnableClient:             true,
			UserInterface:            "text",
			DownloadsPath:            "~/Downloads",
			NotifyOnNewMessage:       true,
			AnnounceAtStart:          true,
			AnnounceInterval:         360 * 60, // 360 minutes → seconds
			TryPropagationOnSendFail: true,
			PeriodicLXMFSync:         true,
			LXMFSyncInterval:         360 * 60, // 360 minutes → seconds
			LXMFSyncLimit:            8,
			RequiredStampCost:        nil,
			AcceptInvalidStamps:      false,
			MaxAcceptedSize:          500,
			CompactAnnounceStream:    true,
			ComposeInMarkdown:        true,
		},
		TextUI: TextUIConfig{
			IntroTime:     1,
			Theme:         "dark",
			ColorMode:     "24bit",
			Glyphs:        "nerdfont",
			MouseEnabled:  true,
			Editor:        "nano",
			HideGuide:     false,
			SanitizeNames: true,
			ClipboardCopy: false,
		},
		RRC: RRCConfig{
			HistoryPerRoomCap:      500,
			FilterLoadedHistory:    true,
			EphemeralNotices:       10,
			ColorMentionTimestamps: true,
			RenderMarkdown:         true,
			RenderMicron:           true,
			NickColors:             true,
			JustifyMsgs:            true,
			SpaceMsgs:              false,
			ShowGutters:            true,
		},
		Node: NodeConfig{
			EnableNode:             false,
			AnnounceInterval:       360 * 60, // 360 minutes → seconds
			AnnounceAtStart:        true,
			DisablePropagation:     true,
			PropagationCost:        16,
			MaxTransferSize:        256,
			MaxSyncSize:            10240,
			MessageStorageLimit:    2000,
			PrioritiseDestinations: []string{}, // Python applyConfig defaults to []
			StaticPeers:            []string{}, // Python applyConfig defaults to []
		},
		Printing: PrintingConfig{
			PrintMessages: false,
			PrintCommand:  "lp",
		},
		Raw: make(map[string]map[string]string),
	}
}

// ConfigDir returns the NomadNet config directory by checking
// standard locations in priority order:
// 1. /etc/nomadnetwork
// 2. ~/.config/nomadnetwork
// 3. ~/.nomadnetwork
func ConfigDir() string {
	home, _ := os.UserHomeDir()

	// Check /etc/nomadnetwork
	if _, err := os.Stat("/etc/nomadnetwork/config"); err == nil {
		return "/etc/nomadnetwork"
	}

	// Check ~/.config/nomadnetwork
	configPath := filepath.Join(home, ".config", "nomadnetwork")
	if _, err := os.Stat(filepath.Join(configPath, "config")); err == nil {
		return configPath
	}

	// Fallback to ~/.nomadnetwork
	return filepath.Join(home, ".nomadnetwork")
}

// asBool converts a string value to a boolean, matching Python's
// truthy conventions (yes/true/1 → true, no/false/0 → false).
func asBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "yes" || s == "true" || s == "1"
}

// asInt converts a string value to an integer.
func asInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// asFloat converts a string value to a float64.
func asFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// asList converts a string value to a list by splitting on commas.
func asList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isHexColor reports whether s is a 6-character hexadecimal color string,
// matching Python's `len(value) == 6 and bytes.fromhex(value)` validation.
func isHexColor(s string) bool {
	if len(s) != 6 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Apply parses the raw config values and populates the typed fields.
func (c *Config) Apply() {
	if c.Raw == nil {
		return
	}

	c.applyLogging()
	c.applyClient()
	c.applyTextUI()
	c.applyRRC()
	c.applyNode()
	c.applyPrinting()
}

func (c *Config) applyLogging() {
	sec, ok := c.Raw["logging"]
	if !ok {
		return
	}

	if v, ok := sec["loglevel"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 0 {
				n = 0
			}
			if n > 7 {
				n = 7
			}
			c.Logging.LogLevel = n
		}
	}

	if v, ok := sec["destination"]; ok {
		c.Logging.Destination = strings.ToLower(strings.TrimSpace(v))
	}

	if v, ok := sec["logfile"]; ok {
		c.Logging.LogFile = v
	}
}

func (c *Config) applyClient() {
	sec, ok := c.Raw["client"]
	if !ok {
		return
	}

	if v, ok := sec["enable_client"]; ok {
		c.Client.EnableClient = asBool(v)
	}
	if v, ok := sec["user_interface"]; ok {
		c.Client.UserInterface = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := sec["downloads_path"]; ok {
		c.Client.DownloadsPath = expandUser(strings.TrimSpace(v))
	}
	if v, ok := sec["attachment_save_path"]; ok {
		c.Client.AttachmentSavePath = expandUser(strings.TrimSpace(v))
	}
	if v, ok := sec["notify_on_new_message"]; ok {
		c.Client.NotifyOnNewMessage = asBool(v)
	}
	if v, ok := sec["announce_at_start"]; ok {
		c.Client.AnnounceAtStart = asBool(v)
	}
	if v, ok := sec["announce_interval"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 30 {
				n = 30
			}
			c.Client.AnnounceInterval = n * 60 // minutes → seconds
		}
	}
	if v, ok := sec["try_propagation_on_send_fail"]; ok {
		c.Client.TryPropagationOnSendFail = asBool(v)
	}
	if v, ok := sec["periodic_lxmf_sync"]; ok {
		c.Client.PeriodicLXMFSync = asBool(v)
	}
	if v, ok := sec["lxmf_sync_interval"]; ok {
		if n, err := asInt(v); err == nil {
			// Python: value = minutes*60; only assign when value >= 60 (i.e.
			// minutes >= 1). Below that the configured default is preserved.
			seconds := n * 60
			if seconds >= 60 {
				c.Client.LXMFSyncInterval = seconds
			}
		}
	}
	if v, ok := sec["lxmf_sync_limit"]; ok {
		if n, err := asInt(v); err == nil {
			if n > 0 {
				c.Client.LXMFSyncLimit = n
			} else {
				c.Client.LXMFSyncLimit = 0
			}
		}
	}
	if v, ok := sec["required_stamp_cost"]; ok {
		v = strings.TrimSpace(v)
		if strings.ToLower(v) == "none" {
			c.Client.RequiredStampCost = nil
		} else if n, err := asInt(v); err == nil {
			// Python: clamp to 255; assign only when > 0, else None.
			if n > 255 {
				n = 255
			}
			if n > 0 {
				c.Client.RequiredStampCost = &n
			} else {
				c.Client.RequiredStampCost = nil
			}
		}
	}
	if v, ok := sec["accept_invalid_stamps"]; ok {
		c.Client.AcceptInvalidStamps = asBool(v)
	}
	if v, ok := sec["max_accepted_size"]; ok {
		if f, err := asFloat(v); err == nil {
			if f <= 0 {
				f = 500
			}
			c.Client.MaxAcceptedSize = f
		}
	}
	if v, ok := sec["compact_announce_stream"]; ok {
		c.Client.CompactAnnounceStream = asBool(v)
	}
	if v, ok := sec["compose_in_markdown"]; ok {
		c.Client.ComposeInMarkdown = asBool(v)
	}
}

func (c *Config) applyTextUI() {
	sec, ok := c.Raw["textui"]
	if !ok {
		return
	}

	if v, ok := sec["intro_time"]; ok {
		if f, err := asFloat(v); err == nil {
			c.TextUI.IntroTime = f
		}
	}
	if v, ok := sec["theme"]; ok {
		c.TextUI.Theme = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := sec["colormode"]; ok {
		c.TextUI.ColorMode = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := sec["glyphs"]; ok {
		c.TextUI.Glyphs = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := sec["mouse_enabled"]; ok {
		c.TextUI.MouseEnabled = asBool(v)
	}
	if v, ok := sec["editor"]; ok {
		c.TextUI.Editor = strings.TrimSpace(v)
	}
	if v, ok := sec["hide_guide"]; ok {
		c.TextUI.HideGuide = asBool(v)
	}
	if v, ok := sec["sanitize_names"]; ok {
		c.TextUI.SanitizeNames = asBool(v)
	}
	if v, ok := sec["clipboard_copy"]; ok {
		c.TextUI.ClipboardCopy = asBool(v)
	}
}

func (c *Config) applyRRC() {
	sec, ok := c.Raw["rrc"]
	if !ok {
		return
	}

	if v, ok := sec["history_per_room_cap"]; ok {
		if n, err := asInt(v); err == nil {
			c.RRC.HistoryPerRoomCap = n
		}
	}
	if v, ok := sec["filter_loaded_history"]; ok {
		c.RRC.FilterLoadedHistory = asBool(v)
	}
	if v, ok := sec["ephemeral_notices"]; ok {
		if f, err := asFloat(v); err == nil {
			c.RRC.EphemeralNotices = f
		}
	}
	if v, ok := sec["color_mention_timestamps"]; ok {
		c.RRC.ColorMentionTimestamps = asBool(v)
	}
	if v, ok := sec["render_markdown"]; ok {
		c.RRC.RenderMarkdown = asBool(v)
	}
	if v, ok := sec["render_micron"]; ok {
		c.RRC.RenderMicron = asBool(v)
	}
	if v, ok := sec["nick_colors"]; ok {
		c.RRC.NickColors = asBool(v)
	}
	if v, ok := sec["justify_msgs"]; ok {
		c.RRC.JustifyMsgs = asBool(v)
	}
	if v, ok := sec["space_msgs"]; ok {
		c.RRC.SpaceMsgs = asBool(v)
	}
	if v, ok := sec["show_gutters"]; ok {
		c.RRC.ShowGutters = asBool(v)
	}
	if v, ok := sec["mention_color"]; ok {
		// Python only accepts a 6-character hex string; anything else is None.
		v = strings.TrimSpace(v)
		if isHexColor(v) {
			c.RRC.MentionColor = v
		} else {
			c.RRC.MentionColor = ""
		}
	}
	if v, ok := sec["nick_colors_theme"]; ok {
		// Python keeps only entries that are valid 6-character hex strings.
		theme := asList(v)
		filtered := make([]string, 0, len(theme))
		for _, c := range theme {
			if isHexColor(c) {
				filtered = append(filtered, c)
			}
		}
		c.RRC.NickColorsTheme = filtered
	}
	if v, ok := sec["enable_esoterics"]; ok {
		c.RRC.EnableEsoterics = asBool(v)
	}
}

func (c *Config) applyNode() {
	sec, ok := c.Raw["node"]
	if !ok {
		return
	}

	if v, ok := sec["enable_node"]; ok {
		c.Node.EnableNode = asBool(v)
	}
	if v, ok := sec["node_name"]; ok {
		v = strings.TrimSpace(v)
		if strings.ToLower(v) == "none" || v == "" {
			c.Node.NodeName = ""
		} else {
			c.Node.NodeName = v
		}
	}
	if v, ok := sec["announce_interval"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 1 {
				n = 1
			}
			c.Node.AnnounceInterval = n * 60 // minutes → seconds
		}
	}
	if v, ok := sec["announce_at_start"]; ok {
		c.Node.AnnounceAtStart = asBool(v)
	}
	if v, ok := sec["disable_propagation"]; ok {
		c.Node.DisablePropagation = asBool(v)
	}
	if v, ok := sec["propagation_cost"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 13 {
				n = 13
			}
			c.Node.PropagationCost = n
		}
	}
	if v, ok := sec["max_transfer_size"]; ok {
		if f, err := asFloat(v); err == nil {
			// Python clamps values below 1 up to 1 (not to the default 256).
			if f < 1 {
				f = 1
			}
			c.Node.MaxTransferSize = f
		}
	}
	if v, ok := sec["max_sync_size"]; ok {
		if f, err := asFloat(v); err == nil {
			// Python floors max_sync_size at lxmf_max_propagation_size
			// (the configured max_transfer_size, already clamped to >= 1).
			minSize := c.Node.MaxTransferSize
			if f < minSize {
				f = minSize
			}
			c.Node.MaxSyncSize = f
		}
	}
	if v, ok := sec["pages_path"]; ok {
		c.Node.PagesPath = expandUser(strings.TrimSpace(v))
	}
	if v, ok := sec["page_refresh_interval"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 0 {
				n = 0
			}
			c.Node.PageRefreshInterval = n
		}
	}
	if v, ok := sec["files_path"]; ok {
		c.Node.FilesPath = expandUser(strings.TrimSpace(v))
	}
	if v, ok := sec["file_refresh_interval"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 0 {
				n = 0
			}
			c.Node.FileRefreshInterval = n
		}
	}
	if v, ok := sec["prioritise_destinations"]; ok {
		c.Node.PrioritiseDestinations = asList(v)
	}
	if v, ok := sec["static_peers"]; ok {
		c.Node.StaticPeers = asList(v)
	}
	if v, ok := sec["max_peers"]; ok {
		if n, err := asInt(v); err == nil {
			if n < 0 {
				n = 0
			}
			c.Node.MaxPeers = &n
		}
	}
	if v, ok := sec["message_storage_limit"]; ok {
		if f, err := asFloat(v); err == nil {
			// Python floors values below 0.005 at 0.005 (not at the default 2000).
			if f < 0.005 {
				f = 0.005
			}
			c.Node.MessageStorageLimit = f
		}
	}
}

func (c *Config) applyPrinting() {
	sec, ok := c.Raw["printing"]
	if !ok {
		return
	}

	if v, ok := sec["print_messages"]; ok {
		c.Printing.PrintMessages = asBool(v)
	}
	if v, ok := sec["print_command"]; ok {
		c.Printing.PrintCommand = strings.TrimSpace(v)
	}
	if v, ok := sec["print_from"]; ok {
		c.Printing.PrintFrom = strings.TrimSpace(v)
	}
	if v, ok := sec["message_template"]; ok {
		c.Printing.MessageTemplate = strings.TrimSpace(v)
	}
}

// expandUser expands ~ to the user's home directory.
func expandUser(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
