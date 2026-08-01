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

package tui

import (
	"strings"
	"testing"
)

// TestMenuStructureMatchesPython asserts the top-level menu matches the Python
// original (nomadnet/ui/textui/Main.py:201-204). Golden values were captured
// from a live headless run of the original (nerd glyph set, dark theme,
// 135x32, frame 00) via tooling/tui-parity/summary.py:
//
//	menu_items(8)=['Conversations','Network','Channels','Log','Interfaces','Config','Guide','Quit']
//	menu_raw= 󰐻 [ Conversations ] [ Network ] [ Channels ] [ Log ] [ Interfaces ] [ Config ] [ Guide ] [ Quit ]
//
// The leading " 󰐻" is the nerd decoration_menu glyph (" "+U+F043B). Directory
// and Map are deliberately NOT top-level entries.
func TestMenuStructureMatchesPython(t *testing.T) {
	t.Parallel()

	wantLabels := []string{
		"Conversations", "Network", "Channels", "Log",
		"Interfaces", "Config", "Guide", "Quit",
	}
	wantKeys := []string{
		"conversations", "network", "channels", "log",
		"interfaces", "config", "guide", "quit",
	}

	if len(MenuItems) != len(wantLabels) {
		t.Fatalf("MenuItems len = %d, want %d (%v)", len(MenuItems), len(wantLabels), MenuItems)
	}
	for i, want := range wantLabels {
		if MenuItems[i].Label != want {
			t.Errorf("MenuItems[%d].Label = %q, want %q", i, MenuItems[i].Label, want)
		}
		if MenuItems[i].Key != wantKeys[i] {
			t.Errorf("MenuItems[%d].Key = %q, want %q", i, MenuItems[i].Key, wantKeys[i])
		}
	}

	// Directory and Map must not appear as top-level menu items.
	for _, mi := range MenuItems {
		if mi.Label == "Directory" || mi.Label == "Map" || mi.Key == "directory" || mi.Key == "map" {
			t.Errorf("Directory/Map must not be top-level menu items; found %+v", mi)
		}
	}

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphNerd)

	// Raw menu bar text with color tags stripped, matching summary.py menu_raw.
	got := md.menuBar.GetText(true)

	// nerd decoration_menu glyph = " " + U+F043B, then one dividechars space,
	// then the bracketed buttons separated by single spaces (Main.py:206).
	indicator := glyphsNerd["decoration_menu"]
	bracketed := make([]string, len(wantLabels))
	for i, label := range wantLabels {
		bracketed[i] = "[ " + label + " ]"
	}
	want := indicator + " " + strings.Join(bracketed, " ")

	if got != want {
		t.Errorf("menu bar raw text mismatch:\n got: %q\nwant: %q", got, want)
	}

	// Every button must be rendered with surrounding "[ " ... " ]" brackets.
	for _, label := range wantLabels {
		if !strings.Contains(got, "[ "+label+" ]") {
			t.Errorf("menu bar missing bracketed label %q in %q", label, got)
		}
	}
	// The menu-indicator glyph must lead the menu bar.
	if !strings.HasPrefix(got, indicator) {
		t.Errorf("menu bar missing leading indicator glyph %q; got %q", indicator, got)
	}
}

// TestMenuStructureHideGuide asserts the hide_guide config branch (Main.py:201)
// drops only the Guide button, leaving 7 items in the same relative order.
func TestMenuStructureHideGuide(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphNerd)
	md.SetHideGuide(true)

	wantLabels := []string{
		"Conversations", "Network", "Channels", "Log",
		"Interfaces", "Config", "Quit",
	}
	if len(md.menuItems) != len(wantLabels) {
		t.Fatalf("menuItems len = %d, want %d (%v)", len(md.menuItems), len(wantLabels), md.menuItems)
	}
	for i, want := range wantLabels {
		if md.menuItems[i].Label != want {
			t.Errorf("menuItems[%d].Label = %q, want %q", i, md.menuItems[i].Label, want)
		}
	}
	for _, mi := range md.menuItems {
		if mi.Label == "Guide" {
			t.Errorf("Guide should be hidden; found %+v", mi)
		}
	}
}

// TestMainDisplaySelectPage verifies SelectPage switches the active body page
// by menu key (the programmatic equivalent of clicking a menu button), and is
// a no-op for an unknown key.
func TestMainDisplaySelectPage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.SelectPage("network")
	if md.activePage != "network" {
		t.Errorf("after SelectPage(network): activePage = %q, want network", md.activePage)
	}
	md.SelectPage("conversations")
	if md.activePage != "conversations" {
		t.Errorf("after SelectPage(conversations): activePage = %q, want conversations", md.activePage)
	}
	// Unknown key is a no-op.
	md.SelectPage("does-not-exist")
	if md.activePage != "conversations" {
		t.Errorf("after unknown SelectPage: activePage = %q, want conversations", md.activePage)
	}
}
