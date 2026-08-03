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

// TestScanLinksPythonParity verifies ScanLinks against Python's _scan_links
// (Channels.py:75) using _LINK_RE (Channels.py:60). Expected values were
// captured from the Python source run in /tmp/scan_ref.py. Each expectation
// records the kind, the link target, and the exact matched substring
// (text[Start:End]) so the test is independent of byte-vs-code-point index
// conventions. These cases exercise the lookbehind/lookahead boundaries that
// Go's regexp (which lacks lookarounds) must replicate manually: a page link
// preceded by '@' is suppressed, and a page link hidden inside a rejected
// room token ("pre#<hash>") is still found.
func TestScanLinksPythonParity(t *testing.T) {
	t.Parallel()

	const h32 = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	const a63 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tests := []struct {
		name string
		text string
		want []linkWant
	}{
		{"lxmf link", "Message me at lxmf@" + h32, []linkWant{{"lxmf", h32, "lxmf@" + h32}}},
		{"room link", "Join #general for chat", []linkWant{{"room", "general", "#general"}}},
		{"room with underscores", "Try #my_room-name", []linkWant{{"room", "my_room-name", "#my_room-name"}}},
		{"page address", "Visit " + h32 + " for info", []linkWant{{"page", h32, h32}}},
		{"multiple links", "See lxmf@deadbeefdeadbeefdeadbeefdeadbeef and #test-room", []linkWant{
			{"lxmf", "deadbeefdeadbeefdeadbeefdeadbeef", "lxmf@deadbeefdeadbeefdeadbeefdeadbeef"},
			{"room", "test-room", "#test-room"},
		}},
		{"no links", "Just plain text here", nil},
		{"empty", "", nil},
		{"room at start", "#general is active", []linkWant{{"room", "general", "#general"}}},
		{"room after word char rejected", "foo#bar", nil},
		{"lxmf after word char rejected", "not_lxmf@" + h32, nil},
		{"page with path", h32 + ":path/to/page", []linkWant{{"page", h32 + ":path/to/page", h32 + ":path/to/page"}}},
		{"page preceded by at rejected", "@" + h32, nil},
		{"page inside rejected room", "pre#" + h32, []linkWant{{"page", h32, h32}}},
		{"single char room", "#x", []linkWant{{"room", "x", "#x"}}},
		{"room max length 63", "#" + a63, []linkWant{{"room", a63, "#" + a63}}},
		{"room truncates at 63", "#" + a63 + "a", []linkWant{{"room", a63, "#" + a63}}},
		{"multibyte before page", "café " + h32, []linkWant{{"page", h32, h32}}},
		{"room stops at non-ascii", "room #café-test", []linkWant{{"room", "caf", "#caf"}}},
		{"mix mention and links", "mix @alice and " + h32 + " plus #room", []linkWant{
			{"page", h32, h32},
			{"room", "room", "#room"},
		}},
		{"fenced code no links", "```\ncode @alice\n```", nil},
		{"inline code no links", "inline `code @bob` here", nil},
		{"overlapping code and room", "overlapping `code` and #room", []linkWant{{"room", "room", "#room"}}},
		{"64 hex no link", h32 + h32, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScanLinks(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("ScanLinks(%q) got %v links, want %v: %+v", tt.text, len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Kind != w.kind {
					t.Errorf("link[%v].Kind = %q, want %q", i, g.Kind, w.kind)
				}
				if g.Target != w.target {
					t.Errorf("link[%v].Target = %q, want %q", i, g.Target, w.target)
				}
				match := ""
				if g.Start >= 0 && g.End <= len(tt.text) && g.Start <= g.End {
					match = tt.text[g.Start:g.End]
				}
				if match != w.match {
					t.Errorf("link[%v] match = %q (text[%v:%v]), want %q", i, match, g.Start, g.End, w.match)
				}
			}
		})
	}
}

// TestScanCodeBlocksPythonParity verifies ScanCodeBlocks against Python's
// _scan_code_blocks (Channels.py:139). Expected regions were captured from
// the Python source run in /tmp/scan_ref.py. The inline-backtick regex in
// Python uses a (?<!`) lookbehind so a backtick immediately preceded by
// another backtick is not treated as an opener; Go's regexp lacks
// lookarounds, so the scanner must replicate the check manually. The
// double-backtick and "```fence```" cases exercise this.
func TestScanCodeBlocksPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want [][2]int
	}{
		{"no code", "hello world", nil},
		{"inline code", "use `fmt.Println()` here", [][2]int{{4, 19}}},
		{"two inline", "text `x` more `y` end", [][2]int{{5, 8}, {14, 17}}},
		{"fenced block", "```\ncode @alice\n```", [][2]int{{0, 19}}},
		{"fenced with lang", "fence ```py\nprint(1)\n``` tail", [][2]int{{6, 24}}},
		{"inline in code", "inline `code @bob` here", [][2]int{{7, 18}}},
		{"inline and room", "overlapping `code` and #room", [][2]int{{12, 18}}},
		{"empty", "", nil},
		{"double backtick rejected", "``x``", nil},
		{"empty fence", "```\n```", [][2]int{{0, 7}}},
		{"backtick before inline", "`a``b`", [][2]int{{0, 3}}},
		{"two then space backticks", "`` ``", nil},
		{"space backtick space", "a ` ` b", [][2]int{{2, 5}}},
		{"triple then inline", "```fence``` and `inline`", [][2]int{{16, 24}}},
		{"unclosed", "`unclosed", nil},
		{"double unclosed", "``unclosed``", nil},
		{"triple around letter", "x```y```z", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScanCodeBlocks(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("ScanCodeBlocks(%q) got %v regions, want %v: %+v", tt.text, len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Start != w[0] || g.End != w[1] {
					t.Errorf("region[%v] = {%v,%v}, want {%v,%v} (%q)", i, g.Start, g.End, w[0], w[1], tt.text[w[0]:w[1]])
				}
			}
		})
	}
}

// linkWant is an expected ScanLinks result for parity testing.
type linkWant struct {
	kind   string
	target string
	match  string
}

// TestScanMentionsPythonParity verifies ScanMentions (self-mentions) and
// scanNickMentions (other-nick mentions) against Python's _scan_mentions and
// _scan_nick_mentions (Channels.py:124,131). Expected matched substrings were
// captured from /tmp/scan_ref.py. Python _scan_mentions yields only matches of
// the own nick; ScanMentions must return exactly those (self) matches.
func TestScanMentionsPythonParity(t *testing.T) {
	t.Parallel()

	selfCases := []struct {
		text string
		want []string // matched substrings text[Start:End]
	}{
		{"Hey @alice, how are you?", []string{"@alice"}},
		{"Hey @bob here", nil},
		{"@alice and @bob", []string{"@alice"}},
		{"not@alice here", nil},
		{"Hey @Alice", []string{"@Alice"}},
		{"@ALICE!", []string{"@ALICE"}},
		{"mail@alice.com", nil},
		{"café @alice", []string{"@alice"}},
		{"@alice_2", nil},
		{"@ali", nil},
	}
	for _, tc := range selfCases {
		t.Run("self/"+tc.text, func(t *testing.T) {
			t.Parallel()
			got := ScanMentions(tc.text, "alice")
			if len(got) != len(tc.want) {
				t.Fatalf("ScanMentions(%q, alice) got %v, want %v: %+v", tc.text, len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				match := ""
				if got[i].Start >= 0 && got[i].End <= len(tc.text) {
					match = tc.text[got[i].Start:got[i].End]
				}
				if match != w {
					t.Errorf("mention[%v] = %q, want %q", i, match, w)
				}
				if !got[i].IsSelf {
					t.Errorf("mention[%v] IsSelf = false, want true", i)
				}
			}
		})
	}

	nickCases := []struct {
		text string
		want []nickWant
	}{
		{"Hey @alice and @bob", []nickWant{{"@bob", "bob"}}},
		{"@alice @bob @carol", []nickWant{{"@bob", "bob"}, {"@carol", "carol"}}},
		{"not@bob", nil},
		{"@bob_2 hi", []nickWant{{"@bob_2", "bob_2"}}},
		{"café @bob", []nickWant{{"@bob", "bob"}}},
	}
	for _, tc := range nickCases {
		t.Run("nick/"+tc.text, func(t *testing.T) {
			t.Parallel()
			got := scanNickMentions(tc.text, "alice")
			if len(got) != len(tc.want) {
				t.Fatalf("scanNickMentions(%q, alice) got %v, want %v: %+v", tc.text, len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				match := ""
				if got[i].start >= 0 && got[i].end <= len(tc.text) {
					match = tc.text[got[i].start:got[i].end]
				}
				if match != w.match {
					t.Errorf("nickMention[%v] = %q, want %q", i, match, w.match)
				}
				if got[i].nick != w.nick {
					t.Errorf("nickMention[%v].nick = %q, want %q", i, got[i].nick, w.nick)
				}
			}
		})
	}
}

type nickWant struct {
	match string
	nick  string
}
