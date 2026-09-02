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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	"github.com/gmlewis/go-nomadnet/nomadnet/version"
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

	// RNS/LXMF references
	Logger       *rns.Logger
	Transport    *rns.TransportSystem
	RNS          *rns.Reticulum
	Identity     *rns.Identity
	Router       *lxmf.Router
	LXMFDest     *rns.Destination
	RNSConfigDir string

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

	mu sync.Mutex
	// psMu guards all access to PeerSettings fields. The node job-loop,
	// announce-at-start, sync, propagation, and TUI/main goroutines all reach
	// into PeerSettings; without a lock the background save (msgpack marshal of
	// the whole struct) races with concurrent field mutations such as
	// NodeLastAnnounce. SavePeerSettings marshals while holding psMu, so any
	// mutating a.PeerSettings field must do so under psMu as well.
	psMu   sync.Mutex
	initWG sync.WaitGroup
	// jobsWG tracks the background a.jobs() goroutine so Shutdown can wait for
	// it to observe ShouldRunJobs=false and exit before a.RNS.Close() tears down
	// the transport. Without this, a.RequestLXMFSync/a.AnnounceNow can race
	// ahead of RNS close and dereference a torn-down transport/router.
	// jobsStop is closed by Shutdown to interrupt the loop's sleeps so the wait
	// is immediate rather than blocking up to DeferJobs (90 s) seconds.
	// jobsStopOnce guards the close so a second Shutdown call (or ExitHandler
	// followed by Shutdown) does not panic closing an already-closed channel.
	jobsWG       sync.WaitGroup
	jobsStop     chan struct{}
	jobsStopOnce sync.Once
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
		Version:                   version.VERSION,
		ConfigDir:                 configDir,
		RNSConfigDir:              rnsConfigDir,
		EnableClient:              true,
		ForceConsoleLog:           forceConsole,
		ShouldRunJobs:             true,
		JobInterval:               5,
		DeferJobs:                 90,
		jobsStop:                  make(chan struct{}),
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

	// Seed the pages directory with a starter index.mu when one is absent
	// (the embedded default-index.mu mascot page). This is run on every
	// startup, independent of enable_node, so a fresh install — where the
	// default config has enable_node = no and startNode is therefore a no-op
	// — still gets the starter page copied into storage/pages/index.mu for
	// the operator to customize once they enable node hosting. Python only
	// creates the pages directory unconditionally (NomadNetworkApp.py:182-183)
	// and serves an in-memory placeholder when no index.mu exists; gonomadnet
	// additionally seeds a starter file. Idempotent: an existing index.mu is
	// never overwritten. (Also called from startNode; both calls are safe
	// because the write is gated on os.IsNotExist.)
	if err := a.ensureDefaultPages(); err != nil {
		return fmt.Errorf("seeding default pages: %w", err)
	}

	// Initialize logger. The [logging] loglevel config key drives the level
	// (Python NomadNetworkApp.applyConfig sets RNS.loglevel from it,
	// NomadNetworkApp.py:804-809); the config package applies the Python
	// default (4 = info) and the 0..7 clamp. The RNS stack may re-apply its
	// own config's loglevel later in NewReticulumWithLogger, matching Python
	// where Reticulum init reads ~/.reticulum/config after the app's
	// applyConfig.
	a.Logger = rns.NewLogger()
	a.Logger.SetLogLevel(a.Config.Logging.LogLevel)

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

	a.Logger.Info("Nomad Network Client %v starting...", a.Version)

	// Initialize non-blocking subsystems
	a.Dir = directory.New()
	a.Dir.SanitizeNames = a.Config.TextUI.SanitizeNames
	// Restore the persisted directory (Python Directory.__init__ →
	// load_from_disk, Directory.py:82). A missing file is not an error.
	if err := a.Dir.LoadFromDisk(a.DirectoryPath); err != nil {
		a.Logger.Error("Could not load directory from %v: %v", a.DirectoryPath, err)
	}
	// Eager-persist on every Remember (Python remember→save_to_disk,
	// Directory.py:350) so discovered nodes survive across runs even on a
	// non-shutdown exit. See directory.SetPersistPath.
	a.Dir.SetPersistPath(a.DirectoryPath)
	a.RRC = rrc.NewManager(a.StoragePath, nil)
	a.loadPeerSettings()
	a.loadIgnoredList()
	// Go-only enhancement: block the announce streams for every ignored
	// destination ("never see announcements from blocked nodes"). No Python
	// SOT counterpart — see directory.Directory.SetBlockedFilter.
	a.Dir.SetBlockedFilter(a.IsIgnored)
	a.ConversationCache.SetAttachmentPath(a.AttachmentPath)

	// Start RNS initialization in a goroutine so the TUI can start immediately.
	// RNS initialization may block on network interfaces.
	a.initWG.Go(a.initRNS)

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
	a.RRC.SetTransport(transport)
	rnsConfigDir := a.RNSConfigDir
	// When no -rnsconfig is given, pass "" through to go-reticulum, which
	// resolves it to ~/.reticulum (or /etc/reticulum / ~/.config/reticulum
	// if a config already exists there) and creates the dir + a default
	// share_instance=Yes config on first run — matching Python nomadnet's
	// RNS.Reticulum(configdir=None).

	ret, err := rns.NewReticulumWithLogger(a.Transport, rnsConfigDir, a.Logger)
	if err != nil {
		a.Logger.Error("Could not initialize Reticulum: %v", err)
		return
	}
	// Re-assert the app's loglevel: Reticulum's applyConfig re-reads
	// ~/.reticulum/config and overrides the level we set in Init (Python
	// nomadnet instead passes its loglevel into the RNS constructor, so the
	// app value always wins there).
	a.Logger.SetLogLevel(a.Config.Logging.LogLevel)
	// Publish the Reticulum under a.mu so concurrent readers (RNSConfigPath,
	// polled from the Interfaces page / UI ticker) see the pointer safely.
	// The pointer is assigned once here and never reassigned, but a bare
	// unsynchronized assignment races a lock-free reader under -race.
	a.mu.Lock()
	a.RNS = ret
	a.mu.Unlock()

	// Load or create identity
	var err2 error
	a.Identity, err2 = a.loadOrCreateIdentity(a.IdentityPath)
	if err2 != nil {
		a.Logger.Error("Could not load identity: %v", err2)
		return
	}
	a.Logger.Info("Identity loaded: %v", rns.PrettyHex(a.Identity.Hash))
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
	// Mirror Python NomadNetworkApp.py:351-352: seed the router's ignored list
	// from the persisted ignored file at startup.
	a.applyIgnoredDestinations()

	// Register delivery identity for receiving messages
	a.LXMFDest, err = a.Router.RegisterDeliveryIdentity(a.Identity, a.Config.Client.UserInterface, nil)
	if err != nil {
		a.Logger.Error("Could not register delivery identity: %v", err)
		return
	}
	a.Logger.Info("LXMF Router ready to receive on %v", rns.PrettyHex(a.LXMFDest.Hash))

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

	// Restore the saved (or best trusted) propagation node at boot (Python
	// NomadNetworkApp.py:413 self.autoselect_propagation_node() right after the
	// announce handlers register). Without this the LXMF router starts with no
	// outbound propagation node, so propagation sync and the Conversations
	// "Last sync: ... (<node>)" footer lose their default node.
	a.AutoSelectPropagationNode()

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
	// shortly after boot instead of "Never". Python makes that thread a daemon
	// and os._exits on Quit so a never-fired announce is simply abandoned; Go
	// has no daemon goroutines, so the sleep selects on jobsStop to exit on
	// shutdown instead of firing AnnounceNow against a torn-down transport.
	if a.PeerAnnounceAtStart {
		go func() {
			select {
			case <-time.After(startAnnounceDelay):
			case <-a.jobsStop:
				return
			}
			a.AnnounceNow()
			if a.UIChangeCallback != nil {
				a.UIChangeCallback()
			}
		}()
	}

	// Start background jobs. Tracked on jobsWG so Shutdown waits for the loop
	// to exit before closing a.RNS (otherwise a.RequestLXMFSync/a.AnnounceNow
	// can dereference a torn-down transport/router).
	a.jobsWG.Go(a.jobs)
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

	// Seed storage/pages/index.mu with the starter page when absent — see the
	// matching call in Init for rationale. Run unconditionally (independent of
	// enable_node) so the test harness mirrors production startup.
	if err := a.ensureDefaultPages(); err != nil {
		return fmt.Errorf("seeding default pages: %w", err)
	}

	if a.Logger == nil {
		a.Logger = rns.NewLogger()
		// Honor the [logging] loglevel config key — see Init.
		a.Logger.SetLogLevel(a.Config.Logging.LogLevel)
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
		a.Logger.Error("Could not load directory from %v: %v", a.DirectoryPath, err)
	}
	// Eager-persist on every Remember (Python remember→save_to_disk,
	// Directory.py:350) so discovered nodes survive across runs even on a
	// non-shutdown exit. See directory.SetPersistPath.
	a.Dir.SetPersistPath(a.DirectoryPath)
	a.RRC = rrc.NewManager(a.StoragePath, nil)
	a.RRC.SetIdentity(a.Identity)
	a.RRC.SetTransport(ts)
	a.RRC.SetHistoryConfig(a.RRCHistoryPerRoomCap, a.RRCFilterLoadedHistory, a.RRCEphemeralNotices)
	a.loadPeerSettings()
	a.loadIgnoredList()
	// Go-only enhancement: block the announce streams for every ignored
	// destination ("never see announcements from blocked nodes"). No Python
	// SOT counterpart — see directory.Directory.SetBlockedFilter.
	a.Dir.SetBlockedFilter(a.IsIgnored)
	a.ConversationCache.SetAttachmentPath(a.AttachmentPath)

	a.Router, err = lxmf.NewRouter(a.Transport, a.Identity, a.StoragePath)
	if err != nil {
		return fmt.Errorf("creating LXMF router: %w", err)
	}
	// Mirror Python NomadNetworkApp.py:351-352: seed the router's ignored list
	// from the persisted ignored file at startup.
	a.applyIgnoredDestinations()
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

	// Mirror the production initRNS path (which calls startNode so a node is
	// hosted when the config enables it). Tests that need a specific node state
	// write their own private config (e.g. enable_node = no) before calling
	// InitWithTransport, so the test suite never depends on the default/user
	// config — see writeTestNomadNetConfig.
	if err := a.startNode(); err != nil && a.Logger != nil {
		a.Logger.Error("node start failed in InitWithTransport: %v", err)
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
	a.LXMFMaxIncomingSize = new(int(cfg.Client.MaxAcceptedSize))
	a.LXMFMaxPropagationSize = new(int(cfg.Node.MaxTransferSize))
	a.LXMFMaxSyncSize = new(int(cfg.Node.MaxSyncSize))
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
	for part := range strings.SplitSeq(s, ",") {
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

	// Signal the background jobs loop to stop, then wait for it to exit BEFORE
	// closing RNS. The loop's RequestLXMFSync/AnnounceNow take a.mu briefly, so
	// a.mu must NOT be held across jobsWG.Wait() or the wait deadlocks with a
	// goroutine blocked acquiring a.mu. Closing jobsStop interrupts the loop's
	// time.After waits so the wait is immediate rather than blocking up to
	// DeferJobs (90 s) seconds.
	a.mu.Lock()
	a.ShouldRunJobs = false
	a.mu.Unlock()
	a.jobsStopOnce.Do(func() { close(a.jobsStop) })
	a.jobsWG.Wait()

	a.mu.Lock()
	defer a.mu.Unlock()

	// Persist the in-memory directory (known peers, trust levels, announce
	// stream) to disk before tearing anything down — Python's exit_handler does
	// self.directory.save_to_disk() first (NomadNetworkApp.py:42). Without this
	// the directory is ephemeral and every known peer is lost on restart.
	if a.Dir != nil {
		if err := a.Dir.SaveToDisk(a.DirectoryPath); err != nil {
			a.Logger.Error("Could not save directory to %v: %v", a.DirectoryPath, err)
		}
	}

	// Stop the hosted node's background job loop (Python exit_handler sets
	// should_run_jobs false, which halts Node.__jobs). stopNode waits for the
	// node Jobs goroutine to exit so n.Announce/RegisterPages do not race
	// ahead of a.RNS.Close below.
	a.stopNode()

	if a.RRC != nil {
		a.RRC.Shutdown()
	}

	if a.RNS != nil {
		if err := a.RNS.Close(); err != nil {
			a.Logger.Warning("Could not close Reticulum: %v", err)
		}
	}
}

// AnnounceNow sends an LXMF delivery announce using the configured display
// name and stamp cost, then records the announce time and persists peer
// settings. This mirrors the Python NomadNetworkApp.announce_now.
func (a *App) AnnounceNow() {
	if a.Router != nil && a.LXMFDest != nil {
		a.Router.SetInboundStampCost(a.LXMFDest.Hash, a.RequiredStampCost)
		a.Router.SetDisplayName(a.LXMFDest.Hash, a.GetDisplayName())
		if err := a.Router.Announce(a.LXMFDest.Hash); err != nil {
			a.Logger.Error("Announce failed for %x: %v", a.LXMFDest.Hash, err)
		} else {
			a.Logger.Info("Announce sent for %x", a.LXMFDest.Hash)
		}
	} else {
		a.Logger.Error("AnnounceNow skipped: router=%v lxmfDest=%v", a.Router != nil, a.LXMFDest != nil)
	}
	now := time.Now()
	a.mu.Lock()
	a.LastAnnounce = now
	a.mu.Unlock()
	a.psMu.Lock()
	if a.PeerSettings != nil {
		a.PeerSettings.LastAnnounce = float64(now.UnixNano()) / 1e9
	}
	a.savePeerSettingsLocked()
	a.psMu.Unlock()
}

// AutoSelectPropagationNode selects a default LXMF propagation node.
// When the user has manually selected a node it is used; otherwise the
// trusted known node with the fewest hops is chosen. The selection is
// pushed to the LXMF router. This mirrors the Python NomadNet
// autoselect_propagation_node.
func (a *App) AutoSelectPropagationNode() {
	var selected []byte
	a.psMu.Lock()
	if ps := a.PeerSettings; ps != nil {
		if h, ok := ps.PropagationNode.([]byte); ok && len(h) > 0 {
			selected = h
		}
	}
	a.psMu.Unlock()
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

// DirAnnounceEvents returns the persisted directory announce stream as display
// events (AnnounceEvent), mirroring Python's AnnounceStream widget which
// iterates app.directory.announce_stream (Network.py:489) — the on-disk-persisted,
// per-source-compacted stream restored by LoadFromDisk at boot. This is the
// single source of truth for the Network panel's announce stream, matching
// the original nomadnet, which keeps announce history solely on Directory
// (Directory.py:59-61 announce_stream property combining the per-type lists).
// Display names are derived from app_data: node announces store the raw UTF-8
// name (Directory.py:198), peer/pn announces use the LXMF app_data format.
func (a *App) DirAnnounceEvents() []AnnounceEvent {
	stream := a.Dir.AnnounceStream()
	events := make([]AnnounceEvent, 0, len(stream))
	for _, an := range stream {
		var name string
		switch an.AnnounceType {
		case "node":
			name = string(an.AppData)
		default: // "peer", "pn" use the LXMF app_data format.
			name, _ = lxmf.DisplayNameFromAppData(an.AppData)
		}
		sec := int64(an.Timestamp)
		nanos := int64((an.Timestamp - float64(sec)) * 1e9)
		events = append(events, AnnounceEvent{
			Timestamp:    time.Unix(sec, nanos),
			TimestampF:   an.Timestamp,
			SourceHash:   an.SourceHash,
			AppData:      an.AppData,
			AnnounceType: an.AnnounceType,
			DisplayName:  name,
		})
	}
	return events
}

// SetUIChangeCallback sets a callback for UI refresh.
func (a *App) SetUIChangeCallback(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.UIChangeCallback = fn
}

// ConversationList returns the list of all conversations, enriched with each
// peer's display name + trust level from the directory. A conversation's name
// and trust live only in the directory (the per-conversation dir on disk holds
// messages, not metadata), so the displayNames/trustLevels maps passed to
// conversation.ConversationList must be built from a.Dir — passing nil (as this
// previously did) left every conversation showing its raw hash with unknown
// trust, ignoring the name + trust the user set in the New Conversation / Peer
// Info dialogs. Mirrors Python Conversations.py, which looks up name + trust
// from self.app.directory for each conversation.
func (a *App) ConversationList() []conversation.ConversationInfo {
	if a.ConversationPath == "" {
		return nil
	}
	displayNames := map[string]string{}
	trustLevels := map[string]byte{}
	sortRanks := map[string]*int{}
	if entries, err := os.ReadDir(a.ConversationPath); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			hashHex := e.Name()
			hashBytes, err := hex.DecodeString(hashHex)
			if err != nil {
				continue
			}
			if a.Dir != nil {
				displayNames[hashHex] = a.Dir.DisplayName(hashBytes)
				trustLevels[hashHex] = a.Dir.TrustLevel(hashBytes, nil)
				if de := a.Dir.Find(hashBytes); de != nil {
					sortRanks[hashHex] = de.SortRank
				}
			}
			// Python builds a cached Conversation object for every list entry,
			// whose __init__ recalls the peer (Conversation.py:204-208) and
			// marks the peer's RNS known-destinations entry as used. Do the
			// same here so peers the user talks to survive the transport's
			// pathless/never-used cleanup even between direct openings.
			if a.Transport != nil {
				a.Transport.Recall(hashBytes)
			}
			// B2: when the directory has no display name for the peer, fall
			// back to the announce app data (LXMF display_name_from_app_data),
			// matching nomadnet 1.2.8's _update_peer_info
			// (Conversations.py:2092-2101): directory → announce app data → hash.
			if displayNames[hashHex] == "" && a.Transport != nil {
				if appData := a.Transport.RecallAppData(hashBytes); len(appData) > 0 {
					if name, err := lxmf.DisplayNameFromAppData(appData); err == nil && name != "" {
						displayNames[hashHex] = name
					}
				}
			}
		}
	}
	return conversation.ConversationList(a.ConversationPath, displayNames, trustLevels, sortRanks)
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

// removeAllWithRetry removes path with os.RemoveAll, retrying on transient
// ENOTEMPTY/EBUSY errors for up to ~100ms. After RNS.Close() a handful of
// background goroutines may still be flushing ratchet/destination storage and
// briefly recreate files mid-removal; the retry lets them quiesce. Mirrors the
// helper of the same name in go-reticulum's testutils package.
func removeAllWithRetry(path string) error {
	const maxAttempts = 10
	const retryDelay = 10 * time.Millisecond

	for range maxAttempts {
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

// loadOrCreateIdentity loads an existing identity or creates a new one.
func (a *App) loadOrCreateIdentity(path string) (*rns.Identity, error) {
	if _, err := os.Stat(path); err == nil {
		id, err := rns.FromFile(path, a.Logger)
		if err != nil {
			return nil, fmt.Errorf("loading identity: %w", err)
		}
		a.Logger.Info("Loaded identity from %v", path)
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
	a.Logger.Info("Created new identity at %v", path)
	return id, nil
}

// lxmfDelivery handles incoming LXMF messages.
func (a *App) lxmfDelivery(msg *lxmf.Message) {
	a.Logger.Info("Received LXMF message from %v", rns.PrettyHex(msg.SourceHash))

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

	a.Dir.NodeAnnounceReceived(directory.Announce{
		Timestamp:    nowF,
		SourceHash:   destHash,
		AppData:      appData,
		AnnounceType: "node",
	}, true)

	if identity != nil && !a.IsIgnored(destHash) {
		// Go-only enhancement guard (no Python SOT counterpart): a blocked
		// node is never auto-remembered as a trusted directory entry, even
		// when its operator peer is trusted. See Directory.SetBlockedFilter,
		// which already rejected the stream entry above.
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

	if a.UIChangeCallback != nil {
		a.UIChangeCallback()
	}
}
