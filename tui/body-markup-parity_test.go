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

// TestBodyMarkupPythonParity is a LIVE cross-implementation check: it execs
// Python's nomadnet.ui.textui.Channels._body_markup (Channels.py:152) with
// check_links=False — the branch that emits plain string styles
// ("body_text"/"irc_mention"/"nick_mention"/"link_<kind>") which is what Go's
// BodyMarkup produces — and derives the expected span sequence freshly on
// every run. Go owns the input battery; Python owns the reference behavior.
// The test SKIPs, not fails, when the Python reference is not importable.
//
// Each case records the exact sequence of (style, text) spans and whether the
// body contains links. Cases cover self-mentions ("irc_mention"), other-nick
// mentions ("nick_mention"), the three link kinds, code-block exclusion of
// links and mentions, repeated self-mentions (which require byte-exact span
// positions, not a re-search), repeated links, and a multibyte prefix.
func TestBodyMarkupPythonParity(t *testing.T) {
	t.Parallel()

	const h32 = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	type bodyInput struct {
		Body    string `json:"body"`
		OwnNick string `json:"own_nick"`
	}
	cases := []struct {
		name    string
		body    string
		ownNick string
	}{
		{"plain", "Hello world", ""},
		{"self mention", "Hey @alice!", "alice"},
		{"self and other", "@alice and @bob", "alice"},
		{"other only", "Hey @bob", "alice"},
		{"room link", "Join #general", ""},
		{"page link", "Visit " + h32, ""},
		{"lxmf link", "msg lxmf@" + h32, ""},
		{"mix all", "hi @alice see #room and " + h32, "alice"},
		{"code excludes link", "see `" + h32 + "` code", ""},
		{"code excludes mention", "`@alice` code", "alice"},
		{"two self mentions", "@alice hi @alice", "alice"},
		{"link appears twice", h32 + " then " + h32, ""},
		{"multibyte", "café @alice #room " + h32, "alice"},
	}

	inputs := make([]bodyInput, len(cases))
	for i, c := range cases {
		inputs[i] = bodyInput{Body: c.body, OwnNick: c.ownNick}
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
inputs = json.load(sys.stdin)
out = []
for inp in inputs:
    body = inp["body"]
    own = inp.get("own_nick") or None
    spans, has_links = C._body_markup(body, "body_text", own_nick=own, check_links=False)
    row = {"spans": [{"style": s, "text": t} for s, t in spans], "has_links": has_links}
    out.append(row)
json.dump(out, sys.stdout)
`

	var want []bodyMarkupLiveWant
	runPythonNomadnet(t, inputs, script, &want)

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var spans []StyledSpan
			var hasLinks bool
			if c.ownNick == "" {
				spans, hasLinks = BodyMarkup(c.body, ThemeDark)
			} else {
				spans, hasLinks = BodyMarkup(c.body, ThemeDark, c.ownNick)
			}
			w := want[i]
			if hasLinks != w.HasLinks {
				t.Errorf("hasLinks = %v, want %v (Python)", hasLinks, w.HasLinks)
			}
			if len(spans) != len(w.Spans) {
				t.Fatalf("BodyMarkup(%q) got %v spans, want %v (Python):\n  got=%+v\n  want=%+v",
					c.body, len(spans), len(w.Spans), spans, w.Spans)
			}
			for j, ws := range w.Spans {
				g := spans[j]
				if g.Style != ws.Style {
					t.Errorf("span[%v].Style = %q, want %q (Python)", j, g.Style, ws.Style)
				}
				if g.Text != ws.Text {
					t.Errorf("span[%v].Text = %q, want %q (Python)", j, g.Text, ws.Text)
				}
			}
		})
	}
}

type bodyMarkupLiveWant struct {
	Spans    []bodyMarkupSpan `json:"spans"`
	HasLinks bool             `json:"has_links"`
}

type bodyMarkupSpan struct {
	Style string `json:"style"`
	Text  string `json:"text"`
}
