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
)

// tabMenubarApp wires a test app with a real MainDisplay so the FocusMenu
// transition can be asserted.
func tabMenubarApp(t *testing.T) (*App, *MainDisplay) {
	t.Helper()
	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	app.Main = md
	return app, md
}

// Tab in the conversations list area must move focus to the menubar (Python
// ConversationsArea.keypress "tab" → frame.focus_position = "header",
// Conversations.py:97-98), and must NOT fire when the editor/body (right
// column) has focus — Python's ConversationsArea only sees keys while the
// list region has focus.
func TestConversationsTabInListRegionFocusesMenu(t *testing.T) {
	t.Parallel()

	app, md := tabMenubarApp(t)
	cd := NewConversationsDisplay(app, deleteSelectedModel())

	// List region: Tab → menubar.
	cd.shortcutFocus = "list"
	if got := cd.handleInput(tcell.NewEventKey(tcell.KeyTab, '\t', tcell.ModNone)); got != nil {
		t.Fatal("Tab in the list region was not consumed by the conversations page")
	}
	if md.focusRegion != "menu" {
		t.Errorf("Tab in the list region left focusRegion = %q; want \"menu\"", md.focusRegion)
	}
}

// With the conversation detail focused, Tab belongs to the conversation
// widget (toggle editor/body), so the page capture must pass it through.
func TestConversationsTabOutsideListRegionPassesThrough(t *testing.T) {
	t.Parallel()

	_, md := tabMenubarApp(t)
	cd := NewConversationsDisplay(md.app, deleteSelectedModel())
	cd.shortcutFocus = "body"
	if got := cd.handleInput(tcell.NewEventKey(tcell.KeyTab, '\t', tcell.ModNone)); got == nil {
		t.Fatal("Tab was consumed by the conversations page while the body region was focused")
	}
}

// Tab in the channels list region must move focus to the menubar (Python
// ChannelsListArea.keypress "tab" → header, Channels.py:374-375).
func TestChannelsTabInListRegionFocusesMenu(t *testing.T) {
	t.Parallel()

	app, md := tabMenubarApp(t)
	cd := NewChannelsDisplay(app, nil)
	cd.shortcutFocus = "list"
	if got := cd.handleInput(tcell.NewEventKey(tcell.KeyTab, '\t', tcell.ModNone)); got != nil {
		t.Fatal("Tab in the channels list region was not consumed")
	}
	if md.focusRegion != "menu" {
		t.Errorf("Tab in the channels list region left focusRegion = %q; want \"menu\"", md.focusRegion)
	}
}

// Outside the channels list region, Tab belongs to the room (nick completion
// / ↓ editor), so the page capture must pass it through.
func TestChannelsTabOutsideListRegionPassesThrough(t *testing.T) {
	t.Parallel()

	app, _ := tabMenubarApp(t)
	cd := NewChannelsDisplay(app, nil)
	cd.shortcutFocus = "editor"
	if got := cd.handleInput(tcell.NewEventKey(tcell.KeyTab, '\t', tcell.ModNone)); got == nil {
		t.Fatal("Tab was consumed by the channels page while the editor region was focused")
	}
}
