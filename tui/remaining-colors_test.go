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
	"github.com/rivo/tview"
)

// TestBrowserPaneFallbackColor pins the browser-pane disconnected fallback to
// the cube-quantized browser_inactive value (#444 → #5f5f5f), not the prior
// nibble-doubled 0x444444.
//
// Python source: TextUI.py:30 (browser_inactive = #444, 3-hex).
func TestBrowserPaneFallbackColor(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bp := NewBrowserPane(app)
	if bp.body == nil {
		t.Fatal("browser pane body is nil")
	}
	if got := uint32(bp.body.color.Hex()) & 0xffffff; got != 0x5f5f5f {
		t.Errorf("browser-pane fallback color = #%06x, want #5f5f5f "+
			"(browser_inactive #444 cube-quantized)", got)
	}
}

// TestLXMFPeersViewTitleColor pins the LXMF peers view title to ColorDefault.
// Python wraps the peers panel in `AttrMap(LineBox(...), widget_style)` where
// widget_style is None (default) when populated (Network.py:1786). The LineBox
// title inherits default styling. The Go port previously used 0xdddddd.
func TestLXMFPeersViewTitleColor(t *testing.T) {
	t.Parallel()

	lv := NewLXMFPeersView(nil)
	if lv == nil {
		t.Fatal("NewLXMFPeersView returned nil")
	}
	lv.title.SetTextAlign(tview.AlignLeft)
	lv.title.SetText("X")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(40, 3)
	lv.title.SetRect(0, 0, 40, 3)
	lv.title.Draw(screen)
	if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
		t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
	} else {
		fg, _, _ := style.Decompose()
		if fg != tcell.ColorDefault {
			t.Errorf("LXMF peers title fg = %v, want ColorDefault "+
				"(Python default LineBox title)", fg)
		}
	}
}

// TestRoomWidgetHeaderColor pins the room header text/bg to the
// msg_header_sent palette entry. Python wraps the room header in
// `AttrMap(urwid.Text(""), "msg_header_sent")` (Channels.py:602);
// msg_header_sent is #111/#ddd (dark, 3-hex cube-quantized to
// #000000/#d7d7d7) / #111/#ddd (light, cube-quantized same). The Go port
// previously used 0xdddddd for text only, with no background.
//
// Python source: Channels.py:602; TextUI.py:35.
func TestRoomWidgetHeaderColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		theme  int
		fgWant uint32
		bgWant uint32
	}{
		{"dark", ThemeDark, 0x000000, 0xd7d7d7},
		{"light", ThemeLight, 0x000000, 0xd7d7d7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			rw := NewRoomWidget(app, "hub", "room")

			// Probe the header via draw. Override align to left so the
			// text starts at column 0.
			rw.header.SetTextAlign(tview.AlignLeft)
			rw.header.SetText("X")
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 3)
			rw.header.SetRect(0, 0, 40, 3)
			rw.header.Draw(screen)
			if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
				t.Fatalf("header cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, bg, _ := style.Decompose()
				if got := uint32(fg.Hex()) & 0xffffff; got != tc.fgWant {
					t.Errorf("header fg = #%06x, want #%06x (msg_header_sent fg)", got, tc.fgWant)
				}
				if got := uint32(bg.Hex()) & 0xffffff; got != tc.bgWant {
					t.Errorf("header bg = #%06x, want #%06x (msg_header_sent bg)", got, tc.bgWant)
				}
			}
		})
	}
}

// TestRoomWidgetUsersTitle pins the room users-panel title: Python's UsersBox
// is a `urwid.LineBox(self.users_listbox, title="Users")` (Channels.py:625) —
// the title lives IN THE BORDER with default styling, not in a title row
// inside the pane.
func TestRoomWidgetUsersTitle(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	rw := NewRoomWidget(app, "hub", "room")
	if got := rw.usersBox.GetTitle(); got != " Users " {
		t.Errorf("users box title = %q, want %q (Python UsersBox title in border)", got, " Users ")
	}
}

// TestDirectoryDisplayTitleColor pins the directory title text to body_text.
// Python's Directory.py uses body_text for its only text (Directory.py:14);
// body_text is 3-hex #ddd (dark) / #222 (light), cube-quantized to
// #d7d7d7 / #000000. The Go port previously used 0xdddddd (nibble-doubled).
func TestDirectoryDisplayTitleColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
		want  uint32
	}{
		{"dark", ThemeDark, 0xd7d7d7},
		{"light", ThemeLight, 0x000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			dd := NewDirectoryDisplay(app, nil)
			dd.title.SetTextAlign(tview.AlignLeft)
			dd.title.SetText("X")
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 3)
			dd.title.SetRect(0, 0, 40, 3)
			dd.title.Draw(screen)
			if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
				t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if got := uint32(fg.Hex()) & 0xffffff; got != tc.want {
					t.Errorf("directory title fg = #%06x, want #%06x (body_text)", got, tc.want)
				}
			}
		})
	}
}

// TestConversationsSyncStatusColor pins the sync-status footer text to the
// shortcutbar palette fg (#111 cube-quantized to #000000). Python wraps the
// sync-status line in `AttrMap(urwid.Text(...), "shortcutbar")`
// (Conversations.py:410); shortcutbar has the same colors as menubar
// (#111/#bbb, 3-hex cube-quantized to #000000/#afafaf). The Go port
// previously used 0xaaaaaa (nibble-doubled #aaa).
func TestConversationsSyncStatusColor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		theme  int
		fgWant uint32
	}{
		{"dark", ThemeDark, 0x000000},
		{"light", ThemeLight, 0x000000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			cd := NewConversationsDisplay(app, nil)
			cd.syncStatus.SetText("X")
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(40, 3)
			cd.syncStatus.SetRect(0, 0, 40, 3)
			cd.syncStatus.Draw(screen)
			if c, _, style, _ := cellContent(screen, 0, 0); c != 'X' {
				t.Fatalf("cell (0,0) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if got := uint32(fg.Hex()) & 0xffffff; got != tc.fgWant {
					t.Errorf("syncStatus fg = #%06x, want #%06x (shortcutbar fg #111)", got, tc.fgWant)
				}
			}
		})
	}
}
