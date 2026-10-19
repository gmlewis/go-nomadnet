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
	"os"
	"path/filepath"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

// resolveUIMode decides between the text UI and daemon mode the way Python does
// (NomadNetworkApp.py:432-443 + ui/__init__.py:24-45):
//
//   - --daemon forces UI_NONE, whatever the config says;
//   - otherwise the config's [client] user_interface selects: "text" → the text
//     UI, "none" → the daemon;
//   - "menu"/"graphical"/"web" reach Python's unimplemented stubs, which log and
//     panic;
//   - a missing or unrecognised value (including an absent key, Python's None)
//     is "Invalid UI mode" and panics.
//
// --textui only cancels --daemon in Python; it does not override the config, so
// a config saying user_interface = none still runs the daemon under -t.
//
// A missing config file resolves to the text UI: Python creates its default
// config (which says user_interface = text) and then applies it.
func resolveUIMode(configDir string, daemon, textUI bool) (bool, error) {
	if textUI {
		daemon = false
	}
	if daemon {
		return true, nil
	}

	configPath := filepath.Join(configDir, "config")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return false, err
	}

	switch ui := strings.ToLower(strings.TrimSpace(cfg.Client.UserInterface)); ui {
	case "text":
		return false, nil
	case "none":
		return true, nil
	case "menu":
		return false, fmt.Errorf("Menu UI not implemented")
	case "graphical":
		return false, fmt.Errorf("Graphical UI not implemented")
	case "web":
		return false, fmt.Errorf("Web UI not implemented")
	case "":
		return false, fmt.Errorf("invalid UI mode")
	default:
		return false, fmt.Errorf("invalid UI mode: %v", ui)
	}
}
