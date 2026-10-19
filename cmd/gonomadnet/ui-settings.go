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
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-nomadnet/tui"
)

// networkShortcutBar renders the Network shortcut bar, mirroring Python's
// NetworkDisplayShortcuts (Network.py:19-23): the "[C-y] Copy  " token is
// present only when [textui] clipboard_copy is enabled. Python advertises the
// key without binding a handler; the port matches that exactly.
func networkShortcutBar(clipboardCopy bool) string {
	bar := "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  "
	if clipboardCopy {
		bar += "[C-y] Copy  "
	}
	return bar + "[C-g] Fullscreen  [C-s / C-b] Save Node"
}

// textUIAppearance validates the [textui] theme and colormode the way Python's
// applyConfig does (NomadNetworkApp.py:953-977): an absent theme is dark, an
// absent colormode is COLORMODE_16, and any other unrecognized spelling raises
// ValueError (which panics). This port silently fell back to dark/truecolor,
// hiding a typo in the operator's config.
func textUIAppearance(cfg *config.Config) (theme, colorMode int, err error) {
	theme = tui.ThemeDark
	colorMode = tui.ColorMode16
	if cfg == nil {
		colorMode = tui.ColorModeTrue
		return theme, colorMode, nil
	}

	switch strings.ToLower(strings.TrimSpace(cfg.TextUI.Theme)) {
	case "", "dark":
		theme = tui.ThemeDark
	case "light":
		theme = tui.ThemeLight
	default:
		return 0, 0, fmt.Errorf("the selected text UI theme is invalid: %v", cfg.TextUI.Theme)
	}

	cm, ok := tui.ParseColorModeStrict(cfg.TextUI.ColorMode)
	if !ok {
		return 0, 0, fmt.Errorf("the selected text UI color mode is invalid: %v", cfg.TextUI.ColorMode)
	}
	return theme, cm, nil
}
