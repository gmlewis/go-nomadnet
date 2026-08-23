// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestMicronViewBaseColor pins the MicronViewDisplay base text color to the
// micron plain default #dddddd (6-hex exact). Python's MicronParser uses
// DEFAULT_FG_DARK = "ddd" → high_color("ddd") = "#dddddd" (nibble-doubled
// to 6-hex, parsed exact by urwid). The Go port previously used 0xbbbbbb
// which Python never emits.
//
// Python source: MicronParser.py:12-14,19 (DEFAULT_FG_DARK="ddd", STYLES_DARK
// "plain" fg="ddd"); MicronParser.py:556-567 (high_color nibble-doubles).
func TestMicronViewBaseColor(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	mvd := NewMicronViewDisplay(app)
	if mvd == nil {
		t.Fatal("NewMicronViewDisplay returned nil")
	}
	mvd.view.SetText("X")

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(20, 3)
	mvd.view.SetRect(0, 0, 20, 3)
	mvd.view.Draw(screen)

	if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
		t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if got := uint32(fg.Hex()) & 0xffffff; got != 0xdddddd {
			t.Errorf("micron-view base fg = #%06x, want #dddddd (micron plain default)", got)
		}
	}
}

// TestLinkableTextBaseColor pins the LinkableText base text color to the
// micron plain default #dddddd (6-hex exact). Python's LinkableText is a
// bare urwid.Text whose parts carry the synthesized micron_ddd_default_
// style (#dddddd dark / #222222 light). The Go port previously used
// 0xbbbbbb which Python never emits.
//
// Python source: MicronParser.py:866,405,424-425 (LinkableText uses
// make_style(state) → micron_ddd_default_ = #dddddd dark).
func TestLinkableTextBaseColor(t *testing.T) {
	t.Parallel()

	lt := NewLinkableText(nil)
	if lt == nil {
		t.Fatal("NewLinkableText returned nil")
	}
	lt.SetText("X")

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(20, 3)
	lt.SetRect(0, 0, 20, 3)
	lt.Draw(screen)

	if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
		t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if got := uint32(fg.Hex()) & 0xffffff; got != 0xdddddd {
			t.Errorf("linkable-text base fg = #%06x, want #dddddd (micron plain default)", got)
		}
	}
}

// TestBrowserControlsFallback pins the browserControlsColor fallback to
// the cube-quantized browser_controls value (#bbb → #afafaf), not the
// prior nibble-doubled 0xbbbbbb. This fallback is hit only when the theme
// lookup yields ColorDefault (test-only); the primary path already uses
// GetThemeColors.
//
// Python source: TextUI.py:53 (browser_controls = #bbb dark, 3-hex).
func TestBrowserControlsFallback(t *testing.T) {
	t.Parallel()

	// With a nil app, the fallback path is hit.
	got := browserControlsColor(nil)
	if uint32(got.Hex())&0xffffff != 0xafafaf {
		t.Errorf("browserControlsColor(nil) = #%06x, want #afafaf "+
			"(browser_controls #bbb cube-quantized)",
			uint32(got.Hex())&0xffffff)
	}
}

// TestMsgViewBaseColor pins the MessageViewDisplay base text color to
// #dddddd (micron plain default, 6-hex exact). Python has no standalone
// "message view" widget; messages are inline LXMessageWidget with
// msg_header_* styles and bare Text content. The closest defensible base
// is the micron plain default #dddddd. The Go port previously used
// 0xbbbbbb which Python never emits.
//
// Python source: Conversations.py:2576 (LXMessageWidget), 2737 (bare
// urwid.Text for plain content); MicronParser.py:12 (DEFAULT_FG_DARK).
func TestMsgViewBaseColor(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	mvd := NewMessageViewDisplay(app)
	if mvd == nil {
		t.Fatal("NewMessageViewDisplay returned nil")
	}
	mvd.view.SetText("X")

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(20, 5)
	mvd.view.SetRect(0, 0, 20, 3)
	mvd.view.Draw(screen)

	if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
		t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if got := uint32(fg.Hex()) & 0xffffff; got != 0xdddddd {
			t.Errorf("msgview base fg = #%06x, want #dddddd (micron plain default)", got)
		}
	}
}
