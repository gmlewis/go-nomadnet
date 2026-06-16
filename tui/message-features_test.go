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
)

func TestScanLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want []LinkSpan
	}{
		{
			name: "lxmf link",
			text: "Message me at lxmf@a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want: []LinkSpan{
				{Kind: "lxmf", Target: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"},
			},
		},
		{
			name: "room link",
			text: "Join #general for chat",
			want: []LinkSpan{
				{Kind: "room", Target: "general"},
			},
		},
		{
			name: "room with underscores",
			text: "Try #my_room-name",
			want: []LinkSpan{
				{Kind: "room", Target: "my_room-name"},
			},
		},
		{
			name: "page address",
			text: "Visit a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6 for info",
			want: []LinkSpan{
				{Kind: "page", Target: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"},
			},
		},
		{
			name: "multiple links",
			text: "See lxmf@deadbeefdeadbeefdeadbeefdeadbeef and #test-room",
			want: []LinkSpan{
				{Kind: "lxmf", Target: "deadbeefdeadbeefdeadbeefdeadbeef"},
				{Kind: "room", Target: "test-room"},
			},
		},
		{
			name: "no links",
			text: "Just plain text here",
			want: nil,
		},
		{
			name: "empty",
			text: "",
			want: nil,
		},
		{
			name: "room at start of line",
			text: "#general is active",
			want: []LinkSpan{
				{Kind: "room", Target: "general"},
			},
		},
		{
			name: "not room after word char",
			text: "foo#bar",
			want: nil,
		},
		{
			name: "lxmf not in word",
			text: "not_lxmf@a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScanLinks(tt.text)
			if len(got) != len(tt.want) {
				t.Errorf("got %d links, want %d", len(got), len(tt.want))
				for i, l := range got {
					t.Logf("  [%d] kind=%q target=%q", i, l.Kind, l.Target)
				}
				return
			}
			for i := range got {
				if got[i].Kind != tt.want[i].Kind {
					t.Errorf("link[%d].Kind = %q, want %q", i, got[i].Kind, tt.want[i].Kind)
				}
				if got[i].Target != tt.want[i].Target {
					t.Errorf("link[%d].Target = %q, want %q", i, got[i].Target, tt.want[i].Target)
				}
			}
		})
	}
}

func TestScanMentions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		ownNick string
		want    []MentionSpan
	}{
		{
			name:    "self mention",
			text:    "Hey @alice, how are you?",
			ownNick: "alice",
			want: []MentionSpan{
				{Nick: "alice", IsSelf: true},
			},
		},
		{
			name:    "other mention",
			text:    "Hey @bob, how are you?",
			ownNick: "alice",
			want: []MentionSpan{
				{Nick: "bob", IsSelf: false},
			},
		},
		{
			name:    "multiple mentions",
			text:    "@alice and @bob are here",
			ownNick: "alice",
			want: []MentionSpan{
				{Nick: "alice", IsSelf: true},
				{Nick: "bob", IsSelf: false},
			},
		},
		{
			name:    "no mentions",
			text:    "Hello world",
			ownNick: "alice",
			want:    nil,
		},
		{
			name:    "mention in word boundary",
			text:    "not@alice here",
			ownNick: "alice",
			want:    nil,
		},
		{
			name:    "case insensitive",
			text:    "Hey @Alice",
			ownNick: "alice",
			want: []MentionSpan{
				{Nick: "Alice", IsSelf: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScanMentions(tt.text, tt.ownNick)
			if len(got) != len(tt.want) {
				t.Errorf("got %d mentions, want %d", len(got), len(tt.want))
				for i, m := range got {
					t.Logf("  [%d] nick=%q self=%v", i, m.Nick, m.IsSelf)
				}
				return
			}
			for i := range got {
				if got[i].Nick != tt.want[i].Nick {
					t.Errorf("mention[%d].Nick = %q, want %q", i, got[i].Nick, tt.want[i].Nick)
				}
				if got[i].IsSelf != tt.want[i].IsSelf {
					t.Errorf("mention[%d].IsSelf = %v, want %v", i, got[i].IsSelf, tt.want[i].IsSelf)
				}
			}
		})
	}
}

func TestNickCompletion(t *testing.T) {
	t.Parallel()

	members := []ChannelMember{
		{Nick: "Alice", Hash: "aaaa"},
		{Nick: "Bob", Hash: "bbbb"},
		{Nick: "Charlie", Hash: "cccc"},
		{Nick: "Aria", Hash: "dddd"},
	}

	tests := []struct {
		name    string
		text    string
		cursor  int
		members []ChannelMember
		self    string
		want    []string
	}{
		{
			name:    "prefix a",
			text:    "hello @a",
			cursor:  8,
			members: members,
			self:    "alice",
			want:    []string{"Alice", "Aria"},
		},
		{
			name:    "prefix b",
			text:    "@b",
			cursor:  2,
			members: members,
			self:    "alice",
			want:    []string{"Bob"},
		},
		{
			name:    "empty prefix at start",
			text:    "@",
			cursor:  1,
			members: members,
			self:    "alice",
			want:    []string{"Alice", "Aria", "Bob", "Charlie"},
		},
		{
			name:    "no at prefix at start",
			text:    "ch",
			cursor:  2,
			members: members,
			self:    "alice",
			want:    []string{"Charlie"},
		},
		{
			name:    "exclude self",
			text:    "@al",
			cursor:  3,
			members: members,
			self:    "alice",
			want:    []string{"Alice"},
		},
		{
			name:    "no matches",
			text:    "@zzz",
			cursor:  4,
			members: members,
			self:    "alice",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CompleteNick(tt.text, tt.cursor, tt.members, tt.self)
			if len(got) != len(tt.want) {
				t.Errorf("got %d completions, want %d", len(got), len(tt.want))
				for i, c := range got {
					t.Logf("  [%d] %q", i, c)
				}
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("completion[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
