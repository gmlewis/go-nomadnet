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
	"reflect"
	"testing"
)

// TestBodyMarkupPythonParity verifies BodyMarkup against Python's
// _body_markup (Channels.py:152). Expected span styles and texts were
// captured from /tmp/body_markup_ref.py. Each case records the exact
// sequence of (style, text) spans and whether the body contains links.
// Cases cover self-mentions ("irc_mention"), other-nick mentions
// ("nick_mention"), the three link kinds, code-block exclusion of links
// and mentions, repeated self-mentions (which require byte-exact span
// positions, not a re-search), repeated links, and a multibyte prefix.
func TestBodyMarkupPythonParity(t *testing.T) {
	t.Parallel()

	const h32 = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	tests := []struct {
		name     string
		body     string
		ownNick  string
		hasLinks bool
		want     []spanExpect
	}{
		{"plain", "Hello world", "", false, []spanExpect{{"body_text", "Hello world"}}},
		{"self mention", "Hey @alice!", "alice", false, []spanExpect{
			{"body_text", "Hey "}, {"irc_mention", "@alice"}, {"body_text", "!"},
		}},
		{"self and other", "@alice and @bob", "alice", false, []spanExpect{
			{"irc_mention", "@alice"}, {"body_text", " and "}, {"nick_mention", "@bob"},
		}},
		{"other only", "Hey @bob", "alice", false, []spanExpect{
			{"body_text", "Hey "}, {"nick_mention", "@bob"},
		}},
		{"room link", "Join #general", "", true, []spanExpect{
			{"body_text", "Join "}, {"link_room", "#general"},
		}},
		{"page link", "Visit " + h32, "", true, []spanExpect{
			{"body_text", "Visit "}, {"link_page", h32},
		}},
		{"lxmf link", "msg lxmf@" + h32, "", true, []spanExpect{
			{"body_text", "msg "}, {"link_lxmf", "lxmf@" + h32},
		}},
		{"mix all", "hi @alice see #room and " + h32, "alice", true, []spanExpect{
			{"body_text", "hi "}, {"irc_mention", "@alice"},
			{"body_text", " see "}, {"link_room", "#room"},
			{"body_text", " and "}, {"link_page", h32},
		}},
		{"code excludes link", "see `" + h32 + "` code", "", false, []spanExpect{
			{"body_text", "see `" + h32 + "` code"},
		}},
		{"code excludes mention", "`@alice` code", "alice", false, []spanExpect{
			{"body_text", "`@alice` code"},
		}},
		{"two self mentions", "@alice hi @alice", "alice", false, []spanExpect{
			{"irc_mention", "@alice"}, {"body_text", " hi "}, {"irc_mention", "@alice"},
		}},
		{"link appears twice", h32 + " then " + h32, "", true, []spanExpect{
			{"link_page", h32}, {"body_text", " then "}, {"link_page", h32},
		}},
		{"multibyte", "café @alice #room " + h32, "alice", true, []spanExpect{
			{"body_text", "café "}, {"irc_mention", "@alice"},
			{"body_text", " "}, {"link_room", "#room"},
			{"body_text", " "}, {"link_page", h32},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var spans []StyledSpan
			var hasLinks bool
			if tt.ownNick == "" {
				spans, hasLinks = BodyMarkup(tt.body, ThemeDark)
			} else {
				spans, hasLinks = BodyMarkup(tt.body, ThemeDark, tt.ownNick)
			}
			if hasLinks != tt.hasLinks {
				t.Errorf("hasLinks = %v, want %v", hasLinks, tt.hasLinks)
			}
			got := make([]spanExpect, len(spans))
			for i, s := range spans {
				got[i] = spanExpect{s.Style, s.Text}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BodyMarkup(%q) spans =\n  %v\nwant\n  %v", tt.body, got, tt.want)
			}
		})
	}
}

type spanExpect struct {
	style string
	text  string
}
