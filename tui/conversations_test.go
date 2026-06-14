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

	"github.com/rivo/tview"
)

func TestNewConversationsDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	convs := []ConversationInfo{
		{DisplayName: "Alice", TrustLevel: "trusted", Unread: true, LastTime: time.Now()},
		{DisplayName: "Bob", TrustLevel: "unknown", Unread: false, LastTime: time.Now().Add(-time.Hour)},
	}

	cd := NewConversationsDisplay(app, convs)
	if cd == nil {
		t.Fatal("NewConversationsDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestConversationsDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	_, ok := cd.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestConversationsDisplayEmpty(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, []ConversationInfo{})

	if cd == nil {
		t.Fatal("NewConversationsDisplay with empty list returned nil")
	}
}

func TestNewComposeDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewComposeDisplay(app)

	if cd == nil {
		t.Fatal("NewComposeDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestComposeDisplayGetSetText(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewComposeDisplay(app)

	cd.editor.SetText("Hello World")
	if cd.GetText() != "Hello World" {
		t.Errorf("GetText() = %q, want %q", cd.GetText(), "Hello World")
	}

	cd.title.SetText("Alice")
	if cd.GetTitle() != "Alice" {
		t.Errorf("GetTitle() = %q, want %q", cd.GetTitle(), "Alice")
	}
}

func TestComposeDisplayClear(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewComposeDisplay(app)

	cd.editor.SetText("test")
	cd.title.SetText("test")
	cd.Clear()

	if cd.GetText() != "" {
		t.Errorf("GetText() after clear = %q, want empty", cd.GetText())
	}
	if cd.GetTitle() != "" {
		t.Errorf("GetTitle() after clear = %q, want empty", cd.GetTitle())
	}
}

func TestNewMessageViewDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	mvd := NewMessageViewDisplay(app)

	if mvd == nil {
		t.Fatal("NewMessageViewDisplay returned nil")
	}
	if mvd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestMessageViewShowMessage(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	mvd := NewMessageViewDisplay(app)

	msg := MessageInfo{
		Title:      "Test",
		Content:    "Hello World",
		Sender:     "Alice",
		Timestamp:  "2024-01-01",
		TrustLevel: "trusted",
	}

	// Should not panic
	mvd.ShowMessage(msg)
}

func TestMessageViewClear(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	mvd := NewMessageViewDisplay(app)

	mvd.ShowMessage(MessageInfo{Content: "test"})
	mvd.Clear()
	// Should not panic
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input time.Time
		want  string
	}{
		{time.Now(), "just now"},
		{time.Now().Add(-30 * time.Second), "just now"},
		{time.Now().Add(-5 * time.Minute), "5m ago"},
		{time.Now().Add(-3 * time.Hour), "3h ago"},
		{time.Now().Add(-25 * time.Hour), "yesterday"},
		{time.Now().Add(-3 * 24 * time.Hour), "3d ago"},
	}

	for _, tt := range tests {
		got := relativeTime(tt.input)
		if got != tt.want {
			t.Errorf("relativeTime(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLooksLikeMicron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"plain text", false},
		{">>Heading", true},
		{"```code```", true},
		{"`!bold`", true},
		{"Hello World", false},
	}

	for _, tt := range tests {
		got := looksLikeMicron(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeMicron(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRenderMicronAsText(t *testing.T) {
	t.Parallel()

	result := renderMicronAsText("plain text")
	if result != "plain text" {
		t.Errorf("renderMicronAsText plain = %q, want %q", result, "plain text")
	}
}
