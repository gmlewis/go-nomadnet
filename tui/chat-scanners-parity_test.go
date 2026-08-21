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

// The scanner parity tests are LIVE cross-implementation checks: they exec the
// real Python nomadnet reference (nomadnet.ui.textui.Channels) and derive the
// expected output freshly on every run, so a divergence is a real Go/Python
// mismatch to fix in production — never a stale literal transcription. The Go
// test owns the input battery (the edge cases that exercise the
// lookbehind/lookahead boundaries Go's regexp must replicate manually); Python
// owns the reference behavior. Tests SKIP, not fail, when the Python reference
// is not importable (testutils.SkipIfNoPythonNomadnet).

// linkLiveCase is one ScanLinks input row for live parity.
type linkLiveCase struct {
	name string
	text string
}

var scanLinksCases = []linkLiveCase{
	{"lxmf link", "Message me at lxmf@" + h32Parity},
	{"room link", "Join #general for chat"},
	{"room with underscores", "Try #my_room-name"},
	{"page address", "Visit " + h32Parity + " for info"},
	{"multiple links", "See lxmf@deadbeefdeadbeefdeadbeefdeadbeef and #test-room"},
	{"no links", "Just plain text here"},
	{"empty", ""},
	{"room at start", "#general is active"},
	{"room after word char rejected", "foo#bar"},
	{"lxmf after word char rejected", "not_lxmf@" + h32Parity},
	{"page with path", h32Parity + ":path/to/page"},
	{"page preceded by at rejected", "@" + h32Parity},
	{"page inside rejected room", "pre#" + h32Parity},
	{"single char room", "#x"},
	{"room max length 63", "#" + a63Parity},
	{"room truncates at 63", "#" + a63Parity + "a"},
	{"multibyte before page", "café " + h32Parity},
	{"room stops at non-ascii", "room #café-test"},
	{"mix mention and links", "mix @alice and " + h32Parity + " plus #room"},
	{"fenced code no links", "```\ncode @alice\n```"},
	{"inline code no links", "inline `code @bob` here"},
	{"overlapping code and room", "overlapping `code` and #room"},
	{"64 hex no link", h32Parity + h32Parity},
}

const (
	h32Parity = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	a63Parity = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// TestScanLinksPythonParity verifies ScanLinks against Python's _scan_links
// (Channels.py:75) using _LINK_RE (Channels.py:60), executed live. Each
// expectation records the kind, the link target, and the exact matched
// substring (text[Start:End]) so the comparison is independent of
// byte-vs-code-point index conventions. These cases exercise the
// lookbehind/lookahead boundaries that Go's regexp (which lacks lookarounds)
// must replicate manually: a page link preceded by '@' is suppressed, and a
// page link hidden inside a rejected room token ("pre#<hash>") is still found.
func TestScanLinksPythonParity(t *testing.T) {
	t.Parallel()

	texts := make([]string, len(scanLinksCases))
	for i, c := range scanLinksCases {
		texts[i] = c.text
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
texts = json.load(sys.stdin)
out = []
for t in texts:
    row = [{"kind": k, "target": tgt, "match": t[s:e]} for s, e, k, tgt in C._scan_links(t)]
    out.append(row)
json.dump(out, sys.stdout)
`

	var want [][]linkLiveWant
	runPythonNomadnet(t, texts, script, &want)

	for i, c := range scanLinksCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := ScanLinks(c.text)
			wantLinks := want[i]
			if len(got) != len(wantLinks) {
				t.Fatalf("ScanLinks(%q) got %v links, want %v (Python): %+v vs %+v",
					c.text, len(got), len(wantLinks), got, wantLinks)
			}
			for j, w := range wantLinks {
				g := got[j]
				if g.Kind != w.Kind {
					t.Errorf("link[%v].Kind = %q, want %q (Python)", j, g.Kind, w.Kind)
				}
				if g.Target != w.Target {
					t.Errorf("link[%v].Target = %q, want %q (Python)", j, g.Target, w.Target)
				}
				match := ""
				if g.Start >= 0 && g.End <= len(c.text) && g.Start <= g.End {
					match = c.text[g.Start:g.End]
				}
				if match != w.Match {
					t.Errorf("link[%v] match = %q (text[%v:%v]), want %q (Python)",
						j, match, g.Start, g.End, w.Match)
				}
			}
		})
	}
}

type linkLiveWant struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Match  string `json:"match"`
}

// TestScanCodeBlocksPythonParity verifies ScanCodeBlocks against Python's
// _scan_code_blocks (Channels.py:139), executed live. The inline-backtick
// regex in Python uses a (?<!`) lookbehind so a backtick immediately preceded
// by another backtick is not treated as an opener; Go's regexp lacks
// lookarounds, so the scanner must replicate the check manually. The
// double-backtick and "```fence```" cases exercise this.
func TestScanCodeBlocksPythonParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
	}{
		{"no code", "hello world"},
		{"inline code", "use `fmt.Println()` here"},
		{"two inline", "text `x` more `y` end"},
		{"fenced block", "```\ncode @alice\n```"},
		{"fenced with lang", "fence ```py\nprint(1)\n``` tail"},
		{"inline in code", "inline `code @bob` here"},
		{"inline and room", "overlapping `code` and #room"},
		{"empty", ""},
		{"double backtick rejected", "``x``"},
		{"empty fence", "```\n```"},
		{"backtick before inline", "`a``b`"},
		{"two then space backticks", "`` ``"},
		{"space backtick space", "a ` ` b"},
		{"triple then inline", "```fence``` and `inline`"},
		{"unclosed", "`unclosed"},
		{"double unclosed", "``unclosed``"},
		{"triple around letter", "x```y```z"},
	}

	texts := make([]string, len(cases))
	for i, c := range cases {
		texts[i] = c.text
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
texts = json.load(sys.stdin)
out = []
for t in texts:
    row = [{"start": s, "end": e, "match": t[s:e]} for s, e in C._scan_code_blocks(t)]
    out.append(row)
json.dump(out, sys.stdout)
`

	var want [][]codeBlockLiveWant
	runPythonNomadnet(t, texts, script, &want)

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := ScanCodeBlocks(c.text)
			wantRegions := want[i]
			if len(got) != len(wantRegions) {
				t.Fatalf("ScanCodeBlocks(%q) got %v regions, want %v (Python): %+v vs %+v",
					c.text, len(got), len(wantRegions), got, wantRegions)
			}
			for j, w := range wantRegions {
				g := got[j]
				if g.Start != w.Start || g.End != w.End {
					t.Errorf("region[%v] = {%v,%v}, want {%v,%v} (Python) (%q)",
						j, g.Start, g.End, w.Start, w.End, c.text[g.Start:g.End])
				}
				match := ""
				if g.Start >= 0 && g.End <= len(c.text) && g.Start <= g.End {
					match = c.text[g.Start:g.End]
				}
				if match != w.Match {
					t.Errorf("region[%v] match = %q, want %q (Python)", j, match, w.Match)
				}
			}
		})
	}
}

type codeBlockLiveWant struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Match string `json:"match"`
}

// TestScanMentionsPythonParity verifies ScanMentions (self-mentions) and
// scanNickMentions (other-nick mentions) against Python's _scan_mentions and
// _scan_nick_mentions (Channels.py:124,131), executed live. Python
// _scan_mentions yields only matches of the own nick; ScanMentions must return
// exactly those (self) matches. The matched substring (text[Start:End]) is
// compared so the check is independent of byte-vs-code-point index
// conventions.
func TestScanMentionsPythonParity(t *testing.T) {
	t.Parallel()

	type mentionInput struct {
		Text    string `json:"text"`
		OwnNick string `json:"own_nick"`
	}

	selfCases := []mentionInput{
		{"Hey @alice, how are you?", "alice"},
		{"Hey @bob here", "alice"},
		{"@alice and @bob", "alice"},
		{"not@alice here", "alice"},
		{"Hey @Alice", "alice"},
		{"@ALICE!", "alice"},
		{"mail@alice.com", "alice"},
		{"café @alice", "alice"},
		{"@alice_2", "alice"},
		{"@ali", "alice"},
	}
	nickCases := []mentionInput{
		{"Hey @alice and @bob", "alice"},
		{"@alice @bob @carol", "alice"},
		{"not@bob", "alice"},
		{"@bob_2 hi", "alice"},
		{"café @bob", "alice"},
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
inputs = json.load(sys.stdin)
self_out = []
nick_out = []
for idx, inp in enumerate(inputs["self"]):
    row = [{"start": s, "end": e, "match": inp["text"][s:e]} for s, e, _, _ in C._scan_mentions(inp["text"], inp["own_nick"])]
    self_out.append(row)
for idx, inp in enumerate(inputs["nick"]):
    row = [{"start": s, "end": e, "nick": nick, "match": inp["text"][s:e]} for s, e, _, nick in C._scan_nick_mentions(inp["text"], inp["own_nick"])]
    nick_out.append(row)
json.dump({"self": self_out, "nick": nick_out}, sys.stdout)
`

	stdin := map[string]any{
		"self": selfCases,
		"nick": nickCases,
	}
	var want struct {
		Self [][]mentionLiveWant `json:"self"`
		Nick [][]nickLiveWant    `json:"nick"`
	}
	runPythonNomadnet(t, stdin, script, &want)

	for i, c := range selfCases {
		t.Run("self/"+c.Text, func(t *testing.T) {
			t.Parallel()
			got := ScanMentions(c.Text, c.OwnNick)
			wantMentions := want.Self[i]
			if len(got) != len(wantMentions) {
				t.Fatalf("ScanMentions(%q, %q) got %v, want %v (Python): %+v vs %+v",
					c.Text, c.OwnNick, len(got), len(wantMentions), got, wantMentions)
			}
			for j, w := range wantMentions {
				match := ""
				if got[j].Start >= 0 && got[j].End <= len(c.Text) {
					match = c.Text[got[j].Start:got[j].End]
				}
				if match != w.Match {
					t.Errorf("mention[%v] = %q, want %q (Python)", j, match, w.Match)
				}
				if !got[j].IsSelf {
					t.Errorf("mention[%v] IsSelf = false, want true", j)
				}
			}
		})
	}

	for i, c := range nickCases {
		t.Run("nick/"+c.Text, func(t *testing.T) {
			t.Parallel()
			got := scanNickMentions(c.Text, c.OwnNick)
			wantMentions := want.Nick[i]
			if len(got) != len(wantMentions) {
				t.Fatalf("scanNickMentions(%q, %q) got %v, want %v (Python): %+v vs %+v",
					c.Text, c.OwnNick, len(got), len(wantMentions), got, wantMentions)
			}
			for j, w := range wantMentions {
				match := ""
				if got[j].start >= 0 && got[j].end <= len(c.Text) {
					match = c.Text[got[j].start:got[j].end]
				}
				if match != w.Match {
					t.Errorf("nickMention[%v] = %q, want %q (Python)", j, match, w.Match)
				}
				if got[j].nick != w.Nick {
					t.Errorf("nickMention[%v].nick = %q, want %q (Python)", j, got[j].nick, w.Nick)
				}
			}
		})
	}
}

type mentionLiveWant struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Match string `json:"match"`
}

type nickLiveWant struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Nick  string `json:"nick"`
	Match string `json:"match"`
}
