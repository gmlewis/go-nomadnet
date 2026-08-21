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

// TestLinkifyMOTDPythonParity is a LIVE cross-implementation check: it execs
// Python's real Channels._linkify_motd (nomadnet.ui.textui.Channels), which
// substitutes #room references with the Micron link form `[`#room`room://room]`
// using the _MOTD_ROOM_RE pattern:
//
//	(?<!\[)(?<!\w)#([A-Za-z0-9][A-Za-z0-9_\-]{0,62})
//
// The two lookbehinds reject a `#` that is preceded by `[` (so already-linked
// rooms are not re-linkified) or by a word character (so `word#room` is left
// alone). Go's regexp lacks lookarounds, so LinkifyMOTD replicates them with a
// manual scan; this test proves that replication matches Python's regex
// behavior across a wide battery, freshly derived on every run. Go owns the
// input battery; Python owns the reference behavior. The test SKIPs, not
// fails, when the Python reference is not importable.
func TestLinkifyMOTDPythonParity(t *testing.T) {
	t.Parallel()

	a63 := "" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 63 a's
	a64 := a63 + "a"

	inputs := []string{
		"",
		"Welcome to the hub",
		"Join #general for chat",
		"Rooms: #general and #random",
		"Already linked `[#foo`room://foo] stay",
		"word#notaroom",
		"café #room",
		"#room-at-start",
		"end with #room",
		"underscore #my_room and hyphen #my-room",
		"number #room123",
		"#123starts",
		"no room # here",
		"#",
		"#a",
		"#" + a63,
		"#" + a64,
		"trailing #room!",
		"parens (#general) here",
		"bracket [#general] already",
		"double ##double",
		"Mixed #General CASE",
		"dot #room.test stops",
		"newlines\n#room\nhere",
		"tab\t#room",
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
inputs = json.load(sys.stdin)
json.dump([C._linkify_motd(s) for s in inputs], sys.stdout)
`

	var want []string
	runPythonNomadnet(t, inputs, script, &want)

	for i, in := range inputs {
		t.Run(linkifyCaseName(in), func(t *testing.T) {
			t.Parallel()
			got := LinkifyMOTD(in)
			if got != want[i] {
				t.Errorf("LinkifyMOTD(%q) = %q, want %q (Python)", in, got, want[i])
			}
		})
	}
}

func linkifyCaseName(in string) string {
	switch in {
	case "":
		return "empty"
	case "#":
		return "bare_hash"
	default:
		return in
	}
}
