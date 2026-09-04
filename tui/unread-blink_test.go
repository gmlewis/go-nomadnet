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
	"time"

	"github.com/gmlewis/go-reticulum/testutils"
)

// TestUnreadIndicatorGlyph asserts updateUnreadIndicator swaps the leading
// menu-indicator glyph to unread_menu when there are unread conversations and
// back to decoration_menu when there are none — the synchronous core of the
// Python MenuDisplay.update_display job (Main.py:216-230).
//
// Uses GlyphUnicode so the glyphs are deterministic across platforms:
// decoration_menu = " +", unread_menu = " !" (glyphs.go:50-51).
func TestUnreadIndicatorGlyph(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	decoration := glyphsUnicode["decoration_menu"]
	unread := glyphsUnicode["unread_menu"]

	// Initially the decoration glyph leads the menu bar.
	if got := menuText(md); !strings.HasPrefix(got, decoration) {
		t.Errorf("initial menu prefix = %q, want decoration %q (full %q)", got[:len(decoration)], decoration, got)
	}

	// An unread conversation swaps the glyph to unread_menu.
	md.SetUnreadCheck(func() bool { return true })
	md.updateUnreadIndicator()
	if got := menuText(md); !strings.HasPrefix(got, unread) {
		t.Errorf("unread menu prefix = %q, want unread %q (full %q)", got[:len(unread)], unread, got)
	}

	// Clearing unread reverts to the decoration glyph.
	md.SetUnreadCheck(func() bool { return false })
	md.updateUnreadIndicator()
	if got := menuText(md); !strings.HasPrefix(got, decoration) {
		t.Errorf("reverted menu prefix = %q, want decoration %q (full %q)", got[:len(decoration)], decoration, got)
	}
}

// TestUnreadBlinkTick drives the blink goroutine with a fast injected ticker
// (a mocked clock) and asserts the glyph toggles on each tick — mirroring the
// 2 s UPDATE_INTERVAL loop in production without waiting for it.
func TestUnreadBlinkTick(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	unread := glyphsUnicode["unread_menu"]
	decoration := glyphsUnicode["decoration_menu"]

	md.SetUnreadCheck(func() bool { return true })
	md.startUnreadBlink(time.NewTicker(5*time.Millisecond), false)
	defer md.StopUnreadBlink()

	if !waitForPrefix(md, unread, time.Second) {
		t.Errorf("unread glyph never appeared after tick; menu = %q", menuText(md))
	}

	md.StopUnreadBlink()
	md.SetUnreadCheck(func() bool { return false })
	md.startUnreadBlink(time.NewTicker(5*time.Millisecond), false)
	defer md.StopUnreadBlink()

	if !waitForPrefix(md, decoration, time.Second) {
		t.Errorf("decoration glyph never reappeared after tick; menu = %q", menuText(md))
	}
}

// TestUnreadIndicatorNoCheck asserts that with no unread-check registered the
// indicator stays at the decoration glyph (treats unread as false, never nil-deref).
func TestUnreadIndicatorNoCheck(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.updateUnreadIndicator()
	decoration := glyphsUnicode["decoration_menu"]
	if got := menuText(md); !strings.HasPrefix(got, decoration) {
		t.Errorf("no-check menu prefix = %q, want decoration %q (full %q)", got[:len(decoration)], decoration, got)
	}
}

// menuText returns the menu bar text with color tags stripped, read under md.mu
// so it never races the blink goroutine's redrawMenuBar.
func menuText(md *MainDisplay) string {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.menuBar.GetText(true)
}

// waitForPrefix polls the menu text (under md.mu) for up to timeout for prefix.
func waitForPrefix(md *MainDisplay, prefix string, timeout time.Duration) bool {
	return testutils.PollUntil(timeout, func() bool {
		return strings.HasPrefix(menuText(md), prefix)
	})
}
