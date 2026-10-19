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
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-nomadnet/tui"
)

// TestNetworkShortcutBar pins Python's NetworkDisplayShortcuts (Network.py:
// 19-23): the Copy token appears only with clipboard_copy enabled.
func TestNetworkShortcutBar(t *testing.T) {
	t.Parallel()

	withCopy := networkShortcutBar(true)
	if !strings.Contains(withCopy, "[C-y] Copy  ") {
		t.Errorf("clipboard_copy bar = %q, want the [C-y] Copy token", withCopy)
	}
	without := networkShortcutBar(false)
	if strings.Contains(without, "Copy") {
		t.Errorf("bar without clipboard_copy = %q, want no Copy token", without)
	}
	if !strings.HasPrefix(without, "[C-l] Nodes/Announces") || !strings.HasSuffix(without, "[C-s / C-b] Save Node") {
		t.Errorf("network bar = %q, want Python's surrounding text", without)
	}
}

// TestTextUIAppearance pins Python's theme/colormode validation
// (NomadNetworkApp.py:953-977): recognized spellings map to their values, an
// absent theme is dark, and anything else is a ValueError (a panic).
func TestTextUIAppearance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		theme     string
		colorMode string
		wantTheme int
		wantColor int
		wantErr   bool
	}{
		{name: "dark 24bit", theme: "dark", colorMode: "24bit", wantTheme: tui.ThemeDark, wantColor: tui.ColorModeTrue},
		{name: "light 256", theme: "light", colorMode: "256", wantTheme: tui.ThemeLight, wantColor: tui.ColorMode256},
		{name: "monochrome", theme: "dark", colorMode: "monochrome", wantTheme: tui.ThemeDark, wantColor: tui.ColorModeMono},
		{name: "16 and 88", theme: "light", colorMode: "16", wantTheme: tui.ThemeLight, wantColor: tui.ColorMode16},
		{name: "invalid theme", theme: "blue", colorMode: "24bit", wantErr: true},
		{name: "invalid colormode", theme: "dark", colorMode: "junk", wantErr: true},
		{name: "empty colormode is invalid", theme: "dark", colorMode: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.DefaultConfig()
			cfg.TextUI.Theme = tc.theme
			cfg.TextUI.ColorMode = tc.colorMode
			theme, colorMode, err := textUIAppearance(cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("textUIAppearance accepted theme=%q colormode=%q", tc.theme, tc.colorMode)
				}
				return
			}
			if err != nil {
				t.Fatalf("textUIAppearance: %v", err)
			}
			if theme != tc.wantTheme || colorMode != tc.wantColor {
				t.Errorf("textUIAppearance = (%v, %v), want (%v, %v)", theme, colorMode, tc.wantTheme, tc.wantColor)
			}
		})
	}

	// A nil config (defensive) renders with the shipped defaults.
	if theme, colorMode, err := textUIAppearance(nil); err != nil || theme != tui.ThemeDark || colorMode != tui.ColorModeTrue {
		t.Errorf("textUIAppearance(nil) = (%v, %v, %v)", theme, colorMode, err)
	}
}
