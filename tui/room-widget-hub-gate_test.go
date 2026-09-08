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

// TestShowRoomRebuildsWidgetOnHubChange verifies the composer's connected-gate
// follows the hub being viewed: two hubs can expose the same room name, and
// reusing the old RoomWidget across a hub switch left its hubStatusFn bound to
// the previous hub — a dead hub there silently swallowed every plain-message
// send while slash commands (which bypass the gate) kept working.
func TestShowRoomRebuildsWidgetOnHubChange(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{
		&fakeHub{name: "Dead Hub", addressHex: "aaaa", status: hubStatusDisconnected, joined: []string{"general"}},
		&fakeHub{name: "Live Hub", addressHex: "bbbb", status: hubStatusConnected, joined: []string{"general"}},
	})

	cd.ShowRoom(0, "general", nil)
	w1 := cd.roomWidget
	if w1 == nil {
		t.Fatal("ShowRoom did not create a room widget")
	}
	if w1.hubAddress != "aaaa" {
		t.Fatalf("widget hubAddress = %q, want %q", w1.hubAddress, "aaaa")
	}

	// Switching to a different hub exposing the SAME room name must rebuild
	// the widget so the gate reads the new hub's status.
	cd.ShowRoom(1, "general", nil)
	w2 := cd.roomWidget
	if w2 == nil {
		t.Fatal("ShowRoom on second hub returned no widget")
	}
	if w2 == w1 {
		t.Fatal("room widget not rebuilt after hub switch on same-named room: hubStatusFn stays bound to the previous hub")
	}
	if w2.hubAddress != "bbbb" {
		t.Fatalf("widget hubAddress = %q, want %q", w2.hubAddress, "bbbb")
	}

	// Switching back rebuilds again (identity is the hub, not the name).
	cd.ShowRoom(0, "general", nil)
	if cd.roomWidget == w2 {
		t.Fatal("room widget not rebuilt when switching back to the first hub")
	}
	if cd.roomWidget.hubAddress != "aaaa" {
		t.Fatalf("widget hubAddress = %q, want %q", cd.roomWidget.hubAddress, "aaaa")
	}
}

// TestSendMessageDisconnectedGateKeepsDraftWithNotice verifies the composer's
// disconnected gate keeps the draft (Python Channels.py:873-876) but tells the
// user why nothing was transmitted, instead of silently swallowing Enter.
func TestSendMessageDisconnectedGateKeepsDraftWithNotice(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "Dead Hub", "general")

	var connected int
	rw.hubStatusFn = func() int { return hubStatusDisconnected }
	rw.OnConnectHub = func() { connected++ }

	rw.editor.SetText("hello")
	rw.sendMessage()

	if got := rw.editor.GetText(); got != "hello" {
		t.Errorf("draft = %q, want it kept", got)
	}
	if connected != 1 {
		t.Errorf("OnConnectHub called %v times, want 1", connected)
	}
	if n := len(rw.chatMessages); n != 1 {
		t.Fatalf("chatMessages = %v, want 1 notice", n)
	}
	notice := rw.chatMessages[0]
	if !notice.IsError {
		t.Errorf("notice IsError = false, want true")
	}
	if !strings.Contains(notice.Text, "Dead Hub") {
		t.Errorf("notice %q does not name the disconnected hub", notice.Text)
	}
}

// TestSendMessageConnectedGateSendsWithoutNotice verifies the connected path is
// unchanged: the message goes out, the editor clears, and no gate notice is
// appended.
func TestSendMessageConnectedGateSendsWithoutNotice(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "Live Hub", "general")

	var sent string
	rw.hubStatusFn = func() int { return hubStatusConnected }
	rw.OnConnectHub = func() { t.Error("OnConnectHub must not fire on a connected hub") }
	rw.OnSendMessage = func(text string) { sent = text }

	rw.editor.SetText("hello")
	rw.sendMessage()

	if sent != "hello" {
		t.Errorf("sent = %q, want %q", sent, "hello")
	}
	if got := rw.editor.GetText(); got != "" {
		t.Errorf("editor = %q, want cleared", got)
	}
	if n := len(rw.chatMessages); n != 0 {
		t.Errorf("chatMessages = %v, want 0 (no gate notice on connected hub)", n)
	}
}
