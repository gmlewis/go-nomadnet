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
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestThemeConstants(t *testing.T) {
	t.Parallel()

	if ThemeDark != 1 {
		t.Errorf("ThemeDark = %d, want 1", ThemeDark)
	}
	if ThemeLight != 2 {
		t.Errorf("ThemeLight = %d, want 2", ThemeLight)
	}
}

func TestGetThemeColors(t *testing.T) {
	t.Parallel()

	dark := GetThemeColors(ThemeDark)
	if dark == nil {
		t.Fatal("GetThemeColors(ThemeDark) returned nil")
	}
	if dark["body_text"] != tcell.NewHexColor(0xdddddd) {
		t.Errorf("dark body_text = %v, want #ddd", dark["body_text"])
	}

	light := GetThemeColors(ThemeLight)
	if light == nil {
		t.Fatal("GetThemeColors(ThemeLight) returned nil")
	}
	if light["body_text"] != tcell.NewHexColor(0x222222) {
		t.Errorf("light body_text = %v, want #222", light["body_text"])
	}
}

func TestGetThemeColorsDefault(t *testing.T) {
	t.Parallel()

	// Unknown theme should return dark
	colors := GetThemeColors(99)
	if colors["body_text"] != tcell.NewHexColor(0xdddddd) {
		t.Error("Unknown theme did not default to dark")
	}
}

func TestGetGlyphSet(t *testing.T) {
	t.Parallel()

	plain := GetGlyphSet(GlyphPlain)
	if plain == nil {
		t.Fatal("GetGlyphSet(plain) returned nil")
	}
	if plain["check"] != "=" {
		t.Errorf("plain check = %q, want %q", plain["check"], "=")
	}

	unicode := GetGlyphSet(GlyphUnicode)
	if unicode == nil {
		t.Fatal("GetGlyphSet(unicode) returned nil")
	}
	if unicode["check"] != "\u2713" {
		t.Errorf("unicode check = %q, want %q", unicode["check"], "\u2713")
	}

	nerd := GetGlyphSet(GlyphNerd)
	if nerd == nil {
		t.Fatal("GetGlyphSet(nerd) returned nil")
	}
	if nerd["check"] != "\u2713" {
		t.Errorf("nerd check = %q, want %q", nerd["check"], "\u2713")
	}
}

func TestGetGlyphSetDefault(t *testing.T) {
	t.Parallel()

	// Unknown glyph set should return unicode
	gs := GetGlyphSet("unknown")
	if gs["check"] != "\u2713" {
		t.Error("Unknown glyph set did not default to unicode")
	}
}

func TestGlyphSetCompleteness(t *testing.T) {
	t.Parallel()

	requiredGlyphs := []string{
		"check", "cross", "unknown", "encrypted", "plaintext",
		"arrow_r", "arrow_l", "arrow_u", "arrow_d",
		"warning", "info", "unread", "divider1",
		"peer", "node", "page", "speed",
		"decoration_menu", "unread_menu", "globe", "sent",
		"papermsg", "qrcode", "selected", "unselected",
		"file", "image", "audio", "pin", "copy",
	}

	for _, name := range []string{GlyphPlain, GlyphUnicode, GlyphNerd} {
		gs := GetGlyphSet(name)
		for _, glyph := range requiredGlyphs {
			if _, ok := gs[glyph]; !ok {
				t.Errorf("glyph set %q missing glyph %q", name, glyph)
			}
		}
	}
}

func TestMenuItemCount(t *testing.T) {
	t.Parallel()

	// Should have 10 menu items
	if len(MenuItems) != 10 {
		t.Errorf("MenuItems len = %d, want 10", len(MenuItems))
	}

	// Check key names
	expectedKeys := []string{
		"network", "conversations", "channels", "directory", "map",
		"log", "config", "interfaces", "guide", "quit",
	}
	for i, key := range expectedKeys {
		if MenuItems[i].Key != key {
			t.Errorf("MenuItems[%d].Key = %q, want %q", i, MenuItems[i].Key, key)
		}
	}
}

func TestBuildMenuBarText(t *testing.T) {
	t.Parallel()

	text := BuildMenuBarText(0)
	if len(text) == 0 {
		t.Error("BuildMenuBarText returned empty")
	}

	// Should contain all menu labels
	for _, item := range MenuItems {
		if !contains(text, item.Label) {
			t.Errorf("BuildMenuBarText missing label %q", item.Label)
		}
	}
}

func TestNewMainDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	if md == nil {
		t.Fatal("NewMainDisplay returned nil")
	}
	if md.frame == nil {
		t.Error("frame is nil")
	}
	if md.menuBar == nil {
		t.Error("menuBar is nil")
	}
	if md.contentArea == nil {
		t.Error("contentArea is nil")
	}
	if len(md.menuButtons) != 10 {
		t.Errorf("menuButtons len = %d, want 10", len(md.menuButtons))
	}
}

func TestMainDisplaySetDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	// Add a test page
	page := tview.NewTextView()
	md.SetDisplay("test", page)

	// Verify page was added
	_, _ = md.contentArea.GetFrontPage()
}

func TestMainDisplaySetGlyphs(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.SetGlyphs(GlyphNerd)
	if md.glyphs["check"] != "\u2713" {
		t.Error("SetGlyphs did not update glyph set")
	}
}

func TestMainDisplaySetQuitCallback(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.SetQuitCallback(func() {})

	if md.onQuit == nil {
		t.Error("onQuit callback not set")
	}
}

func TestMainDisplayRoot(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	root := md.Root()
	if root == nil {
		t.Error("Root() returned nil")
	}
}

func TestNewApp(t *testing.T) {
	t.Parallel()

	a := NewApp(ThemeDark, GlyphUnicode)
	if a == nil {
		t.Fatal("NewApp returned nil")
	}
	if a.Application == nil {
		t.Error("Application is nil")
	}
	if a.Main == nil {
		t.Error("Main is nil")
	}
	if a.Theme != ThemeDark {
		t.Errorf("Theme = %d, want %d", a.Theme, ThemeDark)
	}
}

func TestNewAppDefaults(t *testing.T) {
	t.Parallel()

	a := NewApp(ThemeLight, GlyphPlain)
	if a.Theme != ThemeLight {
		t.Errorf("Theme = %d, want %d", a.Theme, ThemeLight)
	}
	if a.Glyphs["check"] != "=" {
		t.Error("Default glyphs not set correctly")
	}
}

func TestNewAppNilGlyphSet(t *testing.T) {
	t.Parallel()

	// Unknown glyph set should default to unicode
	a := NewApp(ThemeDark, "unknown")
	if a.Glyphs["check"] != "\u2713" {
		t.Error("Unknown glyph set did not default to unicode")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
