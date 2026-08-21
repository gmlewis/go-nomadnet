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
	"github.com/rivo/tview"
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
		// Heading line: Python's urwid.AttrMap fills the full row width with
		// the heading background, so the converter pads the 5-char "Hello" to
		// the 40-col pane width with heading-bg spaces (35 trailing spaces).
		"[#222222:#bbbbbb]Hello[-:-:-][#222222:#bbbbbb]" + strings.Repeat(" ", 35) + "[-:-:-]",
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
	_, after, ok := strings.Cut(out, "bold here[-:-:-]")
	if !ok {
		t.Fatalf("bold run not closed with reset in: %q", out)
	}
	rest := after
	before0, _, ok := strings.Cut(rest, "and plain")
	if !ok {
		t.Fatalf("plain text not found after bold run in: %q", out)
	}
	// The tag preceding "and plain" must not carry the bold flag.
	before := before0
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

	// Headings fill the full row width with the heading background (Python
	// urwid.AttrMap fallback), so the indent spaces now live inside a color
	// tag rather than the raw string prefix. Strip tview tags to measure the
	// visible leading indent.
	// Heading 1 at depth 1: 0 indent
	if got := leadingSpaces(stripTviewTags(renderedLines[0])); got != 0 {
		t.Errorf("heading 1 leading indent = %d, want 0 (line=%q)", got, renderedLines[0])
	}
	// Heading 2 at depth 2: 2 spaces indent
	if got := leadingSpaces(stripTviewTags(renderedLines[1])); got != 2 {
		t.Errorf("heading 2 leading indent = %d, want 2 (line=%q)", got, renderedLines[1])
	}
	// Heading 3 at depth 3: 4 spaces indent
	if got := leadingSpaces(stripTviewTags(renderedLines[2])); got != 4 {
		t.Errorf("heading 3 leading indent = %d, want 4 (line=%q)", got, renderedLines[2])
	}
	// Each heading row must fill the full 50-col width with the heading
	// background (the parity fix): the visible width equals the pane width.
	for i, l := 0, renderedLines; i < 3 && i < len(l); i++ {
		if got := visibleWidth(l[i]); got != 50 {
			t.Errorf("heading %d visible width = %d, want 50 (line=%q)", i+1, got, l[i])
		}
	}
	// Divider line follows the depth-3 heading, so Python renders it as
	// Padding(Divider, left=4, right=4): 4 leading spaces + (50-8)=42 '─' runes.
	// (micron_parseline.py:259-262: depth>0 → Padding(Divider, left=left_indent,
	// right=right_indent); depth persists from the prior >>>heading.)
	divider := renderedLines[3]
	if got := len([]rune(divider)); got != 46 {
		t.Errorf("divider line runes = %v, want 46 (4 spaces + 42 dashes)", got)
	}
	if !strings.HasPrefix(divider, "    ") {
		t.Errorf("divider line = %q, want 4 leading spaces (depth-3 indent)", divider)
	}
	dashes := strings.TrimPrefix(divider, "    ")
	if strings.Trim(dashes, "─") != "" {
		t.Errorf("divider line = %q, want 4 spaces then only '─' chars", divider)
	}
	if got := len([]rune(dashes)); got != 42 {
		t.Errorf("divider dash count = %v, want 42 (width 50 - left 4 - right 4)", got)
	}
}

// TestStyledLinesToTviewTextAlignment pins micron `c`/`r` alignment rendering
// (B3): Python's urwid.Text(align=state["align"]) centers/right-aligns each
// line within the pane width, so `cThis line will be centered.` at width 40
// renders with (40-27+1)//2 = 7 leading spaces (urwid's center pad is the
// ceiling of half the slack, urwid/text_layout.py:177), and `r...` right-aligns
// with 40-textlen leading spaces. The Go converter must emit that leading
// padding (matching Python's urwid center/right math), not left-align the line.
func TestStyledLinesToTviewTextAlignment(t *testing.T) {
	t.Parallel()

	const w = 40
	cases := []struct {
		name    string
		markup  string
		wantPad int // expected leading spaces before the first color tag
	}{
		{"center", "`cThis line will be centered.", (w - len("This line will be centered.") + 1) / 2},
		{"right", "`rThis will be aligned to the right", w - len("This will be aligned to the right")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := micron.RenderToStyledLines(tc.markup, micron.ThemeDark)
			out, _ := StyledLinesToTviewText(lines, w)
			rendered := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			// Find the content line (skip any blank/leading lines).
			var content string
			for _, l := range rendered {
				if strings.TrimSpace(l) != "" {
					content = l
					break
				}
			}
			if content == "" {
				t.Fatalf("no content line in %q", out)
			}
			gotPad := len(content) - len(strings.TrimLeft(content, " "))
			if gotPad != tc.wantPad {
				t.Errorf("%s: leading pad = %d, want %d (line=%q)", tc.name, gotPad, tc.wantPad, content)
			}
		})
	}
}

// stripTviewTags removes tview color/region tags ([...]) from a string so the
// visible text can be inspected. It is a simple bracket-state machine
// sufficient for the color tags ([fg:bg:flags], [-:-:-]) and region tags
// (["N"], [""]) the converter emits; it is not a general tview parser.
func stripTviewTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '[' {
			inTag = true
			continue
		}
		if inTag {
			if r == ']' {
				inTag = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// leadingSpaces returns the count of leading space runes in s.
func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// visibleWidth returns the on-screen width of a tview-tagged string.
func visibleWidth(s string) int {
	return tview.TaggedStringWidth(s)
}
