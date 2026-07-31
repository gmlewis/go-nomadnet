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

// TestLinkifyMOTDPythonParity verifies LinkifyMOTD against Python's
// _linkify_motd (Channels.py:1231), which substitutes #room references with
// the Micron link form `[`#room`room://room]` using the _MOTD_ROOM_RE pattern
// (Channels.py:1229):
//
//	(?<!\[)(?<!\w)#([A-Za-z0-9][A-Za-z0-9_\-]{0,62})
//
// The two lookbehinds reject a `#` that is preceded by `[` (so already-linked
// rooms are not re-linkified) or by a word character (so `word#room` is left
// alone). Go's regexp lacks lookarounds, so LinkifyMOTD replicates them with a
// manual scan. Expected values were captured from /tmp/linkify_ref.py.
func TestLinkifyMOTDPythonParity(t *testing.T) {
	t.Parallel()

	a63 := "" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 63 a's
	a64 := a63 + "a"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain text", "Welcome to the hub", "Welcome to the hub"},
		{"single room", "Join #general for chat", "Join `[#general`room://general] for chat"},
		{"two rooms", "Rooms: #general and #random",
			"Rooms: `[#general`room://general] and `[#random`room://random]"},
		{"already linked left alone",
			"Already linked `[#foo`room://foo] stay",
			"Already linked `[#foo`room://foo] stay"},
		{"word before hash rejected", "word#notaroom", "word#notaroom"},
		{"multibyte before space", "café #room", "café `[#room`room://room]"},
		{"room at start", "#room-at-start", "`[#room-at-start`room://room-at-start]"},
		{"room at end", "end with #room", "end with `[#room`room://room]"},
		{"underscore and hyphen names",
			"underscore #my_room and hyphen #my-room",
			"underscore `[#my_room`room://my_room] and hyphen `[#my-room`room://my-room]"},
		{"trailing digits", "number #room123", "number `[#room123`room://room123]"},
		{"leading digit name", "#123starts", "`[#123starts`room://123starts]"},
		{"lone hash no name", "no room # here", "no room # here"},
		{"bare hash", "#", "#"},
		{"single char room", "#a", "`[#a`room://a]"},
		{"63 char room", "#" + a63, "`[#" + a63 + "`room://" + a63 + "]"},
		{"64 char room truncates to 63", "#" + a64,
			"`[#" + a63 + "`room://" + a63 + "]a"},
		{"trailing punctuation", "trailing #room!", "trailing `[#room`room://room]!"},
		{"in parens", "parens (#general) here", "parens (`[#general`room://general]) here"},
		{"bracket already linked", "bracket [#general] already", "bracket [#general] already"},
		{"double hash links second", "double ##double",
			"double #`[#double`room://double]"},
		{"mixed case preserved", "Mixed #General CASE",
			"Mixed `[#General`room://General] CASE"},
		{"dot stops room name", "dot #room.test stops",
			"dot `[#room`room://room].test stops"},
		{"newline before room", "newlines\n#room\nhere",
			"newlines\n`[#room`room://room]\nhere"},
		{"tab before room", "tab\t#room", "tab\t`[#room`room://room]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := LinkifyMOTD(tt.input)
			if got != tt.want {
				t.Errorf("LinkifyMOTD(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
