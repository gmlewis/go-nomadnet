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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestNewConversationWidget(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")
	if cw == nil {
		t.Fatal("NewConversationWidget returned nil")
	}
	if cw.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestConversationWidgetEmptySource(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "")
	if cw == nil {
		t.Fatal("NewConversationWidget returned nil")
	}
}

func TestConversationWidgetSetMessages(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	now := time.Now()
	msgs := []ConversationMessage{
		{Content: "Hello", Timestamp: now, IsSent: true},
		{Content: "Hi there", Timestamp: now.Add(time.Minute), IsSent: false},
		{Content: "How are you?", Timestamp: now.Add(2 * time.Minute), IsSent: true, IsFailed: true},
	}

	cw.SetMessages(msgs)
	if len(cw.messages) != 3 {
		t.Errorf("SetMessages: got %d messages, want 3", len(cw.messages))
	}
}

func TestConversationWidgetClearEditor(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")
	cw.ClearEditor()
	if cw.editor.GetText() != "" {
		t.Error("ClearEditor should clear content editor")
	}
}

func TestConversationWidgetSendMessage(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	var sentContent string
	cw.OnSend = func(content, title string) {
		sentContent = content
	}

	cw.editor.SetText("Hello world")
	cw.sendMessage()

	if sentContent != "Hello world" {
		t.Errorf("sendMessage content = %q, want %q", sentContent, "Hello world")
	}
	if cw.editor.GetText() != "" {
		t.Error("sendMessage should clear editor")
	}
}

func TestConversationWidgetSendMessageEmpty(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	sent := false
	cw.OnSend = func(content, title string) { sent = true }
	cw.sendMessage()

	if sent {
		t.Error("Empty message should not be sent")
	}
}

func TestConversationWidgetKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	var fired []string
	cw.OnClose = func() { fired = append(fired, "close") }
	cw.OnPurgeFailed = func() { fired = append(fired, "purge") }
	cw.OnClearHistory = func() { fired = append(fired, "clear") }
	cw.OnAttach = func() { fired = append(fired, "attach") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-w", tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone), "close"},
		{"ctrl-u", tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone), "purge"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "clear"},
		{"ctrl-t", tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModNone), ""},
		{"ctrl-o", tcell.NewEventKey(tcell.KeyCtrlO, 0, tcell.ModNone), ""},
		{"ctrl-a", tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone), "attach"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := cw.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %s was not consumed", tt.name)
			}
			if tt.want == "" {
				if len(fired) != 0 {
					t.Errorf("key %s should not fire callbacks, fired %v", tt.name, fired)
				}
			} else {
				if len(fired) != 1 || fired[0] != tt.want {
					t.Errorf("key %s fired %v, want [%s]", tt.name, fired, tt.want)
				}
			}
		})
	}
}

func TestConversationWidgetToggleEditor(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	if cw.fullEditorActive {
		t.Error("Editor should start in minimal mode")
	}

	cw.toggleEditor()
	if !cw.fullEditorActive {
		t.Error("toggleEditor should activate full editor")
	}

	cw.toggleEditor()
	if cw.fullEditorActive {
		t.Error("toggleEditor again should return to minimal editor")
	}
}

func TestConversationWidgetRenderMessages(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cw := NewConversationWidget(app, "aabb1122")

	cw.SetMessages(nil)
	text := cw.messageList.GetText(false)
	if text == "" {
		t.Error("Empty message list should show placeholder")
	}

	now := time.Now()
	cw.SetMessages([]ConversationMessage{
		{Content: "Hello", Timestamp: now, IsSent: true},
		{Content: "Hi", Timestamp: now.Add(time.Minute), IsSent: false, HasAttach: true, AttachCount: 2},
	})
	text = cw.messageList.GetText(false)
	if text == "" {
		t.Error("Message list should have content after SetMessages")
	}
}
