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

import "testing"

// Golden shortcut-bar text captured verbatim from the Python original:
//
//	Conversations.py:64-80  — three bars (list/editor/body)
//	Network.py:13-25        — NetworkDisplayShortcuts
//	Channels.py:217-229     — three bars (list/editor/body)
//	Interfaces.py:3194      — InterfaceDisplayShortcuts
//	Log.py:14, Config.py:10, Guide.py:13 — empty
const (
	shortcutConversationsList   = "[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o] Sort  [C-p] My LXMF  [C-g] Fullscreen"
	shortcutConversationsEditor = "[C-d] Send  [C-p] Paper Msg  [C-t] Title  [C-f] Attach  [C-s] Save  [Tab] ↑ Messages"
	shortcutConversationsBody   = "[C-s] Save  [C-u] Purge  [C-o] Sort  [C-x] Clear History  [C-g] Fullscreen  [C-w] Close  [Tab] ↓ Editor"

	shortcutNetwork = "[C-l] Nodes/Announces  [C-x] Remove  [C-w] Disconnect  [C-d] Back  [C-f] Forward  [C-r] Reload  [C-u] URL  [C-g] Fullscreen  [C-s / C-b] Save Node"

	shortcutChannelsList   = "[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove"
	shortcutChannelsEditor = "[C-d] Send  [C-x] Leave  [F8] Collapse  [Tab] Complete Nick"
	shortcutChannelsBody   = "[C-x] Leave  [C-u] Users  [C-y] Channels  [F8] Collapse Joins  [Tab] ↓ Editor"

	shortcutInterfaces = "[C-a] Add Interface [C-e] Edit Interface [C-x] Remove Interface [Enter] Show Interface [C-w] Open Text Editor"
)

// TestShortcutBarPerPage asserts the footer follows the DISPLAYED page (Python
// Main.update_active_shortcuts), not a single hardcoded display. A page with a
// registered dynamic callback (Conversations) uses the callback; others use
// their static SetShortcut text. This is the fix for the unconditional
// conversationsDisplay.GetShortcutText() that previously overrode every page.
func TestShortcutBarPerPage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)

	md.SetShortcut("network", shortcutNetwork)
	md.SetShortcut("interfaces", shortcutInterfaces)
	md.SetShortcut("log", "")
	md.SetShortcut("conversations", shortcutConversationsList)
	// Conversations supplies its bar dynamically (it switches by focus region).
	md.SetShortcutCallback("conversations", func() string {
		return shortcutConversationsEditor
	})

	cases := []struct {
		page string
		want string
	}{
		{"network", shortcutNetwork},
		{"interfaces", shortcutInterfaces},
		{"log", ""},
		// Dynamic callback wins over the static SetShortcut for this page.
		{"conversations", shortcutConversationsEditor},
		// A page with no static text and no callback shows an empty bar.
		{"guide", ""},
	}

	for _, c := range cases {
		c := c
		t.Run(c.page, func(t *testing.T) {
			// Sub-tests share md (mutate activePage); run sequentially.
			md.mu.Lock()
			md.activePage = c.page
			md.contentArea.SwitchToPage(c.page)
			md.updateShortcutsLocked()
			md.mu.Unlock()

			if got := md.shortcutBar.GetText(true); got != c.want {
				t.Errorf("page %q footer = %q, want %q", c.page, got, c.want)
			}
		})
	}
}

// TestConversationsShortcutBars asserts GetShortcutText returns the three
// Python bars (Conversations.py:64-80) keyed by focus region: list, editor,
// body. An open dialog suppresses the bar (returns "").
func TestConversationsShortcutBars(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cases := []struct {
		region string
		want   string
	}{
		{"list", shortcutConversationsList},
		{"editor", shortcutConversationsEditor},
		{"body", shortcutConversationsBody},
	}
	for _, c := range cases {
		c := c
		t.Run(c.region, func(t *testing.T) {
			// Sub-tests share cd (mutate shortcutFocus); run sequentially.
			cd.SetShortcutFocus(c.region)
			if got := cd.GetShortcutText(); got != c.want {
				t.Errorf("region %q = %q, want %q", c.region, got, c.want)
			}
		})
	}

	// While a dialog is open the shortcut bar is suppressed.
	cd.dialogOpen = true
	if got := cd.GetShortcutText(); got != "" {
		t.Errorf("open dialog shortcut = %q, want empty", got)
	}
}
