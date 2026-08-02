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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-nomadnet/nomadnet/conversation"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-nomadnet/nomadnet/node"
	"github.com/gmlewis/go-nomadnet/nomadnet/peersettings"
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/gmlewis/go-nomadnet/nomadnet/storage"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// AnnounceEvent represents a received announce.
type AnnounceEvent struct {
	Timestamp    time.Time
	TimestampF   float64 // same instant as Timestamp, as the float64 seconds the directory stores
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

// startAnnounceDelay mirrors Python's NomadNetworkApp.START_ANNOUNCE_DELAY = 3
// (NomadNetworkApp.py:36): when peer_announce_at_start is set, a daemon thread
// sleeps this long after RNS init before sending the first announce.
const startAnnounceDelay = 3 * time.Second

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

	// Local peer state
	PeerSettings *peersettings.Settings
	IgnoredList  [][]byte

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
	MaxPeers             *int
	MessageStorageLimit  float64
	PrioritisedLXMF      []string

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

	// ConversationCache is the per-app in-memory conversation cache (cached
	// conversations, unread/failed flags, and attachment base path). Owned
	// here rather than at package level so parallel tests get isolated state.
	ConversationCache *conversation.ConversationCache

	// Announce streams (populated by RNS announce handlers)
	Announces []AnnounceEvent

	// RNS/LXMF references
	Logger           *rns.Logger
	Transport        *rns.TransportSystem
	RNS              *rns.Reticulum
	Identity         *rns.Identity
	Router           *lxmf.Router
	LXMFDest         *rns.Destination
	RNSConfigDir     string
	standaloneRNSDir string

	// Node is the hosted Nomad Network node (nomadnet/node.Node), started when
	// EnableNode is true. Mirrors Python NomadNetworkApp.py:399
	// self.node = nomadnet.Node(self). It is nil when node hosting is disabled.
	Node *node.Node

	// Announce state
	LastAnnounce    time.Time
	LastLXMFSync    time.Time
	LastPageRefresh time.Time
	LastFileRefresh time.Time

	notifyWriter io.Writer
	// Callbacks
	DeliveryCallback func(msg any)
	UIChangeCallback func()

	mu     sync.Mutex
	initWG sync.WaitGroup
}

// AppOption configures an App during construction.
type AppOption func(*App)

// WithTransport injects an external TransportSystem, skipping the
// automatic transport creation in initRNS.
func WithTransport(ts *rns.TransportSystem) AppOption {
	return func(a *App) { a.Transport = ts }
}

// WithIdentity injects an external Identity, skipping automatic
// identity creation in initRNS.
func WithIdentity(id *rns.Identity) AppOption {
	return func(a *App) { a.Identity = id }
}

// WithLogger injects an external Logger.
func WithLogger(logger *rns.Logger) AppOption {
	return func(a *App) { a.Logger = logger }
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

	a.ConversationCache = conversation.NewConversationCache()

	return a
}

// NewAppWithTransport creates a new App with optional external
// dependencies injected via AppOption. When WithTransport and
// WithIdentity are provided, the App skips creating its own
// transport and identity during initialization.
func NewAppWithTransport(configDir string, opts ...AppOption) *App {
	a := NewApp(configDir, "", false, false)
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Init initializes subsystems that don't block, then starts RNS
// initialization in a goroutine so the TUI can start immediately.
func (a *App) Init() error {
	// Ensure storage directories exist
	if err := a.Storage.EnsureDirs(); err != nil {
		return err
	}

	// Load or create config. On first run (config file missing) write the
	// default NomadNet config to disk — mirroring Python's createDefaultConfig —
	// then load and apply it.
	if _, err := os.Stat(a.ConfigPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("checking config file: %w", err)
		}
		if err := a.CreateDefaultConfig(); err != nil {
			return fmt.Errorf("creating default config: %w", err)
		}
	} else {
		cfg, err := config.Load(a.ConfigPath)
		if err != nil {
			return err
		}
		a.Config = cfg
		a.applyConfig(cfg)
	}

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

	// Initialize non-blocking subsystems
	a.Dir = directory.New()
	a.Dir.SanitizeNames = a.Config.TextUI.SanitizeNames
	// Restore the persisted directory (Python Directory.__init__ →
	// load_from_disk, Directory.py:82). A missing file is not an error.
	if err := a.Dir.LoadFromDisk(a.DirectoryPath); err != nil {
		a.Logger.Error("Could not load directory from %s: %v", a.DirectoryPath, err)
	}
	a.RRC = rrc.NewManager(a.StoragePath, nil)
	a.loadPeerSettings()
	a.loadIgnoredList()
	a.ConversationCache.SetAttachmentPath(a.AttachmentPath)

	// Start RNS initialization in a goroutine so the TUI can start immediately.
	// RNS initialization may block on network interfaces.
	a.initWG.Add(1)
	go func() {
		defer a.initWG.Done()
		a.initRNS()
	}()

	return nil
}

// initRNS initializes RNS transport, LXMF router, and background jobs.
// This runs in a goroutine to avoid blocking the TUI startup.
func (a *App) initRNS() {
	a.Logger.Info("Initializing RNS transport...")

	// Publish the transport under a.mu so readers in other goroutines (e.g.
	// InterfaceStats, polled from a UI ticker) see the pointer safely; the
	// pointer is assigned once here and never reassigned.
	transport := rns.NewTransportSystem(a.Logger)
	a.mu.Lock()
	a.Transport = transport
	a.mu.Unlock()
	a.Dir.SetTransport(transport)
	rnsConfigDir := a.RNSConfigDir
	if rnsConfigDir == "" {
		rnsConfigDir = a.ensureStandaloneRNSConfig()
	}

	ret, err := rns.NewReticulumWithLogger(a.Transport, rnsConfigDir, a.Logger)
	if err != nil {
		a.Logger.Error("Could not initialize Reticulum: %v", err)
		return
	}
	a.RNS = ret

	// Load or create identity
	var err2 error
	a.Identity, err2 = a.loadOrCreateIdentity(a.IdentityPath)
	if err2 != nil {
		a.Logger.Error("Could not load identity: %v", err2)
		return
	}
	a.Logger.Info("Identity loaded: %s", rns.PrettyHex(a.Identity.Hash))
	a.RRC.SetIdentity(a.Identity)
	a.RRC.SetHistoryConfig(a.RRCHistoryPerRoomCap, a.RRCFilterLoadedHistory, a.RRCEphemeralNotices)

	// Initialize LXMF router
	a.Logger.Info("Initializing LXMF router...")
	a.Router, err = lxmf.NewRouter(a.Transport, a.Identity, a.StoragePath)
	if err != nil {
		a.Logger.Error("Could not create LXMF router: %v", err)
		return
	}

	// Register delivery callback
	a.Router.RegisterDeliveryCallback(a.lxmfDelivery)

	// Register delivery identity for receiving messages
	a.LXMFDest, err = a.Router.RegisterDeliveryIdentity(a.Identity, a.Config.Client.UserInterface, nil)
	if err != nil {
		a.Logger.Error("Could not register delivery identity: %v", err)
		return
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

	// Start the hosted node when enable_node is set (Python
	// NomadNetworkApp.py:399 self.node = nomadnet.Node(self)). A start failure is
	// logged but does not abort the client, matching Python's daemon resilience.
	if err := a.startNode(); err != nil {
		a.Logger.Error("Could not start node: %v", err)
	}

	// RNS is now ready (identity + LXMF destination loaded): notify the UI so
	// widgets that need identity data (e.g. the Local Peer Info panel) can
	// populate now, even though initRNS ran asynchronously after Init returned.
	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}

	// Announce at start (Python peer_announce_at_start, NomadNetworkApp.py:415-
	// 421): a daemon thread sleeps START_ANNOUNCE_DELAY then sends the first
	// announce so the Local Peer Info "Announced : …" line reads "just now"
	// shortly after boot instead of "Never".
	if a.PeerAnnounceAtStart {
		go func() {
			time.Sleep(startAnnounceDelay)
			a.AnnounceNow()
			if a.UIChangeCallback != nil {
				a.UIChangeCallback()
			}
		}()
	}

	// Start background jobs
	go a.jobs()
}

// InitWithTransport initializes the App synchronously using the provided
// transport and identity, bypassing the automatic initRNS flow. This is
// used for integration testing where external RNS instances are injected.
func (a *App) InitWithTransport(ts *rns.TransportSystem, identity *rns.Identity) error {
	if err := a.Storage.EnsureDirs(); err != nil {
		return err
	}

	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return err
	}
	a.Config = cfg
	a.applyConfig(cfg)

	if a.Logger == nil {
		a.Logger = rns.NewLogger()
		a.Logger.SetLogLevel(rns.LogNotice)
		a.Logger.SetLogDest(rns.LogStdout)
	}

	a.Transport = ts
	a.Identity = identity
	a.Dir = directory.New()
	a.Dir.SanitizeNames = a.Config.TextUI.SanitizeNames
	a.Dir.SetTransport(ts)
	// Restore the persisted directory (Python Directory.__init__ →
	// load_from_disk, Directory.py:82) so known peers, trust levels, and the
	// announce stream survive a restart. A missing file is not an error.
	if err := a.Dir.LoadFromDisk(a.DirectoryPath); err != nil {
		a.Logger.Error("Could not load directory from %s: %v", a.DirectoryPath, err)
	}
	a.RRC = rrc.NewManager(a.StoragePath, nil)
	a.RRC.SetIdentity(a.Identity)
	a.RRC.SetHistoryConfig(a.RRCHistoryPerRoomCap, a.RRCFilterLoadedHistory, a.RRCEphemeralNotices)
	a.loadPeerSettings()
	a.loadIgnoredList()
	a.ConversationCache.SetAttachmentPath(a.AttachmentPath)

	a.Router, err = lxmf.NewRouter(a.Transport, a.Identity, a.StoragePath)
	if err != nil {
		return fmt.Errorf("creating LXMF router: %w", err)
	}
	a.Router.RegisterDeliveryCallback(a.lxmfDelivery)

	a.LXMFDest, err = a.Router.RegisterDeliveryIdentity(a.Identity, a.Config.Client.UserInterface, nil)
	if err != nil {
		return fmt.Errorf("registering delivery identity: %w", err)
	}

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
	a.Transport.RegisterAnnounceHandler(&rns.AnnounceHandler{
		AspectFilter:                "rrc.chat",
		ReceivedAnnounceWithContext: a.handleRRCAnnounce,
	})

	// Start the hosted node when enable_node is set (mirrors initRNS).
	if err := a.startNode(); err != nil {
		return fmt.Errorf("starting node: %w", err)
	}

	return nil
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
	a.AttachmentSavePath = expandUser(cfg.Client.AttachmentSavePath)
	a.PeerAnnounceAtStart = cfg.Client.AnnounceAtStart
	a.applyUIMode(cfg.Client.UserInterface)
	// Python stores these as floats (as_float); the App fields are *int KB
	// values. The config package has already applied defaults and clamping.
	a.LXMFMaxIncomingSize = intPtr(int(cfg.Client.MaxAcceptedSize))
	a.LXMFMaxPropagationSize = intPtr(int(cfg.Node.MaxTransferSize))
	a.LXMFMaxSyncSize = intPtr(int(cfg.Node.MaxSyncSize))
	a.DisablePropagation = cfg.Node.DisablePropagation

	a.PageRefreshInterval = cfg.Node.PageRefreshInterval
	a.FileRefreshInterval = cfg.Node.FileRefreshInterval
	a.StaticPeers = cfg.Node.StaticPeers
	a.MaxPeers = cfg.Node.MaxPeers
	a.MessageStorageLimit = cfg.Node.MessageStorageLimit
	a.PrioritisedLXMF = cfg.Node.PrioritiseDestinations

	a.RRCHistoryPerRoomCap = cfg.RRC.HistoryPerRoomCap
	a.RRCFilterLoadedHistory = cfg.RRC.FilterLoadedHistory
	// Python stores rrc_ephemeral_notices in seconds (value*60); the Go config
	// holds minutes, so convert to seconds here to match Python.
	a.RRCEphemeralNotices = int(cfg.RRC.EphemeralNotices) * 60
	a.RRCNickColors = cfg.RRC.NickColors
	a.RRCNickColorsTheme = cfg.RRC.NickColorsTheme
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
	a.applyPrintingFrom(cfg)
}

// printFromHashHexLen is the hex length of a destination hash
// (RNS.Identity.TRUNCATED_HASHLENGTH/8*2 = 128/8*2 = 32), matching Python's
// `len(...) == (RNS.Identity.TRUNCATED_HASHLENGTH//8)*2` check in applyConfig.
const printFromHashHexLen = 32

// applyPrintingFrom parses the printing.print_from setting when print_messages
// is enabled, matching Python NomadNetworkApp.applyConfig
// (NomadNetworkApp.py:1183-1218). A comma-separated value is treated as a
// list; a single value of "everywhere", "trusted", or a 32-hex destination
// hash selects the corresponding print scope.
func (a *App) applyPrintingFrom(cfg *config.Config) {
	a.PrintAllMessages = false
	a.PrintTrustedMessages = false
	a.AllowedMessagePrintDestinations = nil
	a.PrintingTemplateMsg = ""

	if !cfg.Printing.PrintMessages {
		return
	}

	pf := cfg.Printing.PrintFrom
	if pf == "" {
		// Python: allowed_message_print_destinations = None
	} else if strings.Contains(pf, ",") {
		entries := splitCSV(pf)
		a.AllowedMessagePrintDestinations = entries
		for _, e := range entries {
			if strings.ToLower(e) == "trusted" {
				a.PrintTrustedMessages = true
			}
		}
	} else {
		switch lower := strings.ToLower(pf); {
		case lower == "everywhere":
			a.PrintAllMessages = true
		case lower == "trusted":
			a.PrintTrustedMessages = true
		case len(pf) == printFromHashHexLen:
			a.AllowedMessagePrintDestinations = []string{pf}
		}
	}

	if cfg.Printing.MessageTemplate == "" {
		a.PrintingTemplateMsg = defaultPrintingTemplate
	} else {
		path := expandUser(cfg.Printing.MessageTemplate)
		if data, err := os.ReadFile(path); err == nil {
			a.PrintingTemplateMsg = string(data)
		} else {
			// Python falls back to the default template when the file is
			// missing or unreadable (and would create it with the default).
			a.PrintingTemplateMsg = defaultPrintingTemplate
		}
	}
}

// splitCSV splits a comma-separated string into trimmed, non-empty fields,
// matching the config package's as_list convention for list-valued keys.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applyUIMode maps the user_interface config string to an App.UIMode constant,
// matching Python applyConfig's uimode selection (NomadNetworkApp.py:904-982).
// The config value is already lowercased by config.Load.
func (a *App) applyUIMode(ui string) {
	switch ui {
	case "none":
		a.UIMode = UINone
	case "menu":
		a.UIMode = UIMenu
	case "text":
		a.UIMode = UIText
	case "graphical":
		a.UIMode = UIGraphical
	case "web":
		a.UIMode = UIWeb
	}
}

// Shutdown stops background jobs and persists state. It waits for any
// in-progress asynchronous RNS initialization (started by Init) to complete
// before tearing down subsystems, avoiding races on the RNS/Router fields.
func (a *App) Shutdown() {
	a.initWG.Wait()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.ShouldRunJobs = false

	// Persist the in-memory directory (known peers, trust levels, announce
	// stream) to disk before tearing anything down — Python's exit_handler does
	// self.directory.save_to_disk() first (NomadNetworkApp.py:42). Without this
	// the directory is ephemeral and every known peer is lost on restart.
	if a.Dir != nil {
		if err := a.Dir.SaveToDisk(a.DirectoryPath); err != nil {
			a.Logger.Error("Could not save directory to %s: %v", a.DirectoryPath, err)
		}
	}

	// Stop the hosted node's background job loop (Python exit_handler sets
	// should_run_jobs false, which halts Node.__jobs).
	a.stopNode()

	if a.RRC != nil {
		a.RRC.Shutdown()
	}

	if a.RNS != nil {
		if err := a.RNS.Close(); err != nil {
			a.Logger.Warning("Could not close Reticulum: %v", err)
		}
	}

	// Remove the per-run standalone RNS config dir created by
	// ensureStandaloneRNSConfig so temp dirs don't accumulate across runs.
	// Done after RNS.Close(); the retry handles the brief window where RNS
	// background goroutines finish flushing ratchet/destination storage and
	// recreate files mid-removal (ENOTEMPTY/EBUSY). This mirrors the
	// removeAllWithRetry pattern used in go-reticulum's testutils.
	if a.standaloneRNSDir != "" {
		if err := removeAllWithRetry(a.standaloneRNSDir); err != nil {
			a.Logger.Warning("Could not remove standalone RNS config dir %s: %v", a.standaloneRNSDir, err)
		}
		a.standaloneRNSDir = ""
	}
}

// AnnounceNow sends an LXMF delivery announce using the configured display
// name and stamp cost, then records the announce time and persists peer
// settings. This mirrors the Python NomadNetworkApp.announce_now.
func (a *App) AnnounceNow() {
	if a.Router != nil && a.LXMFDest != nil {
		a.Router.SetInboundStampCost(a.LXMFDest.Hash, a.RequiredStampCost)
		a.Router.SetDisplayName(a.LXMFDest.Hash, a.GetDisplayName())
		_ = a.Router.Announce(a.LXMFDest.Hash)
	}
	a.mu.Lock()
	a.LastAnnounce = time.Now()
	a.mu.Unlock()
	a.SavePeerSettings()
}

// AutoSelectPropagationNode selects a default LXMF propagation node.
// When the user has manually selected a node it is used; otherwise the
// trusted known node with the fewest hops is chosen. The selection is
// pushed to the LXMF router. This mirrors the Python NomadNet
// autoselect_propagation_node.
func (a *App) AutoSelectPropagationNode() {
	var selected []byte
	if ps := a.PeerSettings; ps != nil {
		if h, ok := ps.PropagationNode.([]byte); ok && len(h) > 0 {
			selected = h
		}
	}
	if selected == nil && a.Dir != nil {
		bestHops := rns.PathfinderM + 1
		for _, node := range a.Dir.KnownNodes() {
			if node.TrustLevel != directory.TrustTrusted {
				continue
			}
			hops := rns.PathfinderM + 1
			if a.Transport != nil {
				hops = a.Transport.HopsTo(node.SourceHash)
			}
			if hops < bestHops {
				bestHops = hops
				selected = node.SourceHash
			}
		}
	}
	if selected == nil {
		if a.Logger != nil {
			a.Logger.Notice("Could not autoselect a propagation node")
		}
		return
	}
	if a.Router != nil {
		_ = a.Router.SetOutboundPropagationNode(selected)
	}
}

// GetAnnounces returns a copy of the announce stream.
func (a *App) GetAnnounces() []AnnounceEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]AnnounceEvent, len(a.Announces))
	copy(result, a.Announces)
	return result
}

// prependAnnounce inserts ev at the front of the announce stream, mirroring
// Python's Directory per-type list.insert(0, ...) (Directory.py:171,195,236):
// each received announce is newest-first. The Network panel's AnnounceStream
// filters by tab, so prepending (rather than appending) makes the displayed
// list descending by time, matching the original. Callers must NOT hold a.mu.
func (a *App) prependAnnounce(ev AnnounceEvent) {
	a.mu.Lock()
	a.Announces = append([]AnnounceEvent{ev}, a.Announces...)
	a.mu.Unlock()
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

// intPtr returns a pointer to n, for config fields stored as *int.
func intPtr(n int) *int { return &n }

// ensureStandaloneRNSConfig creates a standalone RNS config directory
// with share_instance = No, matching gornphone's pattern. Each gonomadnet
// instance runs its own standalone RNS stack so destinations are registered
// on its own TransportSystem.
// tempBaseDir returns the base directory for standalone RNS config temp dirs.
// On macOS (darwin) it returns "/tmp" — matching the go-reticulum repo's
// testutils.tempBaseDir() — instead of os.TempDir() ($TMPDIR, a per-user
// /var/folders path) so the dirs live in the conventional, easily inspected
// /tmp location. On every other platform it returns "" so os.MkdirTemp uses
// its default (os.TempDir()).
func tempBaseDir() string {
	if runtime.GOOS == "darwin" {
		return "/tmp"
	}
	return ""
}

// removeAllWithRetry removes path with os.RemoveAll, retrying on transient
// ENOTEMPTY/EBUSY errors for up to ~100ms. After RNS.Close() a handful of
// background goroutines may still be flushing ratchet/destination storage and
// briefly recreate files mid-removal; the retry lets them quiesce. Mirrors the
// helper of the same name in go-reticulum's testutils package.
func removeAllWithRetry(path string) error {
	const maxAttempts = 10
	const retryDelay = 10 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := os.RemoveAll(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		if !isRetriableRemoveAllError(err) {
			return err
		}
		time.Sleep(retryDelay)
	}

	return os.RemoveAll(path)
}

func isRetriableRemoveAllError(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EBUSY)
}

func (a *App) ensureStandaloneRNSConfig() string {
	rnsDir, err := os.MkdirTemp(tempBaseDir(), "gonomadnet-rns-")
	if err != nil {
		a.Logger.Error("Could not create standalone RNS config dir: %v", err)
		return ""
	}
	a.standaloneRNSDir = rnsDir
	configPath := filepath.Join(rnsDir, "config")

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
		if strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]") {
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

	if a.ConversationPath != "" {
		if _, err := a.ConversationCache.Ingest(msg, a.ConversationPath, false); err != nil {
			a.Logger.Error("Failed to ingest LXMF message: %v", err)
		}
	}

	if a.NotifyOnNewMessage {
		a.NotifyMessageReceived()
	}

	if a.ShouldPrint(msg) {
		a.PrintMessage(msg, time.Now())
	}

	if a.DeliveryCallback != nil {
		a.DeliveryCallback(msg)
	}

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}

// handleLXMFAnnounce processes LXMF delivery announces.
func (a *App) handleLXMFAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	displayName, _ := lxmf.DisplayNameFromAppData(appData)
	a.Logger.Info("LXMF announce received: hash=%x name=%q", destHash, displayName)

	now := time.Now()
	nowF := float64(now.UnixNano()) / 1e9
	a.prependAnnounce(AnnounceEvent{
		Timestamp:    now,
		TimestampF:   nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "peer",
		DisplayName:  displayName,
	})

	a.Dir.PeerAnnounceReceived(directory.Announce{
		Timestamp:    nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "peer",
	}, true)

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}

// handleNodeAnnounce processes NomadNet node announces.
// Matches Python's Directory.received_announce (Directory.py:41) and
// Directory.node_announce_received (Directory.py:178).
func (a *App) handleNodeAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	displayName := string(appData)
	a.Logger.Info("Node announce received: hash=%x name=%q", destHash, displayName)

	now := time.Now()
	nowF := float64(now.UnixNano()) / 1e9
	a.prependAnnounce(AnnounceEvent{
		Timestamp:    now,
		TimestampF:   nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "node",
		DisplayName:  displayName,
	})

	a.Dir.NodeAnnounceReceived(directory.Announce{
		Timestamp:    nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "node",
	}, true)

	if identity != nil {
		associatedPeer := rns.CalculateHash(identity, "lxmf", "delivery")
		if a.Dir.TrustLevel(associatedPeer, nil) == directory.TrustTrusted {
			existing := a.Dir.Find(destHash)
			if existing == nil {
				nodeEntry := directory.NewEntry(destHash)
				nodeEntry.DisplayName = displayName
				nodeEntry.TrustLevel = directory.TrustTrusted
				nodeEntry.HostsNode = true
				a.Dir.Remember(nodeEntry)
			}
		}
	}

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}

// handlePNAnnounce processes propagation node announces.
func (a *App) handlePNAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	displayName, _ := lxmf.DisplayNameFromAppData(appData)
	a.Logger.Info("PN announce received: hash=%x name=%q", destHash, displayName)

	now := time.Now()
	nowF := float64(now.UnixNano()) / 1e9
	a.prependAnnounce(AnnounceEvent{
		Timestamp:    now,
		TimestampF:   nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "pn",
		DisplayName:  displayName,
	})

	a.Dir.PNAnnounceReceived(directory.Announce{
		Timestamp:    nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "pn",
	}, true)

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}

// handleRRCAnnounce processes RRC chat announces.
func (a *App) handleRRCAnnounce(destHash []byte, identity *rns.Identity, appData []byte, isPathResponse bool) {
	a.Logger.Info("RRC announce received: hash=%x", destHash)

	if a.RRC != nil {
		a.RRC.AddHub(destHash, "rrc.chat", "")
	}

	a.prependAnnounce(AnnounceEvent{
		Timestamp:    time.Now(),
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "rrc",
	})

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}
