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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestFormatRRCBodyEscapesBrackets pins the live bug where a user-typed
// bracket run without spaces ("[bug]") was parsed by tview as a color tag and
// silently swallowed, while "[ bug ]" survived because a space is not a valid
// tag start. Python urwid has no tag grammar, so the whole literal body —
// including the no-span early return — must reach tview escaped.
func TestFormatRRCBodyEscapesBrackets(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)

	for _, text := range []string{
		"[swallowed] vs [ kept ]",
		"[] [[x]] [1]",
		"[x] lxmf@0123456789abcdef0123456789abcdef [y]",
	} {
		msg := ChannelMessage{Text: text}
		got := formatRRCBody(msg, opts)
		// The invariant that matters: once tview has parsed the tags, the
		// visible text is exactly what the user typed.
		tv := tview.NewTextView().SetDynamicColors(true)
		tv.SetText(got)
		if visible := tv.GetText(true); visible != text {
			t.Errorf("formatRRCBody(%q) renders as %q\tagged=%q", text, visible, got)
		}
	}
}

// TestChatBodyRendersBracketsThroughTviewEngine draws chat rows through a real
// tview TextView (SetDynamicColors) on a simulation screen and reads the
// painted cells. Only a full render proves the fork's tag parser no longer eats
// a literal "[swallowed]" — string-level escape assertions alone are not
// enough. The second row also covers the link-span branch, where the literal
// runs on either side of the link must stay escaped.
func TestChatBodyRendersBracketsThroughTviewEngine(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	opts.OwnNick = "tester"
	ts := time.Date(2026, 9, 15, 13, 25, 4, 0, time.Local).UnixMilli()
	const width = 200
	const height = 12

	msgs := []ChannelMessage{
		{Nick: "tester", Text: "bracket test: [swallowed] vs [ kept ] and [] [[x]]", TsMs: ts},
		{Nick: "tester", Text: "[x] lxmf@0123456789abcdef0123456789abcdef [y]", TsMs: ts},
	}

	var tagged strings.Builder
	for _, msg := range msgs {
		for _, l := range formatRRCMessageLines(msg, opts, width) {
			tagged.WriteString(l)
			tagged.WriteByte('\n')
		}
	}

	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetWordWrap(false)
	tv.SetText(tagged.String())
	tv.SetRect(0, 0, width, height)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	tv.Draw(screen)
	screen.Show()

	var painted strings.Builder
	for row := range height {
		for col := range width {
			main, _, _ := screen.Get(col, row)
			painted.WriteString(main)
		}
		painted.WriteByte('\n')
	}
	visible := painted.String()

	for _, want := range []string{
		"[swallowed]",
		"[ kept ]",
		"[]",
		"[[x]]",
		"[x] lxmf@0123456789abcdef0123456789abcdef [y]",
	} {
		if !strings.Contains(visible, want) {
			t.Errorf("tview engine did not paint %q\npainted=\n%v", want, visible)
		}
	}
	if strings.Contains(visible, "swallowed[]") || strings.Contains(visible, " kept []") {
		t.Errorf("escape tokens leaked into painted cells:\n%v", visible)
	}
}
