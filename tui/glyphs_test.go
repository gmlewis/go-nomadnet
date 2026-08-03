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
	"runtime"
	"testing"
)

// Golden glyph values captured verbatim from Python GLYPHS in
// nomadnet/ui/TextUI.py:140-172. Each row is (name, plain, unicode, nerd).
// The nerd unread/unread_menu glyphs are platform-dependent
// (TextUI.py:133-138): Darwin uses U+F0E0, other platforms U+F003.

type glyphCase struct {
	name    string
	plain   string
	unicode string
	nerd    string
}

func TestGlyphGolden(t *testing.T) {
	t.Parallel()

	// Platform-dependent nerd envelope glyph (TextUI.py:133-138).
	nerdUnread := " "
	nerdUnreadMenu := " "
	if runtime.GOOS != "darwin" {
		nerdUnread = " "
		nerdUnreadMenu = " "
	}

	cases := []glyphCase{
		{"check", "=", "✓", "✓"},
		{"cross", "X", "✕", "✕"},
		{"unknown", "?", "?", "?"},
		{"encrypted", "", "⚿", ""},
		{"plaintext", "!", "!", " "},
		{"arrow_r", "->", "→", "→"},
		{"arrow_l", "<-", "←", "←"},
		{"arrow_u", "/\\", "↑", "↑"},
		{"arrow_d", "\\/", "↓", "↓"},
		{"warning", "!", "⚠", ""},
		{"info", "i", "ℹ", "\U000f064e"},
		{"unread", "[!]", "✉", nerdUnread},
		{"divider1", "-", "┄", "┄"},
		{"peer", "[P]", "Ⓟ ", ""},
		{"node", "[N]", "Ⓝ ", "\U000f0002"},
		{"page", "", "▤ ", " "},
		{"speed", "", "◷ ", "\U000f04c5 "},
		{"decoration_menu", " +", " +", " \U000f043b"},
		{"unread_menu", " !", " ✉", nerdUnreadMenu},
		{"globe", "", "", ""},
		{"sent", "/\\", "↑", "\U000f0cd8"},
		{"papermsg", "P", "▤", ""},
		{"qrcode", "QR", "▤", ""},
		{"selected", "[*] ", "●", "●"},
		{"unselected", "[ ] ", "○", "○"},
		{"file", "[F]", "▤", ""},
		{"image", "[I]", "▣", ""},
		{"audio", "[~]", "♫", ""},
		{"pin", "*", "★", ""},
		{"copy", "[C]", "⧉", ""},
	}

	sets := []struct {
		name string
		pick func(c glyphCase) string
	}{
		{GlyphPlain, func(c glyphCase) string { return c.plain }},
		{GlyphUnicode, func(c glyphCase) string { return c.unicode }},
		{GlyphNerd, func(c glyphCase) string { return c.nerd }},
	}

	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.name] = true
		for _, s := range sets {
			got := Glyph(s.name, c.name)
			want := s.pick(c)
			if got != want {
				t.Errorf("Glyph(%q, %q) = %q, want %q", s.name, c.name, got, want)
			}
		}
	}

	// Key-set parity: every registered set must contain exactly the golden
	// glyph names, no more, no less.
	for _, s := range sets {
		gs := GetGlyphSet(s.name)
		if len(gs) != len(seen) {
			t.Errorf("glyph set %q has %v entries, want %v", s.name, len(gs), len(seen))
		}
		for name := range gs {
			if !seen[name] {
				t.Errorf("glyph set %q has extra entry %q not in Python GLYPHS", s.name, name)
			}
		}
	}
}

func TestGlyphUnknown(t *testing.T) {
	t.Parallel()

	if got := Glyph(GlyphPlain, "nonexistent"); got != "" {
		t.Errorf("Glyph(plain, nonexistent) = %q, want empty", got)
	}
	if got := Glyph("bogus-set", "check"); got != "" {
		t.Errorf("Glyph(bogus-set, check) = %q, want empty", got)
	}
}
