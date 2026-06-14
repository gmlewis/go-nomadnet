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

	if err := tuiApp.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}
