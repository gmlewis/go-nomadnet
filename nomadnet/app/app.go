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

// Package app implements the central NomadNetworkApp singleton that
// coordinates all subsystems: RNS transport, LXMF messaging, directory,
// RRC chat, node serving, and the terminal UI.
//
// The App struct holds references to all subsystems and provides the
// initialization flow, configuration application, background job
// scheduling, and inter-subsystem callbacks.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/gmlewis/go-nomadnet/nomadnet/storage"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// AnnounceEvent represents a received announce.
type AnnounceEvent struct {
	Timestamp    time.Time
	SourceHash   []byte
	AppData      []byte
	AnnounceType string // "node", "peer", "pn"
	DisplayName  string
}

// UI mode constants matching the Python NomadNet UI modes.
const (
	UINone      = 0
	UIText      = 1
	UIGraphical = 2
	UIWeb       = 3
	UIMenu      = 4
)

// App is the central NomadNetworkApp singleton that holds all state.
type App struct {
	// Core settings
	Version         string
	EnableClient    bool
	EnableNode      bool
	UIMode          int
	ForceConsoleLog bool

	// Paths
	ConfigDir          string
	ConfigPath         string
	IgnoredPath        string
	LogFilePath        string
	ErrorFilePath      string
	StoragePath        string
	IdentityPath       string
	CachePath          string
	ResourcePath       string
	ConversationPath   string
	DirectoryPath      string
	PeerSettingsPath   string
	TmpFilesPath       string
	AttachmentPath     string
	PagesPath          string
	FilesPath          string
	ExamplesPath       string
	DownloadsPath      string
	AttachmentSavePath string

	// Runtime settings
	FirstRun             bool
	ShouldRunJobs        bool
	JobInterval          int // seconds
	DeferJobs            int // seconds
	AnnounceInterval     int // seconds
	PageRefreshInterval  int // minutes
	FileRefreshInterval  int // minutes
	StaticPeers          []string
	PeerAnnounceAtStart  bool
	TryPropagationOnFail bool
	DisablePropagation   bool
	NotifyOnNewMessage   bool
	ComposeMarkdown      bool

	// LXMF settings
	LXMFSyncInterval       int // seconds
	LXMFSyncLimit          int
	CompactStream          bool
	RequiredStampCost      *int
	AcceptInvalidStamps    bool
	LXMFMaxPropagationSize *int
	LXMFMaxSyncSize        *int
	LXMFMaxIncomingSize    *int
	NodePropagationCost    int
	PeriodicLXMFSync       bool

	// Node settings
	NodeName             string
	NodeAnnounceInterval int // minutes
	NodeAnnounceAtStart  bool
	PagePath             string
	FilePath             string

	// RRC settings
	RRCHistoryPerRoomCap      int
	RRCFilterLoadedHistory    bool
	RRCEphemeralNotices       int // seconds
	RRCNickColors             bool
	RRCNickColorsTheme        []string
	RRCMentionColor           string
	RRCColorMentionTimestamps bool
	RRCUIJustifyMsgs          bool
	RRCUISpaceMsgs            bool
	RRCUIRenderMarkdown       bool
	RRCUIRenderMicron         bool
	RRCShowGutters            bool
	RRCEnableEsoterics        bool

	// Printing settings
	PrintMessages                   bool
	PrintCommand                    string
	PrintAllMessages                bool
	PrintTrustedMessages            bool
	AllowedMessagePrintDestinations []string
	PrintingTemplateMsg             string

	// Subsystem references
	Config  *config.Config
	Storage *storage.Paths
	Dir     *directory.Directory
	RRC     *rrc.RRCManager

	// Announce streams (populated by RNS announce handlers)
	Announces []AnnounceEvent

	// RNS/LXMF references
	Logger       *rns.Logger
	Transport    *rns.TransportSystem
	RNS          *rns.Reticulum
	Identity     *rns.Identity
	Router       *lxmf.Router
	LXMFDest     *rns.Destination
	RNSConfigDir string

	// Announce state
	LastAnnounce    time.Time
	LastLXMFSync    time.Time
	LastPageRefresh time.Time
	LastFileRefresh time.Time

	// Callbacks
	DeliveryCallback func(msg any)
	UIChangeCallback func()

	mu sync.Mutex
}

var (
	globalApp *App
	appOnce   sync.Once
	globalMu  sync.Mutex
)

// SharedInstance returns the global App singleton. Returns nil if not
// yet initialized.
func SharedInstance() *App {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalApp
}

// NewApp creates a new App with the given configuration directory.
func NewApp(configDir, rnsConfigDir string, daemon, forceConsole bool) *App {
	a := &App{
		Version:                   "0.1.0",
		ConfigDir:                 configDir,
		RNSConfigDir:              rnsConfigDir,
		EnableClient:              true,
		ForceConsoleLog:           forceConsole,
		ShouldRunJobs:             true,
		JobInterval:               5,
		DeferJobs:                 90,
		AnnounceInterval:          6 * 60 * 60, // 6 hours
		PeerAnnounceAtStart:       true,
		TryPropagationOnFail:      true,
		DisablePropagation:        true,
		NotifyOnNewMessage:        true,
		ComposeMarkdown:           true,
		PeriodicLXMFSync:          true,
		LXMFSyncInterval:          360 * 60, // 6 hours
		LXMFSyncLimit:             8,
		RRCHistoryPerRoomCap:      500,
		RRCFilterLoadedHistory:    true,
		RRCEphemeralNotices:       600,
		RRCNickColors:             true,
		RRCColorMentionTimestamps: true,
		RRCUIJustifyMsgs:          true,
		RRCUIRenderMarkdown:       true,
		RRCUIRenderMicron:         true,
		NodePropagationCost:       16,
		PrintCommand:              "lp",
	}

	// Set up paths
	a.setupPaths()

	return a
}

// Init initializes all subsystems: storage, config, identity, directory,
// RRC, and LXMF router.
func (a *App) Init() error {
	// Ensure storage directories exist
	if err := a.Storage.EnsureDirs(); err != nil {
		return err
	}

	// Load or create config
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return err
	}
	a.Config = cfg
	a.applyConfig(cfg)

	// Initialize logger
	a.Logger = rns.NewLogger()
	a.Logger.SetLogLevel(rns.LogNotice)

	// Configure logging destination — always log to file in TUI mode
	// to prevent RNS logs from destroying the terminal display.
	if a.ForceConsoleLog {
		a.Logger.SetLogDest(rns.LogStdout)
	} else {
		// Ensure the log directory exists
		logDir := filepath.Dir(a.LogFilePath)
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return fmt.Errorf("creating log directory: %w", err)
		}
		// Create/touch the log file so the logger can open it
		f, err := os.OpenFile(a.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("creating log file: %w", err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("closing log file: %w", err)
		}

		a.Logger.SetLogDest(rns.LogDestFile)
		a.Logger.SetLogFilePath(a.LogFilePath)
	}

	a.Logger.Info("Nomad Network Client %s starting...", a.Version)

	// Initialize RNS transport and stack
	a.Logger.Info("Substantiating Reticulum...")
	a.Transport = rns.NewTransportSystem(a.Logger)
	rnsConfigDir := a.RNSConfigDir
	if rnsConfigDir == "" {
		// Create a standalone RNS config with share_instance = No
		// so each gonomadnet instance runs independently
		rnsConfigDir = a.ensureStandaloneRNSConfig()
	}
	ret, err := rns.NewReticulumWithLogger(a.Transport, rnsConfigDir, a.Logger)
	if err != nil {
		return fmt.Errorf("could not initialize Reticulum: %w", err)
	}
	a.RNS = ret

	// Load or create identity
	a.Identity, err = a.loadOrCreateIdentity(a.IdentityPath)
	if err != nil {
		return fmt.Errorf("could not load identity: %w", err)
	}
	a.Logger.Info("Identity loaded: %s", rns.PrettyHex(a.Identity.Hash))

	// Initialize subsystems
	a.Dir = directory.New()
	a.RRC = rrc.NewManager(a.StoragePath, nil)

	// Initialize LXMF router
	a.Logger.Info("Initializing LXMF router...")
	a.Router, err = lxmf.NewRouter(a.Transport, a.Identity, a.StoragePath)
	if err != nil {
		return fmt.Errorf("could not create LXMF router: %w", err)
	}

	// Register delivery callback
	a.Router.RegisterDeliveryCallback(a.lxmfDelivery)

	// Register delivery identity for receiving messages
	a.LXMFDest, err = a.Router.RegisterDeliveryIdentity(a.Identity, a.Config.Client.UserInterface, nil)
	if err != nil {
		return fmt.Errorf("could not register delivery identity: %w", err)
	}
	a.Logger.Info("LXMF Router ready to receive on %s", rns.PrettyHex(a.LXMFDest.Hash))

	// Register announce handlers with RNS transport
	a.Transport.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter:                "lxmf.delivery",
		ReceivedAnnounceWithContext: a.handleLXMFAnnounce,
	})
	a.Transport.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter:                "nomadnetwork.node",
		ReceivedAnnounceWithContext: a.handleNodeAnnounce,
	})
	a.Transport.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter:                "lxmf.propagation",
		ReceivedAnnounceWithContext: a.handlePNAnnounce,
	})
	a.Logger.Info("Announce handlers registered")

	// Set global singleton
	globalMu.Lock()
	globalApp = a
	globalMu.Unlock()
	appOnce.Do(func() {})

	// Start background jobs
	go a.jobs()

	return nil
}

// loadOrCreateIdentity loads an existing identity or creates a new one.
func (a *App) loadOrCreateIdentity(path string) (*rns.Identity, error) {
	if _, err := os.Stat(path); err == nil {
		id, err := rns.FromFile(path, a.Logger)
		if err != nil {
			return nil, fmt.Errorf("loading identity: %w", err)
		}
		a.Logger.Info("Loaded identity from %s", path)
		return id, nil
	}

	a.Logger.Info("No identity found, creating new...")
	id, err := rns.NewIdentity(true, a.Logger)
	if err != nil {
		return nil, fmt.Errorf("creating identity: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating identity directory: %w", err)
	}
	if err := id.ToFile(path); err != nil {
		return nil, fmt.Errorf("saving identity: %w", err)
	}
	a.Logger.Info("Created new identity at %s", path)
	return id, nil
}

// lxmfDelivery handles incoming LXMF messages.
func (a *App) lxmfDelivery(msg *lxmf.Message) {
	a.Logger.Info("Received LXMF message from %s", rns.PrettyHex(msg.SourceHash))
	if a.DeliveryCallback != nil {
		a.DeliveryCallback(msg)
	}
}

// handleLXMFAnnounce processes LXMF delivery announces.
func (a *App) handleLXMFAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	displayName, _ := lxmf.DisplayNameFromAppData(appData)
	a.Logger.Info("LXMF announce received: hash=%x name=%q", destHash, displayName)

	a.mu.Lock()
	a.Announces = append(a.Announces, AnnounceEvent{
		Timestamp:    time.Now(),
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "peer",
		DisplayName:  displayName,
	})
	a.mu.Unlock()

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}

// handleNodeAnnounce processes NomadNet node announces.
func (a *App) handleNodeAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	displayName := string(appData)
	a.Logger.Info("Node announce received: hash=%x name=%q", destHash, displayName)

	a.mu.Lock()
	a.Announces = append(a.Announces, AnnounceEvent{
		Timestamp:    time.Now(),
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "node",
		DisplayName:  displayName,
	})
	a.mu.Unlock()

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}

// handlePNAnnounce processes propagation node announces.
func (a *App) handlePNAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	displayName, _ := lxmf.DisplayNameFromAppData(appData)

	a.mu.Lock()
	a.Announces = append(a.Announces, AnnounceEvent{
		Timestamp:    time.Now(),
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "pn",
		DisplayName:  displayName,
	})
	a.mu.Unlock()
}

// setupPaths initializes all file system paths based on ConfigDir.
func (a *App) setupPaths() {
	a.ConfigPath = filepath.Join(a.ConfigDir, "config")
	a.IgnoredPath = filepath.Join(a.ConfigDir, "ignored")
	a.LogFilePath = filepath.Join(a.ConfigDir, "logfile")
	a.ErrorFilePath = filepath.Join(a.ConfigDir, "errors")
	a.StoragePath = filepath.Join(a.ConfigDir, "storage")
	a.IdentityPath = filepath.Join(a.StoragePath, "identity")
	a.CachePath = filepath.Join(a.StoragePath, "cache")
	a.ResourcePath = filepath.Join(a.StoragePath, "resources")
	a.ConversationPath = filepath.Join(a.StoragePath, "conversations")
	a.DirectoryPath = filepath.Join(a.StoragePath, "directory")
	a.PeerSettingsPath = filepath.Join(a.StoragePath, "peersettings")
	a.TmpFilesPath = filepath.Join(a.StoragePath, "tmp")
	a.AttachmentPath = filepath.Join(a.StoragePath, "attachments")
	a.PagesPath = filepath.Join(a.StoragePath, "pages")
	a.FilesPath = filepath.Join(a.StoragePath, "files")
	a.ExamplesPath = filepath.Join(a.ConfigDir, "examples")
	a.DownloadsPath = expandUser("~/Downloads")

	a.Storage = storage.New(a.ConfigDir)
}

// applyConfig applies a loaded config to the App fields.
func (a *App) applyConfig(cfg *config.Config) {
	a.EnableClient = cfg.Client.EnableClient
	a.AnnounceInterval = cfg.Client.AnnounceInterval
	a.TryPropagationOnFail = cfg.Client.TryPropagationOnSendFail
	a.PeriodicLXMFSync = cfg.Client.PeriodicLXMFSync
	a.LXMFSyncInterval = cfg.Client.LXMFSyncInterval
	a.LXMFSyncLimit = cfg.Client.LXMFSyncLimit
	a.RequiredStampCost = cfg.Client.RequiredStampCost
	a.AcceptInvalidStamps = cfg.Client.AcceptInvalidStamps
	a.CompactStream = cfg.Client.CompactAnnounceStream
	a.NotifyOnNewMessage = cfg.Client.NotifyOnNewMessage
	a.ComposeMarkdown = cfg.Client.ComposeInMarkdown
	a.DownloadsPath = expandUser(cfg.Client.DownloadsPath)
	a.DisablePropagation = true

	a.PageRefreshInterval = 0
	a.FileRefreshInterval = 0

	a.RRCHistoryPerRoomCap = cfg.RRC.HistoryPerRoomCap
	a.RRCFilterLoadedHistory = cfg.RRC.FilterLoadedHistory
	a.RRCEphemeralNotices = int(cfg.RRC.EphemeralNotices)
	a.RRCNickColors = cfg.RRC.NickColors
	a.RRCMentionColor = cfg.RRC.MentionColor
	a.RRCColorMentionTimestamps = cfg.RRC.ColorMentionTimestamps
	a.RRCUIJustifyMsgs = cfg.RRC.JustifyMsgs
	a.RRCUISpaceMsgs = cfg.RRC.SpaceMsgs
	a.RRCUIRenderMarkdown = cfg.RRC.RenderMarkdown
	a.RRCUIRenderMicron = cfg.RRC.RenderMicron
	a.RRCShowGutters = cfg.RRC.ShowGutters
	a.RRCEnableEsoterics = cfg.RRC.EnableEsoterics

	a.EnableNode = cfg.Node.EnableNode
	a.NodeName = cfg.Node.NodeName
	a.NodeAnnounceInterval = cfg.Node.AnnounceInterval / 60
	a.NodeAnnounceAtStart = cfg.Node.AnnounceAtStart
	a.NodePropagationCost = cfg.Node.PropagationCost
	if cfg.Node.PagesPath != "" {
		a.PagesPath = cfg.Node.PagesPath
	}
	if cfg.Node.FilesPath != "" {
		a.FilesPath = cfg.Node.FilesPath
	}

	a.PrintMessages = cfg.Printing.PrintMessages
	a.PrintCommand = cfg.Printing.PrintCommand
}

// Shutdown stops background jobs and persists state.
func (a *App) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ShouldRunJobs = false

	if a.RRC != nil {
		a.RRC.Shutdown()
	}

	if a.RNS != nil {
		if err := a.RNS.Close(); err != nil {
			a.Logger.Warning("Could not close Reticulum: %v", err)
		}
	}
}

// RequestLXMFSync initiates an LXMF sync with propagation nodes.
func (a *App) RequestLXMFSync(limit int) {
	a.mu.Lock()
	a.LastLXMFSync = time.Now()
	a.mu.Unlock()
}

// AnnounceNow sends an LXMF announce and records the timestamp.
func (a *App) AnnounceNow() {
	a.mu.Lock()
	a.LastAnnounce = time.Now()
	a.mu.Unlock()
}

// AutoSelectPropagationNode selects the best propagation node from
// known trusted nodes with fewest hops.
func (a *App) AutoSelectPropagationNode() {
	if a.Dir == nil {
		return
	}
	// Selection logic will be implemented when RNS transport is available
}

// IsIgnored checks if a source hash is in the ignored list.
func (a *App) IsIgnored(sourceHash []byte) bool {
	return false // placeholder
}

// GetAnnounces returns a copy of the announce stream.
func (a *App) GetAnnounces() []AnnounceEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]AnnounceEvent, len(a.Announces))
	copy(result, a.Announces)
	return result
}

// AnnounceCount returns the number of announces received.
func (a *App) AnnounceCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.Announces)
}

// SetUIChangeCallback sets a callback for UI refresh.
func (a *App) SetUIChangeCallback(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.UIChangeCallback = fn
}

// ConversationList returns the list of all conversations.
func (a *App) ConversationList() []conversation.ConversationInfo {
	if a.ConversationPath == "" {
		return nil
	}
	return conversation.ConversationList(a.ConversationPath, nil, nil)
}

func expandUser(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// ensureStandaloneRNSConfig creates a standalone RNS config directory
// with share_instance = No, matching gornphone's pattern. Each gonomadnet
// instance runs its own standalone RNS stack so destinations are registered
// on its own TransportSystem.
func (a *App) ensureStandaloneRNSConfig() string {
	rnsDir := filepath.Join(os.TempDir(), fmt.Sprintf("gonomadnet-rns-%d", time.Now().UnixMilli()))
	configPath := filepath.Join(rnsDir, "config")

	_ = os.MkdirAll(rnsDir, 0o755)

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	var content string
	if home != "" {
		systemConfigPath := filepath.Join(home, ".reticulum", "config")
		if data, err := os.ReadFile(systemConfigPath); err == nil {
			content = string(data)
		}
	}

	if content == "" {
		content = `[reticulum]
  share_instance = No

[logging]
  loglevel = 4

[interfaces]
  [[Default Interface]]
    type = AutoInterface
    enabled = Yes
    name = Default Interface
`
	}

	// Override share_instance to No for standalone operation
	content = setRNSConfigDirective(content, "share_instance", "No")

	_ = os.WriteFile(configPath, []byte(content), 0o644)
	a.Logger.Info("Created standalone RNS config at %s", rnsDir)
	return rnsDir
}

// setRNSConfigDirective replaces or adds a key=value directive in the
// [reticulum] section of an RNS config string.
func setRNSConfigDirective(content, key, value string) string {
	lines := strings.Split(content, "\n")
	inReticulum := false
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[[") || strings.HasPrefix(trimmed, "[[") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section := strings.Trim(trimmed, "[] ")
			inReticulum = section == "reticulum"
			continue
		}
		if inReticulum {
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
				continue
			}
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
				lines[i] = fmt.Sprintf("  %s = %s", key, value)
				replaced = true
				break
			}
		}
	}
	if !replaced {
		// Add to end of [reticulum] section
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[reticulum]") {
				lines = append(lines[:i+1], append([]string{fmt.Sprintf("  %s = %s", key, value)}, lines[i+1:]...)...)
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}
