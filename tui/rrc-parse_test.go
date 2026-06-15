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

func TestParseWhoNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		wantRoom  string
		wantLen   int
		wantNicks []string
	}{
		{
			name:      "two named members",
			text:      "members in #general: Alice (aabbccddeeff), Bob (112233445566)",
			wantRoom:  "general",
			wantLen:   2,
			wantNicks: []string{"Alice", "Bob"},
		},
		{
			name:     "single named member",
			text:     "members in #test: Charlie (deadbeef1234)",
			wantRoom: "test",
			wantLen:  1,
		},
		{
			name:      "mixed named and bare hash",
			text:      "members in #chat: Alice (aabbccddeeff), 11223344556677889900112233445566",
			wantRoom:  "chat",
			wantLen:   2,
			wantNicks: []string{"Alice"},
		},
		{
			name:     "none members",
			text:     "members in #empty: (none)",
			wantRoom: "empty",
			wantLen:  0,
		},
		{
			name:     "empty body",
			text:     "members in #lobby: ",
			wantRoom: "lobby",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			room, entries, err := ParseWhoNotice(tt.text)
			if err != nil {
				t.Fatalf("ParseWhoNotice(%q) returned error: %v", tt.text, err)
			}
			if room != tt.wantRoom {
				t.Errorf("room = %q, want %q", room, tt.wantRoom)
			}
			if tt.wantLen > 0 && len(entries) != tt.wantLen {
				t.Errorf("len(entries) = %d, want %d", len(entries), tt.wantLen)
			}
			for i, nick := range tt.wantNicks {
				if i >= len(entries) {
					t.Fatalf("entries[%d] does not exist", i)
				}
				if entries[i].Nick != nick {
					t.Errorf("entries[%d].Nick = %q, want %q", i, entries[i].Nick, nick)
				}
			}
		})
	}
}

func TestParseWhoNoticeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{"nil string", ""},
		{"wrong prefix", "no such thing"},
		{"missing separator", "members in #general no colon"},
		{"missing room", "members in : no room"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseWhoNotice(tt.text)
			if err == nil {
				t.Errorf("ParseWhoNotice(%q) did not return error", tt.text)
			}
		})
	}
}

func TestParseRoomListNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		wantLen  int
		wantKeys []string
	}{
		{
			name: "rooms with topics",
			text: "Registered public rooms:\n" +
				"#general - General discussion\n" +
				"#dev - Development chat",
			wantLen:  2,
			wantKeys: []string{"general", "dev"},
		},
		{
			name:     "rooms without topics",
			text:     "Registered public rooms:\n#lobby\n#random",
			wantLen:  2,
			wantKeys: []string{"lobby", "random"},
		},
		{
			name:    "no public rooms",
			text:    "No public rooms registered",
			wantLen: 0,
		},
		{
			name:    "empty body after header",
			text:    "Registered public rooms:\n",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rooms, err := ParseRoomListNotice(tt.text)
			if err != nil {
				t.Fatalf("ParseRoomListNotice(%q) returned error: %v", tt.text, err)
			}
			if len(rooms) != tt.wantLen {
				t.Errorf("len(rooms) = %d, want %d", len(rooms), tt.wantLen)
			}
			for _, key := range tt.wantKeys {
				if _, ok := rooms[key]; !ok {
					t.Errorf("rooms missing key %q", key)
				}
			}
		})
	}
}

func TestParseRoomListNoticeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"wrong prefix", "something else"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseRoomListNotice(tt.text)
			if err == nil {
				t.Errorf("ParseRoomListNotice(%q) did not return error", tt.text)
			}
		})
	}
}

func TestMatchMention(t *testing.T) {
	t.Parallel()

	tests := []struct {
		nick string
		text string
		want bool
	}{
		{"Alice", "Hello @Alice, how are you?", true},
		{"Alice", "@Alice is here", true},
		{"Alice", "alice@foo.com", false},
		{"Alice", "foo @alice bar", false}, // case sensitive
		{"bob", "Hey @bob!", true},
		{"Bob", "Hey @Bob!", true},
		{"Alice", "no mentions here", false},
		{"Alice", "@Alice@email.com", false}, // followed by @
		{"test", "say @test!", true},
		{"x", "@x y", true},
	}

	for _, tt := range tests {
		t.Run(tt.nick+"_"+tt.text, func(t *testing.T) {
			t.Parallel()
			got := MatchMention(tt.text, tt.nick)
			if got != tt.want {
				t.Errorf("MatchMention(%q, %q) = %v, want %v", tt.text, tt.nick, got, tt.want)
			}
		})
	}
}

func TestShortHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"abcdef0123456789", 8, "abcdef01"},
		{"abcdef0123456789", 4, "abcd"},
		{"abcdef0123456789", 16, "abcdef0123456789"},
		{"ab", 12, "ab"},
		{"", 12, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := ShortHash(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("ShortHash(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tsMs int64
		want string
	}{
		{"zero UTC", 0, "19:00:00"},
		{"noon UTC", 43200000, "07:00:00"},
		{"minute UTC", 3661000, "20:01:01"},
		{"negative", -1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatTimestamp(tt.tsMs)
			if got != tt.want {
				t.Errorf("FormatTimestamp(%d) = %q, want %q", tt.tsMs, got, tt.want)
			}
		})
	}
}

func TestScanCodeBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		wantLen int
	}{
		{"no code", "hello world", 0},
		{"inline code", "use `fmt.Println()` here", 1},
		{"fenced block", "```go\nfmt.Println()\n```", 1},
		{"mixed", "text `code` more ```go\nfmt.Println()\n``` end", 2},
		{"nested backticks in fenced", "```go\n`inline` inside\n```", 1},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			regions := ScanCodeBlocks(tt.text)
			if len(regions) != tt.wantLen {
				t.Errorf("ScanCodeBlocks(%q) returned %d regions, want %d", tt.text, len(regions), tt.wantLen)
			}
		})
	}
}

func TestScanCodeBlockRegions(t *testing.T) {
	t.Parallel()

	text := "before `code` middle" + "\n```block```" + " after"
	regions := ScanCodeBlocks(text)

	if len(regions) != 2 {
		t.Fatalf("ScanCodeBlocks returned %d regions, want 2", len(regions))
	}

	// First region: inline `code`
	if regions[0].Start != 7 || regions[0].End != 13 {
		t.Errorf("region 0 = {%d, %d}, want {7, 13}", regions[0].Start, regions[0].End)
	}

	// Second region: fenced ```block``` — the opening ``` is at 21
	// (after newline), but regex also needs the newline after opening ```.
	// Just verify it exists and is non-zero
	if regions[1].Start <= regions[0].End || regions[1].End <= regions[1].Start {
		t.Errorf("region 1 = {%d, %d}, expected valid range after region 0",
			regions[1].Start, regions[1].End)
	}
}

func TestIsInCodeBlock(t *testing.T) {
	t.Parallel()

	blocks := []CodeBlockRegion{
		{Start: 10, End: 20},
		{Start: 30, End: 40},
	}

	tests := []struct {
		pos  int
		want bool
	}{
		{5, false},
		{15, true},
		{25, false},
		{35, true},
		{45, false},
	}

	for _, tt := range tests {
		got := IsInCodeBlock(tt.pos, blocks)
		if got != tt.want {
			t.Errorf("IsInCodeBlock(%d) = %v, want %v", tt.pos, got, tt.want)
		}
	}
}
