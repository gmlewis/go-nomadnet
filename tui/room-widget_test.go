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

func TestNewRoomWidget(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	if rw == nil {
		t.Fatal("NewRoomWidget returned nil")
	}
	if rw.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestRoomWidgetSendMessage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	var sent string
	rw.OnSendMessage = func(text string) { sent = text }

	rw.editor.SetText("Hello world")
	rw.sendMessage()

	if sent != "Hello world" {
		t.Errorf("sendMessage content = %q, want %q", sent, "Hello world")
	}
	if rw.editor.GetText() != "" {
		t.Error("sendMessage should clear editor")
	}
}

func TestRoomWidgetSendMessageEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	sent := false
	rw.OnSendMessage = func(text string) { sent = true }
	rw.sendMessage()

	if sent {
		t.Error("Empty message should not be sent")
	}
}

func TestRoomWidgetSendMessageWhitespaceOnly(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	sent := false
	rw.OnSendMessage = func(text string) { sent = true }
	rw.editor.SetText("   ")
	rw.sendMessage()

	if sent {
		t.Error("Whitespace-only message should not be sent")
	}
}

func TestRoomWidgetSendMessageHubDisconnected(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	rw.hubConnected = false

	var sent string
	rw.OnSendMessage = func(text string) { sent = text }
	rw.editor.SetText("Hello")
	rw.sendMessage()

	if sent != "/connect" {
		t.Errorf("disconnected hub should send /connect, got %q", sent)
	}
}

func TestRoomWidgetSendMessageTooLong(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	rw.maxMessageBytes = 10

	var splitText string
	rw.OnSplitDialog = func(text string, limit int) { splitText = text }

	longMsg := "This is a very long message that exceeds the byte limit"
	rw.editor.SetText(longMsg)
	rw.sendMessage()

	if splitText != longMsg {
		t.Errorf("long message should trigger split dialog, got %q", splitText)
	}
}

func TestRoomWidgetSetHubConnected(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	if !rw.HubConnected() {
		t.Error("HubConnected should default to true")
	}

	rw.SetHubConnected(false)
	if rw.HubConnected() {
		t.Error("SetHubConnected(false) should make HubConnected return false")
	}
}

func TestRoomWidgetSetMaxMessageBytes(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	if rw.MaxMessageBytes() != 350 {
		t.Errorf("MaxMessageBytes = %d, want 350", rw.MaxMessageBytes())
	}

	rw.SetMaxMessageBytes(500)
	if rw.MaxMessageBytes() != 500 {
		t.Errorf("MaxMessageBytes = %d, want 500", rw.MaxMessageBytes())
	}
}

func TestRoomWidgetKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	var fired []string
	rw.OnSendMessage = func(text string) { fired = append(fired, "send:"+text) }
	rw.OnLeaveRoom = func() { fired = append(fired, "leave") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-d-sends", tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone), ""},
		{"ctrl-x-leaves", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "leave"},
		{"ctrl-u-toggles", tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := rw.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %s was not consumed", tt.name)
			}
			if tt.want != "" {
				if len(fired) != 1 || fired[0] != tt.want {
					t.Errorf("key %s fired %v, want [%s]", tt.name, fired, tt.want)
				}
			}
		})
	}
}

func TestRoomWidgetSetMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	msgs := []ChannelMessage{
		{Nick: "Alice", Text: "Hello", IsSelf: false},
		{Nick: "Bob", Text: "Hi", IsSelf: true},
		{Text: "User joined", IsSystem: true},
	}

	rw.SetMessages(msgs)
	if len(rw.chatMessages) != 3 {
		t.Errorf("SetMessages: got %d, want 3", len(rw.chatMessages))
	}
}

func TestRoomWidgetSetMembers(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	members := []ChannelMember{
		{Nick: "Alice", Online: true},
		{Nick: "Bob", Online: false},
	}

	rw.SetMembers(members)
	if len(rw.members) != 2 {
		t.Errorf("SetMembers: got %d, want 2", len(rw.members))
	}
}
