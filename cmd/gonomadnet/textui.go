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

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/tui"
	"github.com/rivo/tview"
)

// runTextUI starts NomadNet with the terminal UI.
func runTextUI(configDir, rnsConfigDir string) {
	// Ensure the log directory exists
	logDir := filepath.Join(configDir, "logs")
	_ = os.MkdirAll(logDir, 0o755)

	// Redirect standard log to file BEFORE any logging happens
	// (matches gornphone pattern to prevent log output destroying TUI)
	logPath := filepath.Join(logDir, "nomadnet.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer func() { _ = logFile.Close() }()
		log.SetOutput(logFile)
		log.SetFlags(0)
	}

	log.Printf("Nomad Network text UI starting...")

	// Initialize the app
	a := app.NewApp(configDir, rnsConfigDir, false, false)
	if err := a.Init(); err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	// Determine theme from config
	theme := tui.ThemeDark
	if a.Config != nil && a.Config.TextUI.Theme == "light" {
		theme = tui.ThemeLight
	}

	// Determine glyph set
	glyphSet := tui.GlyphUnicode
	if a.Config != nil {
		switch a.Config.TextUI.Glyphs {
		case "plain":
			glyphSet = tui.GlyphPlain
		case "nerdfont":
			glyphSet = tui.GlyphNerd
		}
	}

	// Determine color depth from config (default 24-bit true color).
	colorMode := tui.ColorModeTrue
	if a.Config != nil {
		colorMode = tui.ParseColorMode(a.Config.TextUI.ColorMode)
	}

	// Create and run the TUI
	tuiApp := tui.NewApp(theme, glyphSet, colorMode)
	tuiApp.SetQuitCallback(func() {
		tuiApp.Main.StopUnreadBlink()
		a.Shutdown()
		tuiApp.Stop()
	})

	// Wire up real displays BEFORE setting root
	wireDisplays(tuiApp, a)

	// Probe unread conversations every 2 s and swap the menu indicator glyph
	// (Python MenuDisplay.update_display job, Main.py:216-230). The app injects
	// the probe so the tui package need not import nomadnet/app.
	tuiApp.Main.SetUnreadCheck(func() bool { return a.HasUnreadConversations() })
	tuiApp.Main.StartUnreadBlink()

	// Set root after all displays are wired up
	tuiApp.SetRoot()

	if err := tuiApp.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

// wireDisplays connects the app data to the TUI display widgets.
func wireDisplays(tuiApp *tui.App, a *app.App) {
	main := tuiApp.Main

	// Network display — use real announces from the app
	networkDisplay := tui.NewNetworkDisplay(tuiApp, nil, nil)
	// Wire up real announce data via callback
	a.SetUIChangeCallback(func() {
		anns := a.GetAnnounces()
		tuiConvs := make([]tui.AnnounceEntry, len(anns))
		for i, ann := range anns {
			tuiConvs[i] = tui.AnnounceEntry{
				Timestamp:   ann.Timestamp,
				SourceHash:  fmt.Sprintf("%x", ann.SourceHash),
				AppData:     string(ann.AppData),
				Type:        ann.AnnounceType,
				DisplayName: ann.DisplayName,
			}
		}
		networkDisplay.UpdateAnnounces(tuiConvs)
	})
	main.SetDisplay("network", networkDisplay.Widget())
	main.SetShortcut("network", "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node")

	// Wire Esc to go back from AnnounceInfo before quitting.
	main.SetEscCallback(func() bool {
		return networkDisplay.HandleEsc()
	})

	// Wire node connect to open the browser.
	networkDisplay.SetNavigateCallback(func(url string) {
		// TODO: Switch to browser display and load the URL.
		_ = url
	})

	// Wire network keyboard shortcuts
	networkDisplay.OnDeleteSelected = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Delete selected entry?",
			func() {
				// TODO: Delete selected announce/node
			},
			func() {},
		)
	}
	networkDisplay.OnShowPeers = func() {
		tuiApp.Dialogs.ShowDialog("LXMF Peers",
			tview.NewTextView().
				SetDynamicColors(true).
				SetTextAlign(tview.AlignCenter).
				SetText("[gray]LXMF Peers list — TODO: wire to propagation peers[-]"),
			50, 10, nil)
	}
	networkDisplay.OnURLDialog = func() {
		tuiApp.Dialogs.ShowInputDialog("Navigate",
			"Enter URL:", "",
			func(text string) {
				if text != "" {
					networkDisplay.SetNavigateCallback(func(url string) { _ = url })
				}
			},
			func() {},
		)
	}
	networkDisplay.OnSaveNode = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Save selected node?",
			func() {
				// TODO: Save node to directory
			},
			func() {},
		)
	}

	// Conversations display
	convs := a.ConversationList()
	tuiConvs := make([]tui.ConversationInfo, len(convs))
	for i, c := range convs {
		trustStr := "unknown"
		switch c.TrustLevel {
		case 0xFF:
			trustStr = "trusted"
		case 0x01:
			trustStr = "untrusted"
		case 0x00:
			trustStr = "warning"
		}

		var lastTime time.Time
		if c.LastActivity > 0 {
			lastTime = time.Unix(int64(c.LastActivity), 0)
		}

		tuiConvs[i] = tui.ConversationInfo{
			SourceHash:  c.SourceHash,
			DisplayName: c.DisplayName,
			TrustLevel:  trustStr,
			LastTime:    lastTime,
			Unread:      c.Unread,
		}
	}
	conversationsDisplay := tui.NewConversationsDisplay(tuiApp, tuiConvs)
	main.SetDisplay("conversations", conversationsDisplay.Widget())

	// Conversations supplies its shortcut bar dynamically (it switches between
	// the list/editor/body bars by focus region). Other pages use their static
	// SetShortcut text. Registered per-page so it only applies to conversations.
	main.SetShortcutCallback("conversations", func() string {
		return conversationsDisplay.GetShortcutText()
	})

	// refreshConvs updates the conversation list from the app.
	refreshConvs := func() {
		newConvs := a.ConversationList()
		tuiConvs = make([]tui.ConversationInfo, len(newConvs))
		for i, c := range newConvs {
			trustStr := "unknown"
			switch c.TrustLevel {
			case 0xFF:
				trustStr = "trusted"
			case 0x01:
				trustStr = "untrusted"
			case 0x00:
				trustStr = "warning"
			}
			var lastTime time.Time
			if c.LastActivity > 0 {
				lastTime = time.Unix(int64(c.LastActivity), 0)
			}
			tuiConvs[i] = tui.ConversationInfo{
				SourceHash:  c.SourceHash,
				DisplayName: c.DisplayName,
				TrustLevel:  trustStr,
				LastTime:    lastTime,
				Unread:      c.Unread,
			}
		}
		conversationsDisplay.SetConversations(tuiConvs)
	}

	// Wire conversation keyboard shortcuts
	conversationsDisplay.OnDeleteConv = func() {
		idx := conversationsDisplay.GetSelectedIndex()
		if idx < 0 || idx >= len(tuiConvs) {
			return
		}
		conv := tuiConvs[idx]
		tuiApp.Dialogs.ShowConfirmDialog("Delete conversation with "+conv.DisplayName+"?",
			func() {
				// TODO: Call a.DeleteConversation(conv.SourceHash) when app method exists
				_ = a
				_ = conv
				refreshConvs()
			},
			func() {},
		)
	}
	conversationsDisplay.OnNewConv = func() {
		tuiApp.Dialogs.ShowInputDialog("New Conversation",
			"Address (hex hash):", "",
			func(text string) {
				if text == "" {
					return
				}
				// TODO: Call a.CreateDirectoryEntry when app method exists
				_ = text
				refreshConvs()
			},
			func() {},
		)
	}
	conversationsDisplay.OnToggleSort = func() {
		conversationsDisplay.ToggleSort()
	}
	conversationsDisplay.OnShowQR = func() {
		tuiApp.Dialogs.ShowDialog("LXMF Address",
			tview.NewTextView().
				SetDynamicColors(true).
				SetTextAlign(tview.AlignCenter).
				SetText("[gray]LXMF address display — TODO: wire to app identity[-]"),
			50, 8, nil)
	}
	conversationsDisplay.OnEditPeerInfo = func() {
		conv, ok := conversationsDisplay.GetSelectedConversation()
		if !ok {
			return
		}
		tuiApp.Dialogs.ShowInputDialog("Peer Info",
			"Display name:", conv.DisplayName,
			func(text string) {
				_ = text
				// TODO: Update directory entry when app method exists
			},
			func() {},
		)
	}
	conversationsDisplay.OnIngestURI = func() {
		tuiApp.Dialogs.ShowInputDialog("Ingest LXM URI",
			"URI:", "",
			func(text string) {
				if text == "" {
					return
				}
				_ = text
				// TODO: Parse and ingest LXM URI
			},
			func() {},
		)
	}
	conversationsDisplay.OnSync = func() {
		tuiApp.Dialogs.ShowDialog("Sync",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText("[gray]Syncing conversations...[-]"),
			40, 5, nil)
		// TODO: Trigger actual LXMF sync via app.RequestLXMFSync()
	}

	// Block/Unblock peer callbacks — used in conversation context
	blockPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowConfirmDialog("Block this peer?\nThis will blackhole their identity and delete this conversation.",
			func() {
				// TODO: Block peer via app when method exists
				_ = sourceHash
			},
			func() {},
		)
	}
	unblockPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowConfirmDialog("Unblock this peer?\nThis lifts the blackhole and removes them from your ignored list.",
			func() {
				// TODO: Unblock peer via app when method exists
				_ = sourceHash
			},
			func() {},
		)
	}
	pingPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowDialog("Ping",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[gray]Pinging %s...[-]", sourceHash[:8])),
			40, 5, nil)
		// TODO: Actual ping via RNS transport
	}
	_ = blockPeer
	_ = unblockPeer
	_ = pingPeer

	// Channels display
	channelsDisplay := tui.NewChannelsDisplay(tuiApp, nil)
	main.SetDisplay("channels", channelsDisplay.Widget())
	main.SetShortcut("channels", "[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove")

	// Wire channel keyboard shortcuts
	channelsDisplay.OnNewHub = func() {
		tuiApp.Dialogs.ShowInputDialog("New Hub",
			"Hub address (hex hash):", "",
			func(text string) {
				if text == "" {
					return
				}
				name := tui.TruncateString(text, 8)
				_ = a
				// TODO: Call a.AddHub when app method exists
				_ = text
				_ = name
			},
			func() {},
		)
	}
	channelsDisplay.OnJoinRoom = func() {
		tuiApp.Dialogs.ShowInputDialog("Join Room",
			"Room name:", "",
			func(text string) {
				if text == "" {
					return
				}
				_ = text
				// TODO: Join room via RRC hub
			},
			func() {},
		)
	}
	channelsDisplay.OnRemoveHub = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Remove selected hub/room?",
			func() {
				// TODO: Remove hub/room via RRC hub
			},
			func() {},
		)
	}
	channelsDisplay.OnEditHub = func() {
		tuiApp.Dialogs.ShowInputDialog("Edit Hub",
			"Display name:", "",
			func(text string) {
				if text == "" {
					return
				}
				// TODO: Update hub display name
				_ = text
			},
			func() {},
		)
	}
	channelsDisplay.OnConnect = func() {
		tuiApp.Dialogs.ShowDialog("Connect",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText("[gray]Connecting to hub...[-]"),
			40, 5, nil)
		// TODO: Connect to hub via RRC
	}
	channelsDisplay.OnDisconnect = func() {
		tuiApp.Dialogs.ShowDialog("Disconnect",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText("[gray]Disconnected from hub.[-]"),
			40, 5, nil)
		// TODO: Disconnect from hub
	}
	channelsDisplay.OnToggleAutoReconnect = func() {
		// TODO: Toggle auto-reconnect setting
	}

	// Config display
	configPath := a.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(a.ConfigDir, "config")
	}
	configDisplay := tui.NewConfigDisplay(tuiApp, configPath)
	main.SetDisplay("config", configDisplay.Widget())
	main.SetShortcut("config", "")

	// Log display
	logPath := a.LogFilePath
	if logPath == "" {
		logPath = filepath.Join(a.ConfigDir, "logfile")
	}
	logDisplay := tui.NewLogDisplay(tuiApp, logPath, 50)
	main.SetDisplay("log", logDisplay.Widget())
	main.SetShortcut("log", "")

	// Guide display
	guideDisplay := tui.NewGuideDisplay(tuiApp)
	main.SetDisplay("guide", guideDisplay.Widget())
	main.SetShortcut("guide", "")

	// Interfaces display
	interfaces := []tui.InterfaceInfo{
		{Name: "Michmesh Testnet", Type: "TCPClientInterface", Status: "connected", Target: "RNS.MichMesh.net:7822"},
	}
	interfacesDisplay := tui.NewInterfacesDisplay(tuiApp, interfaces)
	main.SetDisplay("interfaces", interfacesDisplay.Widget())
	main.SetShortcut("interfaces", "[C-a] Add Interface [C-e] Edit Interface [C-x] Remove Interface [Enter] Show Interface [C-w] Open Text Editor")

	// Wire interfaces keyboard shortcuts
	interfacesDisplay.OnAddInterface = func() {
		tuiApp.Dialogs.ShowInputDialog("Add Interface",
			"Interface type:", "",
			func(text string) {
				_ = text
				// TODO: Add interface via RNS config
			},
			func() {},
		)
	}
	interfacesDisplay.OnConfigEditor = func() {
		tuiApp.Dialogs.ShowDialog("Config Editor",
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[gray]Config file: %s[-]\n\n[gray]Edit this file with your text editor.[-]", configPath)),
			60, 8, nil)
	}

	// Browser display (hidden by default, shown when navigating from network)
	browserDisplay := tui.NewBrowserDisplay(tuiApp)
	main.SetDisplay("browser", browserDisplay.Widget())
	main.SetShortcut("browser", "[C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-s] Save  [C-y] Copy URL  [C-g] Fullscreen")

	// Wire browser keyboard shortcuts
	browserDisplay.OnBack = func() { browserDisplay.GoBack() }
	browserDisplay.OnForward = func() { browserDisplay.GoForward() }
	browserDisplay.OnReload = func() { browserDisplay.Reload() }
	browserDisplay.OnCopyURL = func() {
		url := browserDisplay.CurrentURL()
		if url != "" {
			_ = tui.CopyToClipboard(url)
		}
	}
	browserDisplay.OnURLDialog = func() {
		tuiApp.Dialogs.ShowInputDialog("Navigate",
			"Enter URL:", "",
			func(text string) {
				if text != "" {
					browserDisplay.LoadURL(text)
				}
			},
			func() {},
		)
	}
	browserDisplay.OnSaveNode = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Save connected node to directory?",
			func() { /* TODO */ },
			func() {},
		)
	}
	browserDisplay.OnDisconnect = func() {
		browserDisplay.SetContent("[gray]Disconnected.[-]")
	}
	browserDisplay.OnToggleFullscreen = func() {
		// TODO: Toggle fullscreen
	}

	// Wire network connect to browser
	networkDisplay.SetNavigateCallback(func(url string) {
		main.SetDisplay("browser", browserDisplay.Widget())
		browserDisplay.LoadURL(url)
	})

	// Intro/splash display
	introDisplay := tui.NewIntroDisplay("Nomad Network", a.Version)
	main.SetDisplay("quit", introDisplay.Widget())
}
