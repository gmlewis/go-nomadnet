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
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
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

	// Wire up real displays BEFORE setting root. The returned cleanup releases
	// background resources (log tail) on shutdown.
	logCleanup := wireDisplays(tuiApp, a)

	tuiApp.SetQuitCallback(func() {
		tuiApp.Main.StopUnreadBlink()
		if logCleanup != nil {
			logCleanup()
		}
		a.Shutdown()
		tuiApp.Stop()
	})

	// Probe unread conversations every 2 s and swap the menu indicator glyph
	// (Python MenuDisplay.update_display job, Main.py:216-230). The app injects
	// the probe so the tui package need not import nomadnet/app.
	tuiApp.Main.SetUnreadCheck(func() bool { return a.HasUnreadConversations() })
	tuiApp.Main.StartUnreadBlink()

	// Show the intro splash for intro_time seconds (default 1), then swap to
	// the main display — matching Python's TextUI.py:223-232. intro_time <= 0
	// skips the splash and shows the main display immediately.
	introTime := 1.0
	if a.Config != nil {
		introTime = a.Config.TextUI.IntroTime
	}
	intro := tui.NewIntroDisplay("Nomad Network", a.Version)
	tuiApp.ShowIntro(intro.Widget(), introTime)

	if err := tuiApp.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

// wireDisplays connects the app data to the TUI display widgets.
// wireDisplays builds and registers all body-page displays. It returns a
// cleanup function that must be invoked on shutdown to release background
// resources (e.g. the log live-tail goroutine).
func wireDisplays(tuiApp *tui.App, a *app.App) func() {
	main := tuiApp.Main

	// Network display — use real announces from the app
	networkDisplay := tui.NewNetworkDisplay(tuiApp, nil, nil)
	networkDisplay.SanitizeNames = a.Config.TextUI.SanitizeNames

	// refreshAnnounces re-fetches the announce stream from the app and updates
	// the network display's left pane.
	refreshAnnounces := func() {
		anns := a.GetAnnounces()
		tuiConvs := make([]tui.AnnounceEntry, len(anns))
		for i, ann := range anns {
			tuiConvs[i] = tui.AnnounceEntry{
				Timestamp:   ann.Timestamp,
				TimestampF:  ann.TimestampF,
				SourceHash:  fmt.Sprintf("%x", ann.SourceHash),
				AppData:     string(ann.AppData),
				Type:        ann.AnnounceType,
				DisplayName: ann.DisplayName,
			}
		}
		networkDisplay.UpdateAnnounces(tuiConvs)
	}
	// refreshNodes re-fetches the saved-node list from the app directory.
	refreshNodes := func() {
		entries := a.Dir.KnownNodes()
		nodes := make([]tui.NodeEntry, 0, len(entries))
		for _, e := range entries {
			trustStr := "unknown"
			switch e.TrustLevel {
			case directory.TrustTrusted:
				trustStr = "trusted"
			case directory.TrustUntrusted:
				trustStr = "untrusted"
			case directory.TrustWarning:
				trustStr = "warning"
			}
			delivery := "direct"
			if e.PreferredDelivery == 0x01 {
				delivery = "propagated"
			}
			nodes = append(nodes, tui.NodeEntry{
				SourceHash:  fmt.Sprintf("%x", e.SourceHash),
				DisplayName: e.DisplayName,
				TrustLevel:  trustStr,
				HostsNode:   e.HostsNode,
				Delivery:    delivery,
			})
		}
		networkDisplay.UpdateNodes(nodes)
	}
	// Wire up real announce data via callback
	a.SetUIChangeCallback(func() {
		refreshAnnounces()
		refreshNodes()
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
				if networkDisplay.ShowingNodes() {
					node, ok := networkDisplay.SelectedNode()
					if !ok {
						return
					}
					hash, ok := app.SourceHashFromHex(node.SourceHash)
					if ok {
						a.ForgetNode(hash)
					}
					refreshNodes()
				} else {
					ann, ok := networkDisplay.SelectedAnnounce()
					if !ok {
						return
					}
					a.RemoveAnnounce(ann.TimestampF)
					refreshAnnounces()
				}
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
				if ann, ok := networkDisplay.SelectedAnnounce(); ok {
					hash, ok := app.SourceHashFromHex(ann.SourceHash)
					if ok {
						a.SaveNode(hash, ann.DisplayName)
					}
				} else if node, ok := networkDisplay.SelectedNode(); ok {
					hash, ok := app.SourceHashFromHex(node.SourceHash)
					if ok {
						a.SaveNode(hash, node.DisplayName)
					}
				}
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
			UnreadCount: c.UnreadCount,
			Failed:      c.Failed,
			FailedCount: c.FailedCount,
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
				UnreadCount: c.UnreadCount,
				Failed:      c.Failed,
				FailedCount: c.FailedCount,
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
				a.DeleteConversation(conv.SourceHash)
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
				hash, ok := app.SourceHashFromHex(text)
				if !ok {
					return
				}
				a.CreateDirectoryEntry(hash, "")
				refreshConvs()
			},
			func() {},
		)
	}
	conversationsDisplay.OnToggleSort = func() {
		conversationsDisplay.ToggleSort()
	}
	conversationsDisplay.OnShowQR = func() {
		addr := a.LXMFAddressHex()
		text := "[gray]LXMF address display — not available[-]"
		if addr != "" {
			text = fmt.Sprintf("[lightblue]LXMF Address[-]\n\n[white]<%s>[-]", addr)
		}
		tuiApp.Dialogs.ShowDialog("LXMF Address",
			tview.NewTextView().
				SetDynamicColors(true).
				SetTextAlign(tview.AlignCenter).
				SetText(text),
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
				hash, ok := app.SourceHashFromHex(conv.SourceHash)
				if !ok {
					return
				}
				a.SetPeerDisplayName(hash, text)
				refreshConvs()
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
				if a.Router == nil {
					return
				}
				if _, err := a.Router.IngestLXMURI(text); err != nil {
					tuiApp.Dialogs.ShowDialog("Ingest message URI",
						tview.NewTextView().
							SetDynamicColors(true).
							SetText(fmt.Sprintf("[red]Could not decode URI:[-]\n\n%s", err)),
						50, 8, nil)
					return
				}
				refreshConvs()
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
		a.RequestLXMFSync(0)
	}

	// Block/Unblock peer callbacks — used in conversation context
	blockPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowConfirmDialog("Block this peer?\nThis will blackhole their identity and delete this conversation.",
			func() {
				hash, ok := app.SourceHashFromHex(sourceHash)
				if !ok {
					return
				}
				a.BlockDestination(hash, "blocked from conversations")
				a.DeleteConversation(sourceHash)
				refreshConvs()
			},
			func() {},
		)
	}
	unblockPeer := func(sourceHash string) {
		tuiApp.Dialogs.ShowConfirmDialog("Unblock this peer?\nThis lifts the blackhole and removes them from your ignored list.",
			func() {
				hash, ok := app.SourceHashFromHex(sourceHash)
				if !ok {
					return
				}
				a.UnblockDestination(hash)
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

	// Trust banner button wiring (Python _on_trust_click/_on_block_click/
	// _on_ignore_click). Defined after blockPeer so it can delegate to it.
	conversationsDisplay.OnTrustPeer = func(sourceHash string) {
		hash, ok := app.SourceHashFromHex(sourceHash)
		if !ok {
			return
		}
		a.SetPeerTrustLevel(hash, directory.TrustTrusted)
		refreshConvs()
	}
	conversationsDisplay.OnBlockPeer = func(sourceHash string) {
		blockPeer(sourceHash)
	}
	conversationsDisplay.OnIgnorePeer = func(sourceHash string) {
		// "Do nothing" just dismisses the banner; no app action.
		_ = sourceHash
	}

	// Network "Converse" / "Msg Op": open a conversation with the selected
	// announce's identity — create a directory entry, refresh the conversation
	// list, and switch to the Conversations page. (Python converse(msg) /
	// msg_op resolve the operator's LXMF address; until RNS identity resolution
	// is wired, both open a conversation with the announce's own identity.)
	openConversationFromAnnounce := func() {
		ann, ok := networkDisplay.SelectedAnnounce()
		if !ok {
			return
		}
		hash, ok := app.SourceHashFromHex(ann.SourceHash)
		if !ok {
			return
		}
		a.CreateDirectoryEntry(hash, ann.DisplayName)
		refreshConvs()
		main.SelectPage("conversations")
	}
	networkDisplay.OnConverse = openConversationFromAnnounce
	networkDisplay.OnMsgOp = openConversationFromAnnounce

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

	// Begin live tailing (Python's LogTerminal runs `tail -fn50` continuously
	// while the page exists). It is a no-op when the log file is absent. The
	// returned cleanup stops the goroutine on shutdown.
	logDisplay.StartTailing()

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
	interfacesDisplay.OnEditInterface = func() {
		tuiApp.Dialogs.ShowInputDialog("Edit Interface",
			"Interface name:", "",
			func(text string) {
				_ = text
				// TODO: Edit interface via RNS config
			},
			func() {},
		)
	}
	interfacesDisplay.OnRemoveInterface = func() {
		tuiApp.Dialogs.ShowConfirmDialog("Remove selected interface?",
			func() {
				// TODO: Remove interface via RNS config
			},
			func() {},
		)
	}
	interfacesDisplay.OnShowInterface = func(idx int) {
		if idx < 0 || idx >= len(interfaces) {
			return
		}
		iface := interfaces[idx]
		tuiApp.Dialogs.ShowDialog("Interface: "+iface.Name,
			tview.NewTextView().
				SetDynamicColors(true).
				SetText(fmt.Sprintf("[::b]%s[-]\n\nType: %s\nStatus: %s\nTarget: %s",
					iface.Name, iface.Type, iface.Status, iface.Target)),
			50, 9, nil)
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

	// On first run, the original opens the Guide on the "First Run" topic
	// (Main.py:27 `if app.firstrun: active_display = guide_display` +
	// Guide.py:221-224 first_run_entry.display_topic). NewMainDisplay defaults
	// to the Conversations page, so override that here.
	if a.FirstRun {
		guideDisplay.ShowFirstRun()
		main.SelectPage("guide")
	}

	return logDisplay.StopTailing
}
