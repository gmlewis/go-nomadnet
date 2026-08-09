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

func TestFormatChannelMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		msg      ChannelMessage
		wantBody string
		wantType string // "normal", "self", "system", "notice", "error"
	}{
		{
			name:     "normal message",
			msg:      ChannelMessage{Nick: "Alice", Text: "Hello world"},
			wantBody: "Hello world",
			wantType: "normal",
		},
		{
			name:     "self message",
			msg:      ChannelMessage{Nick: "Bob", Text: "I am here", IsSelf: true},
			wantBody: "I am here",
			wantType: "self",
		},
		{
			name:     "system message",
			msg:      ChannelMessage{Text: "User joined", IsSystem: true},
			wantBody: "User joined",
			wantType: "system",
		},
		{
			name:     "notice message",
			msg:      ChannelMessage{Text: "Topic changed", IsNotice: true},
			wantBody: "Topic changed",
			wantType: "notice",
		},
		{
			name:     "error message",
			msg:      ChannelMessage{Text: "Connection lost", IsError: true},
			wantBody: "Connection lost",
			wantType: "error",
		},
		{
			name:     "mention message",
			msg:      ChannelMessage{Nick: "Alice", Text: "@Bob hi", Mention: true},
			wantBody: "@Bob hi",
			wantType: "normal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatChannelMessage(tt.msg, ThemeDark)
			// Verify the message contains the text
			if !strings.Contains(got, tt.wantBody) {
				t.Errorf("FormatChannelMessage() = %q, missing text %q", got, tt.wantBody)
			}
			// Verify color tags are present
			switch tt.wantType {
			case "self":
				if !strings.Contains(got, "[green]") {
					t.Errorf("FormatChannelMessage() = %q, missing [green] tag", got)
				}
			case "system":
				if !strings.Contains(got, "[gray]") {
					t.Errorf("FormatChannelMessage() = %q, missing [gray] tag", got)
				}
			case "notice":
				if !strings.Contains(got, "[yellow]") {
					t.Errorf("FormatChannelMessage() = %q, missing [yellow] tag", got)
				}
			case "error":
				if !strings.Contains(got, "[red]") {
					t.Errorf("FormatChannelMessage() = %q, missing [red] tag", got)
				}
			}
		})
	}
}

func TestFormatConversationItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		conv     ConversationInfo
		wantText string
		wantSec  string
	}{
		{
			name:     "trusted unread",
			conv:     ConversationInfo{DisplayName: "Alice", TrustLevel: "trusted", Unread: true, LastMessage: "Hi"},
			wantText: "[!] ● Alice",
		},
		{
			name:     "trusted read",
			conv:     ConversationInfo{DisplayName: "Bob", TrustLevel: "trusted", Unread: false, LastMessage: "Bye"},
			wantText: "  ● Bob",
		},
		{
			name:     "untrusted",
			conv:     ConversationInfo{DisplayName: "Eve", TrustLevel: "untrusted"},
			wantText: "  × Eve",
		},
		{
			name:     "failed",
			conv:     ConversationInfo{DisplayName: "Mallory", TrustLevel: "trusted", Failed: true},
			wantText: "[x] ● Mallory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			text, _ := FormatConversationItem(tt.conv, ThemeDark)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestBodyMarkupPlain(t *testing.T) {
	t.Parallel()

	spans, hasLinks := BodyMarkup("Hello world", ThemeDark)
	if hasLinks {
		t.Error("plain text should not have links")
	}
	if len(spans) != 1 {
		t.Fatalf("got %v spans, want 1", len(spans))
	}
	if spans[0].Kind != "text" {
		t.Errorf("span.Kind = %q, want %q", spans[0].Kind, "text")
	}
	if spans[0].Text != "Hello world" {
		t.Errorf("span.Text = %q, want %q", spans[0].Text, "Hello world")
	}
}

func TestBodyMarkupWithLink(t *testing.T) {
	t.Parallel()

	spans, hasLinks := BodyMarkup("See #general for chat", ThemeDark)
	if !hasLinks {
		t.Error("should detect link")
	}
	if len(spans) < 2 {
		t.Fatalf("got %v spans, want >= 2", len(spans))
	}
	// Find the link span
	found := false
	for _, s := range spans {
		if s.Kind == "link" {
			found = true
			if s.Target != "general" {
				t.Errorf("link target = %q, want %q", s.Target, "general")
			}
			if s.Style != "link_room" {
				t.Errorf("link style = %q, want %q", s.Style, "link_room")
			}
		}
	}
	if !found {
		t.Error("no link span found")
	}
}

func TestBodyMarkupWithMention(t *testing.T) {
	t.Parallel()

	spans, _ := BodyMarkup("Hey @alice!", ThemeDark, "alice")
	// Should have a self-mention span
	found := false
	for _, s := range spans {
		if s.Kind == "mention" && s.Text == "@alice" {
			found = true
			if s.Style != "irc_mention" {
				t.Errorf("mention style = %q, want %q", s.Style, "irc_mention")
			}
		}
	}
	if !found {
		t.Error("no self-mention span found")
	}
}

func TestBodyMarkupWithNickMention(t *testing.T) {
	t.Parallel()

	spans, _ := BodyMarkup("Hey @bob!", ThemeDark, "alice")
	// Python's _body_markup styles other-nick mentions (not the own nick)
	// as "nick_mention", distinct from the "irc_mention" self-mention style.
	found := false
	for _, s := range spans {
		if s.Kind == "nick_mention" && s.Text == "@bob" {
			found = true
			if s.Style != "nick_mention" {
				t.Errorf("nick_mention style = %q, want %q", s.Style, "nick_mention")
			}
		}
	}
	if !found {
		t.Error("no nick_mention span found for @bob")
	}
}

func TestBodyMarkupWithCodeBlock(t *testing.T) {
	t.Parallel()

	spans, _ := BodyMarkup("Text before `code` and after", ThemeDark)
	// Code blocks don't create spans — they only exclude overlapping spans
	// So plain code with no links/mentions returns a single text span
	if len(spans) != 1 {
		t.Fatalf("got %v spans, want 1", len(spans))
	}
	if spans[0].Kind != "text" {
		t.Errorf("span.Kind = %q, want %q", spans[0].Kind, "text")
	}
	if !strings.Contains(spans[0].Text, "Text before") {
		t.Errorf("span.Text missing content: %q", spans[0].Text)
	}
}

func TestBodyMarkupCodeBlockExcludesMention(t *testing.T) {
	t.Parallel()

	// Mentions inside code blocks should not be highlighted as mentions
	spans, _ := BodyMarkup("`@alice`", ThemeDark, "alice")
	for _, s := range spans {
		if s.Kind == "mention" {
			t.Error("@alice inside code block should not be a mention span")
		}
	}
}

func TestFormatHubEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hub      *HubEntry
		wantText string
	}{
		{
			name: "connected hub",
			hub: &HubEntry{
				Name:   "My Hub",
				Status: HubConnected,
				Rooms: map[string]*HubRoom{
					"general": {Name: "general", Joined: true, Unread: true},
					"random":  {Name: "random", Joined: false},
				},
			},
			wantText: "● My Hub",
		},
		{
			name: "disconnected hub",
			hub: &HubEntry{
				Name:   "Test Hub",
				Status: HubDisconnected,
				Rooms:  map[string]*HubRoom{},
			},
			wantText: "○ Test Hub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			text := FormatHubEntry(tt.hub)
			if text != tt.wantText {
				t.Errorf("FormatHubEntry() = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestFormatHubRoom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		room     HubRoom
		wantText string
	}{
		{
			name:     "joined with unread",
			room:     HubRoom{Name: "general", Joined: true, Unread: true},
			wantText: "  [!] #general",
		},
		{
			name:     "joined no unread",
			room:     HubRoom{Name: "general", Joined: true, Unread: false},
			wantText: "  [*] #general",
		},
		{
			name:     "not joined",
			room:     HubRoom{Name: "random", Joined: false},
			wantText: "  #random",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			text := FormatHubRoom(tt.room)
			if text != tt.wantText {
				t.Errorf("FormatHubRoom() = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestFormatMemberStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		member   ChannelMember
		wantText string
		wantSec  string
	}{
		{
			name:     "online member",
			member:   ChannelMember{Nick: "Alice", Hash: "abc123def456", Online: true},
			wantText: "● Alice",
			wantSec:  "abc123def456",
		},
		{
			name:     "offline member",
			member:   ChannelMember{Nick: "Bob", Hash: "789012ghijkl", Online: false},
			wantText: "○ Bob",
			wantSec:  "789012ghijkl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			text, sec := FormatMemberStatus(tt.member)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if sec != tt.wantSec {
				t.Errorf("sec = %q, want %q", sec, tt.wantSec)
			}
		})
	}
}
