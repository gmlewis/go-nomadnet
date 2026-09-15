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

// These tests pin the same bracket-swallowing bug as the chat body
// (rrc-chat-escape_test.go) at the other RRC surfaces that render external text
// into tag-parsing widgets: the room header, the room users list, the channels
// hub/room list, the hub-info pane, and the legacy message/member views.
//
// tview.List ALWAYS parses tags in item main/secondary text, and any TextView
// with SetDynamicColors(true) parses them too. A nick, room topic, hub display
// name, advertised server name, status text, or MOTD line containing "[x]"
// would otherwise be consumed as a style tag and vanish.

// TestRoomHeaderEscapesBrackets covers the room/server/hub display names in the
// room header (a dynamic-colors TextView).
func TestRoomHeaderEscapesBrackets(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "[hub]", "[room]")
	rw.SetRoomHeader("[server]", "0.3.2", "Connected")

	got := rw.header.GetText(true)
	for _, want := range []string{"[room]", "[server]", "[hub]", "Connected"} {
		if !strings.Contains(got, want) {
			t.Errorf("room header lost %q\nheader=%q", want, got)
		}
	}
}

// TestUsersListEscapesBrackets covers the room users pane. tview.List has no
// strip-tags accessor, so assert the escaped token reached the item text: the
// fork's parser consumes a bare "[bot]", so "[bot[]" is what proves the fix.
func TestUsersListEscapesBrackets(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "general")
	rw.members = []ChannelMember{{
		Nick:   "[bot]",
		Hash:   "0123456789abcdef0123456789abcdef",
		Online: true,
	}}
	rw.renderMembers("")

	main, _ := rw.usersList.GetItemText(0)
	if !strings.Contains(main, "[bot[]") {
		t.Errorf("users-list item not escaped: %q", main)
	}
}

// TestChannelsListEscapesHubNameAndTopic covers the channels sidebar: the hub
// display-name row and the room topic in a row's secondary text.
func TestChannelsListEscapesHubNameAndTopic(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	cd.SetHubs([]HubView{fakeHub{
		name:   "[Hub A]",
		status: hubStatusConnected,
		joined: []string{"general"},
	}})
	main, _ := cd.rooms.GetItemText(0)
	if !strings.Contains(main, "[Hub A[]") {
		t.Errorf("hub row not escaped: %q", main)
	}

	cd.populateRooms([]ChannelInfo{{
		Name:    "general",
		Topic:   "talk about [x]",
		Members: 2,
	}})
	_, secondary := cd.rooms.GetItemText(0)
	if !strings.Contains(secondary, "[x[]") {
		t.Errorf("room topic not escaped: %q", secondary)
	}
}

// TestHubInfoEscapesBrackets covers the hub-info pane (a dynamic-colors
// TextView): hub name, status text, advertised server name and MOTD lines.
func TestHubInfoEscapesBrackets(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	hia := NewHubInfoArea(app, "hub")
	hia.SetHubInfo(HubInfoSnapshot{
		Name:       "[HubName]",
		Status:     hubStatusConnected,
		StatusText: "[ok]",
		ServerName: "[Server]",
		MOTD:       "welcome [friend]",
	})

	body := hia.view.GetText(true)
	for _, want := range []string{"[HubName]", "[ok]", "[Server]", "welcome [friend]"} {
		if !strings.Contains(body, want) {
			t.Errorf("hub info lost %q\nbody=%q", want, body)
		}
	}
	if got := hia.widget.GetTitle(); !strings.Contains(got, "[HubName[]") {
		t.Errorf("hub-info border title not escaped: %q", got)
	}
}

// TestShowMessagesEscapesBrackets covers the legacy channel message view, which
// embeds intentional color tags around untrusted nick and body text.
func TestShowMessagesEscapesBrackets(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.ShowMessages([]ChannelMessage{
		{Nick: "[bot]", Text: "hello [world]"},
		{Text: "system [note]", IsSystem: true},
	})

	body := cd.messages.GetText(true)
	for _, want := range []string{"[bot]", "hello [world]", "system [note]"} {
		if !strings.Contains(body, want) {
			t.Errorf("messages view lost %q\nbody=%q", want, body)
		}
	}
}
