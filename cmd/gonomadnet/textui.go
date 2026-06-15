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
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/tui"
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
		defer logFile.Close()
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

	// Create and run the TUI
	tuiApp := tui.NewApp(theme, glyphSet)
	tuiApp.SetQuitCallback(func() {
		a.Shutdown()
		tuiApp.Stop()
	})

	// Wire up real displays BEFORE setting root
	wireDisplays(tuiApp, a)

	// Set root after all displays are wired up
	tuiApp.SetRoot()

	if err := tuiApp.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

// wireDisplays connects the app data to the TUI display widgets.
func wireDisplays(tuiApp *tui.App, a *app.App) {
	main := tuiApp.Main

	// Network display
	networkDisplay := tui.NewNetworkDisplay(tuiApp.Application, nil, nil)
	main.SetDisplay("network", networkDisplay.Widget())

	// Conversations display
	convs := a.ConversationList()
	tuiConvs := make([]tui.ConversationInfo, len(convs))
	for i, c := range convs {
		// Convert trust level byte to string
		trustStr := "unknown"
		switch c.TrustLevel {
		case 0xFF:
			trustStr = "trusted"
		case 0x01:
			trustStr = "untrusted"
		case 0x00:
			trustStr = "warning"
		}

		// Convert LastActivity (float64 unix timestamp) to time.Time
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
	conversationsDisplay := tui.NewConversationsDisplay(tuiApp.Application, tuiConvs)
	main.SetDisplay("conversations", conversationsDisplay.Widget())

	// Channels display
	channelsDisplay := tui.NewChannelsDisplay(tuiApp.Application, nil)
	main.SetDisplay("channels", channelsDisplay.Widget())

	// Config display
	configPath := a.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(a.ConfigDir, "config")
	}
	configDisplay := tui.NewConfigDisplay(tuiApp.Application, configPath)
	main.SetDisplay("config", configDisplay.Widget())

	// Log display
	logPath := a.LogFilePath
	if logPath == "" {
		logPath = filepath.Join(a.ConfigDir, "logfile")
	}
	logDisplay := tui.NewLogDisplay(tuiApp.Application, logPath, 50)
	main.SetDisplay("log", logDisplay.Widget())

	// Guide display
	guideDisplay := tui.NewGuideDisplay(tuiApp.Application)
	main.SetDisplay("guide", guideDisplay.Widget())

	// Interfaces display
	interfaces := []tui.InterfaceInfo{
		{Name: "Michmesh Testnet", Type: "TCPClientInterface", Status: "connected", Target: "RNS.MichMesh.net:7822"},
	}
	interfacesDisplay := tui.NewInterfacesDisplay(tuiApp.Application, interfaces)
	main.SetDisplay("interfaces", interfacesDisplay.Widget())

	// Intro/splash display
	introDisplay := tui.NewIntroDisplay("Nomad Network", a.Version)
	main.SetDisplay("quit", introDisplay.Widget())
}
