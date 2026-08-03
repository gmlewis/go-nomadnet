// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// TestStyledLinesToTviewText asserts the styled-lines → tview color-tag
// converter produces exact tview tags from hand-built StyledLines: per-span
// [fg:bg:flags]text[-:-:-] (full reset after every span so bold/italic cannot
// bleed into the next run — the cause of the original all-bold body), divider
// lines expanded to width, section indents preserved, and links wrapped in
// numbered region tags ["N"]...[""].
func TestStyledLinesToTviewText(t *testing.T) {
	t.Parallel()

	hbar := "─"
	linkSpan := micron.StyledSpan{Text: "click me", FG: "#79d79d", BG: "default", Underline: true, Link: &micron.LinkSpec{Label: "click me", URL: "#anchor"}}
	lines := []*micron.StyledLine{
		{Spans: []micron.StyledSpan{{Text: "Hello", FG: "#222222", BG: "#bbbbbb"}}, HeadingLevel: 1, Indent: 0},
		{Spans: []micron.StyledSpan{{Text: "body", FG: "#bbbbbb", BG: "default"}}},
		{Spans: []micron.StyledSpan{{Text: "bold", FG: "#bbbbbb", BG: "default", Bold: true}}},
		{Spans: []micron.StyledSpan{{Text: "underlined italic", FG: "#bbbbbb", Underline: true, Italic: true}}},
		{Divider: true, DividerChar: hbar},
		{Indent: 2, Spans: []micron.StyledSpan{{Text: "indented", FG: "#bbbbbb", BG: "default"}}},
		{Spans: []micron.StyledSpan{linkSpan}},
		{Spans: []micron.StyledSpan{{Text: "plain default", FG: "default", BG: "default"}}},
		{Spans: []micron.StyledSpan{{Text: "gray", FG: "g50", BG: "default"}}},
	}

	got, links := StyledLinesToTviewText(lines, 40)
	if len(links) != 1 || links[0].URL != "#anchor" {
		t.Errorf("links = %+v, want one link with URL #anchor", links)
	}
	want := strings.Join([]string{
		"[#222222:#bbbbbb]Hello[-:-:-]",
		"[#bbbbbb:-]body[-:-:-]",
		"[#bbbbbb:-:b]bold[-:-:-]",
		"[#bbbbbb:-:ui]underlined italic[-:-:-]",
		// tview's [-:-:-] reset does NOT clear the latched underline toggle,
		// so a divider following an underlined run needs an explicit [-:-:U]
		// prefix or the divider chars render underlined.
		"[-:-:U]" + strings.Repeat("─", 40),
		"  [#bbbbbb:-]indented[-:-:-]",
		"[\"0\"][#79d79d:-:u]click me[-:-:-][\"\"]",
		// plain run after an underlined run: explicit :U turns the latched
		// underline toggle OFF (the [-:-:-] reset only clears the bold/italic mask).
		"[-:-:U]plain default[-:-:-]",
		"[#808080:-]gray[-:-:-]",
		"",
	}, "\n")

	if got != want {
		t.Errorf("StyledLinesToTviewText mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

// TestStyledLinesToTviewTextNoBoldBleed renders a real micron snippet via
// RenderToStyledLines and confirms a bold run is closed before the following
// plain run — i.e. the converter emits a full reset after the bold span so
// the plain text is NOT bold (the all-bold regression must not return).
func TestStyledLinesToTviewTextNoBoldBleed(t *testing.T) {
	t.Parallel()

	lines := micron.RenderToStyledLines("`!bold here`! and plain", micron.ThemeDark)
	out, _ := StyledLinesToTviewText(lines, 40)
	// The bold span is closed with [-:-:-] before "and plain" appears, and
	// "and plain" starts with its own non-bold tag (no :b flag).
	idxBold := strings.Index(out, "bold here[-:-:-]")
	if idxBold < 0 {
		t.Fatalf("bold run not closed with reset in: %q", out)
	}
	rest := out[idxBold+len("bold here[-:-:-]"):]
	plainAt := strings.Index(rest, "and plain")
	if plainAt < 0 {
		t.Fatalf("plain text not found after bold run in: %q", out)
	}
	// The tag preceding "and plain" must not carry the bold flag.
	before := rest[:plainAt]
	if strings.Contains(before, ":b]") || strings.Contains(before, ":bu]") || strings.Contains(before, ":bi]") || strings.Contains(before, ":bui]") {
		t.Errorf("plain text inherits bold flag (all-bold regression): %q", before)
	}
}

func TestMicronHeadingDepthAndDividerFormatting(t *testing.T) {
	t.Parallel()

	markup := ">Heading 1\n>>Heading 2\n>>>Heading 3\n-"
	lines := micron.RenderToStyledLines(markup, micronTheme(ThemeDark))
	out, _ := StyledLinesToTviewText(lines, 50)

	renderedLines := strings.Split(out, "\n")
	if len(renderedLines) < 4 {
		t.Fatalf("expected at least 4 rendered lines, got %v", len(renderedLines))
	}

	// Heading 1 at depth 1: 0 indent
	if strings.HasPrefix(renderedLines[0], " ") {
		t.Errorf("heading 1 line = %q, want no leading indent", renderedLines[0])
	}
	// Heading 2 at depth 2: 2 spaces indent
	if !strings.HasPrefix(renderedLines[1], "  ") {
		t.Errorf("heading 2 line = %q, want 2 leading spaces", renderedLines[1])
	}
	// Heading 3 at depth 3: 4 spaces indent
	if !strings.HasPrefix(renderedLines[2], "    ") {
		t.Errorf("heading 3 line = %q, want 4 leading spaces", renderedLines[2])
	}
	// Divider line: 50 width of '-'
	if runeCount := len([]rune(renderedLines[3])); runeCount != 50 || strings.Trim(renderedLines[3], "─") != "" {
		t.Errorf("divider line = %q (runes %v), want 50 '─' characters", renderedLines[3], runeCount)
	}
}
