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
