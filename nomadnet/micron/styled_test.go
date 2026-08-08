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

package micron

import (
	"strings"
	"testing"
)

// TestRenderToStyledLinesPlain verifies the base body-text style: full fg/bg
// from the theme "plain" entry (STYLES_DARK plain fg="ddd"→#dddddd, bg="default"),
// NOT bold — the core fix for the "all-bold body" regression (task 3.8).
//
// The high-color values are derived from Python MicronParser.make_style's
// high_color (MicronParser.py:518-567): "ddd"→"#dddddd", "default"→"default".
func TestRenderToStyledLinesPlain(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("Hello world", ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	want := StyledSpan{Text: "Hello world", FG: "#dddddd", BG: "default"}
	if got := lines[0].Spans[0]; got != want {
		t.Errorf("plain span = %+v, want %+v", got, want)
	}
	if lines[0].HeadingLevel != 0 {
		t.Errorf("plain HeadingLevel = %v, want 0", lines[0].HeadingLevel)
	}
}

// TestRenderToStyledLinesLightTheme confirms the light palette (plain fg="222").
func TestRenderToStyledLinesLightTheme(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("Hello", ThemeLight)
	if got := lines[0].Spans[0].FG; got != "#222222" {
		t.Errorf("light plain FG = %q, want #222222", got)
	}
}

// TestRenderToStyledLinesFormattingToggles asserts bold/underline/italic toggles
// are cumulative and persist across text runs (no reset until “). Golden span
// segmentation + per-span flags captured from Python micron_inline.py for input
// `!bold` and `_underline` and `*italic`.
func TestRenderToStyledLinesFormattingToggles(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`!bold` and `_underline` and `*italic`", ThemeDark)
	spans := lines[0].Spans
	wantSpans := []StyledSpan{
		{Text: "bold", FG: "#dddddd", BG: "default", Bold: true},
		{Text: " and ", FG: "#dddddd", BG: "default", Bold: true},
		{Text: "underline", FG: "#dddddd", BG: "default", Bold: true, Underline: true},
		{Text: " and ", FG: "#dddddd", BG: "default", Bold: true, Underline: true},
		{Text: "italic", FG: "#dddddd", BG: "default", Bold: true, Underline: true, Italic: true},
	}
	if len(spans) != len(wantSpans) {
		t.Fatalf("len(spans) = %v, want %v (spans=%+v)", len(spans), len(wantSpans), spans)
	}
	for i, want := range wantSpans {
		if got := spans[i]; got != want {
			t.Errorf("span[%v] = %+v, want %+v", i, got, want)
		}
	}
}

// TestRenderToStyledLinesColors asserts fg/bg color application and reset, with
// golden span colors from Python micron_inline.py for `F0f0bg green`B0f0on green`b.
func TestRenderToStyledLinesColors(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`F0f0bg green`B0f0on green`b", ThemeDark)
	spans := lines[0].Spans
	wantSpans := []StyledSpan{
		{Text: "bg green", FG: "#00ff00", BG: "default"},
		{Text: "on green", FG: "#00ff00", BG: "#00ff00"},
	}
	if len(spans) != len(wantSpans) {
		t.Fatalf("len(spans) = %v, want %v (spans=%+v)", len(spans), len(wantSpans), spans)
	}
	for i, want := range wantSpans {
		if got := spans[i]; got != want {
			t.Errorf("span[%v] = %+v, want %+v", i, got, want)
		}
	}
}

// TestRenderToStyledLinesFGReset asserts `f resets fg to the default plain fg.
func TestRenderToStyledLinesFGReset(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`Ff00red`f back", ThemeDark)
	spans := lines[0].Spans
	wantSpans := []StyledSpan{
		{Text: "red", FG: "#ff0000", BG: "default"},
		{Text: " back", FG: "#dddddd", BG: "default"},
	}
	if len(spans) != len(wantSpans) {
		t.Fatalf("len(spans) = %v, want %v", len(spans), len(wantSpans))
	}
	for i, want := range wantSpans {
		if got := spans[i]; got != want {
			t.Errorf("span[%v] = %+v, want %+v", i, got, want)
		}
	}
}

// TestRenderToStyledLinesFullReset asserts “ (double backtick) clears all
// formatting and colors (MicronParser.py:641-647).
func TestRenderToStyledLinesFullReset(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`!`Ff00boldred``plain", ThemeDark)
	spans := lines[0].Spans
	wantSpans := []StyledSpan{
		{Text: "boldred", FG: "#ff0000", BG: "default", Bold: true},
		{Text: "plain", FG: "#dddddd", BG: "default"},
	}
	if len(spans) != len(wantSpans) {
		t.Fatalf("len(spans) = %v, want %v", len(spans), len(wantSpans))
	}
	for i, want := range wantSpans {
		if got := spans[i]; got != want {
			t.Errorf("span[%v] = %+v, want %+v", i, got, want)
		}
	}
}

// TestRenderToStyledLinesHeadings asserts headings use the headingN fg/bg from
// STYLES_DARK (NOT bold), the correct section indent, HeadingLevel, and an
// auto-generated slug anchor. Values from MicronParser.py:18-23 + parse_line
// (MicronParser.py:287-322).
func TestRenderToStyledLinesHeadings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		markup string
		level  int
		indent int
		fg, bg string
		anchor string
		text   string
	}{
		{">BIG HEADING", 1, 0, "#222222", "#bbbbbb", "big-heading", "BIG HEADING"},
		{">>Sub heading", 2, 2, "#111111", "#999999", "sub-heading", "Sub heading"},
		{">>>Level Three", 3, 4, "#000000", "#777777", "level-three", "Level Three"},
		{">>>>Deep Four", 4, 6, "#000000", "#777777", "deep-four", "Deep Four"},
	}
	for _, tc := range cases {
		t.Run(tc.markup, func(t *testing.T) {
			t.Parallel()
			lines := RenderToStyledLines(tc.markup, ThemeDark)
			if len(lines) != 1 {
				t.Fatalf("len(lines) = %v, want 1", len(lines))
			}
			sl := lines[0]
			if sl.HeadingLevel != tc.level {
				t.Errorf("HeadingLevel = %v, want %v", sl.HeadingLevel, tc.level)
			}
			if sl.Indent != tc.indent {
				t.Errorf("Indent = %v, want %v", sl.Indent, tc.indent)
			}
			if sl.Anchor != tc.anchor {
				t.Errorf("Anchor = %q, want %q", sl.Anchor, tc.anchor)
			}
			if len(sl.Spans) != 1 {
				t.Fatalf("len(Spans) = %v, want 1 (spans=%+v)", len(sl.Spans), sl.Spans)
			}
			want := StyledSpan{Text: tc.text, FG: tc.fg, BG: tc.bg}
			if got := sl.Spans[0]; got != want {
				t.Errorf("heading span = %+v, want %+v", got, want)
			}
		})
	}
}

// TestRenderToStyledLinesSectionIndent verifies body text after a heading
// inherits the heading's depth as a left indent, and `< resets depth to 0.
func TestRenderToStyledLinesSectionIndent(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines(">>Heading\ncontent at depth 2\n<back at root", ThemeDark)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %v, want 3", len(lines))
	}
	if lines[1].Indent != 2 {
		t.Errorf("depth-2 body Indent = %v, want 2", lines[1].Indent)
	}
	if lines[2].Indent != 0 {
		t.Errorf("after reset Indent = %v, want 0", lines[2].Indent)
	}
}

// TestRenderToStyledLinesLink asserts a link span carries URL/Label/Fields
// metadata and the current render-state style.
func TestRenderToStyledLinesLink(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`[Click`http://example.com`f1|f2]", ThemeDark)
	if len(lines) != 1 || len(lines[0].Spans) != 1 {
		t.Fatalf("got lines=%+v", lines)
	}
	span := lines[0].Spans[0]
	if span.Link == nil {
		t.Fatal("link span has nil Link")
	}
	if span.Link.Label != "Click" || span.Link.URL != "http://example.com" || span.Link.Fields != "f1|f2" {
		t.Errorf("Link = %+v, want {Click http://example.com f1|f2}", span.Link)
	}
	if span.FG != "#dddddd" {
		t.Errorf("link FG = %q, want #dddddd", span.FG)
	}
}

// TestRenderToStyledLinesFields asserts text/checkbox/radio field spans carry
// the parsed FieldSpec from the AST (parity-tested in field-parity_test.go).
func TestRenderToStyledLinesFields(t *testing.T) {
	t.Parallel()

	t.Run("text_field", func(t *testing.T) {
		t.Parallel()
		lines := RenderToStyledLines("`<20|username`alice>", ThemeDark)
		span := lines[0].Spans[0]
		if span.Field == nil {
			t.Fatal("text field span has nil Field")
		}
		want := &FieldSpec{Name: "username", Type: "field", Data: "alice", Width: 20}
		if *span.Field != *want {
			t.Errorf("Field = %+v, want %+v", span.Field, want)
		}
	})
	t.Run("checkbox", func(t *testing.T) {
		t.Parallel()
		lines := RenderToStyledLines("`<?|opt|y|*`Label>", ThemeDark)
		span := lines[0].Spans[0]
		if span.Field == nil {
			t.Fatal("checkbox span has nil Field")
		}
		want := &FieldSpec{Name: "opt", Type: "checkbox", Data: "Label", Value: "y", Prechecked: true}
		if *span.Field != *want {
			t.Errorf("Field = %+v, want %+v", span.Field, want)
		}
	})
	t.Run("radio", func(t *testing.T) {
		t.Parallel()
		lines := RenderToStyledLines("`<^|color|red`Pick red>", ThemeDark)
		span := lines[0].Spans[0]
		if span.Field == nil {
			t.Fatal("radio span has nil Field")
		}
		want := &FieldSpec{Name: "color", Type: "radio", Data: "Pick red", Value: "red"}
		if *span.Field != *want {
			t.Errorf("Field = %+v, want %+v", span.Field, want)
		}
	})
}

// TestRenderToStyledLinesDivider asserts a divider line is flagged with the
// divider char (default U+2500).
func TestRenderToStyledLinesDivider(t *testing.T) {
	t.Parallel()

	// A lone "-" (len 1) yields the default U+2500 divider char.
	lines := RenderToStyledLines("-", ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	if !lines[0].Divider {
		t.Error("Divider = false, want true")
	}
	if lines[0].DividerChar != "─" {
		t.Errorf("DividerChar = %q, want %q", lines[0].DividerChar, "─")
	}

	// "-=" (len 2) uses the second char as the divider char.
	custom := RenderToStyledLines("-=", ThemeDark)
	if custom[0].DividerChar != "=" {
		t.Errorf("custom DividerChar = %q, want =", custom[0].DividerChar)
	}
}

// TestRenderToStyledLinesEmptyLine asserts empty input lines render as a blank
// line (Python: [urwid.Text("")]).
func TestRenderToStyledLinesEmptyLine(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("a\n\nb", ThemeDark)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %v, want 3", len(lines))
	}
	if len(lines[1].Spans) != 1 || lines[1].Spans[0].Text != "" {
		t.Errorf("blank line = %+v, want one empty span", lines[1].Spans)
	}
}

// TestRenderToStyledLinesCommentAndLiteralOmitted asserts comments and the
// literal-toggle line produce no output line (Python parse_line returns None).
func TestRenderToStyledLinesCommentAndLiteralOmitted(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("# a comment\n`=\nvisible", ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1 (comment + literal toggle omitted)", len(lines))
	}
	if lines[0].Spans[0].Text != "visible" {
		t.Errorf("only line text = %q, want %q", lines[0].Spans[0].Text, "visible")
	}
}

// TestRenderToStyledLinesFormattingPersistsAcrossLines asserts the inline
// formatting state persists across lines (Python shares one state dict).
func TestRenderToStyledLinesFormattingPersistsAcrossLines(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`!bold on\nstill bold", ThemeDark)
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %v, want 2", len(lines))
	}
	if !lines[1].Spans[0].Bold {
		t.Errorf("line 2 span Bold = false, want true (bold persists across lines)")
	}
}

// TestRenderToStyledLinesLiteralModePreservesRaw asserts literal mode emits the
// raw line text with no formatting processing.
func TestRenderToStyledLinesLiteralModePreservesRaw(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`=\n`!not bold`\n`=", ThemeDark)
	// `= toggle (omitted), raw line, `= toggle (omitted).
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	if lines[0].Spans[0].Text != "`!not bold`" {
		t.Errorf("literal text = %q, want %q", lines[0].Spans[0].Text, "`!not bold`")
	}
	if lines[0].Spans[0].Bold {
		t.Error("literal span should not be bold")
	}
}

// TestRenderToStyledLinesAnchorDeclaration asserts `:name binds an anchor to
// the current line (3.4 groundwork).
func TestRenderToStyledLinesAnchorDeclaration(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`:my-anchor text", ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	if lines[0].Anchor != "my-anchor" {
		t.Errorf("Anchor = %q, want my-anchor", lines[0].Anchor)
	}
	if lines[0].Spans[0].Text != " text" {
		t.Errorf("text after anchor = %q, want %q", lines[0].Spans[0].Text, " text")
	}
}

// TestRenderToStyledLinesTable asserts a table renders via FormatTableRaw into
// box-drawing styled lines (top border, header, separator, data rows, bottom
// border), mirroring Python render_table (MicronParser.py:197-218). Python
// applies no header bolding, so no span should be bold.
func TestRenderToStyledLinesTable(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines("`t\n| Name | Age |\n| ---- | ---- |\n| Alice | 30 |\n`t", ThemeDark)
	// 1 top border + 1 header + 1 separator + 1 data + 1 bottom border = 5.
	if len(lines) != 5 {
		t.Fatalf("len(lines) = %v, want 5", len(lines))
	}

	// No header bolding in the parity model.
	for i, l := range lines {
		for j, s := range l.Spans {
			if s.Bold {
				t.Errorf("line[%v] span[%v] is bold; parity model has no header bold", i, j)
			}
		}
	}

	// The header row (line 1, after the top border) contains "Name"; the data
	// row (line 3, after the separator) contains "Alice".
	if !strings.Contains(spanText(lines[1]), "Name") {
		t.Errorf("header line[1] = %q, want contains Name", spanText(lines[1]))
	}
	if !strings.Contains(spanText(lines[3]), "Alice") {
		t.Errorf("data line[3] = %q, want contains Alice", spanText(lines[3]))
	}

	// No table-level align (`t, not `tc) → all lines AlignLeft.
	for i, l := range lines {
		if l.Align != AlignLeft {
			t.Errorf("line[%v] align = %v, want AlignLeft", i, l.Align)
		}
	}
}

// spanText concatenates a line's span text for substring checks.
func spanText(l *StyledLine) string {
	var sb strings.Builder
	for _, s := range l.Spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// TestBodyBoldRunFraction mirrors the summary.py `body_bold_run_fraction`
// heuristic (tooling/tui-parity/summary.py:118-131) at the unit level: across a
// Guide-like fixture of headings + body paragraphs + a link, the fraction of
// body content runs (non-heading, non-divider spans with non-space text) that
// are bold must be < 0.1 — the "all-bold body" regression guard for task 3.8.
// Headings must use their headingN fg/bg (not bold); body must be plain.
func TestBodyBoldRunFraction(t *testing.T) {
	t.Parallel()

	markup := ">Guide Title\n" +
		"Welcome to the guide. This is normal body text.\n" +
		"Second line of plain body, also not bold.\n" +
		">>A Section\n" +
		"Section body with a `[link`/path`f1] in it.\n" +
		"Final plain line.\n"
	lines := RenderToStyledLines(markup, ThemeDark)

	var bodyRuns, boldRuns int
	headingStyles := map[int][2]string{1: {"#222222", "#bbbbbb"}, 2: {"#111111", "#999999"}}
	for _, l := range lines {
		if l.HeadingLevel > 0 {
			// Headings carry the headingN fg/bg and are NOT bold.
			want, ok := headingStyles[l.HeadingLevel]
			if !ok {
				continue
			}
			for _, s := range l.Spans {
				if s.Bold {
					t.Errorf("heading level %v span %q is bold; headings use headingN, not bold", l.HeadingLevel, s.Text)
				}
				if s.FG != want[0] || s.BG != want[1] {
					t.Errorf("heading level %v span style = fg %q bg %q, want fg %q bg %q", l.HeadingLevel, s.FG, s.BG, want[0], want[1])
				}
			}
			continue
		}
		if l.Divider {
			continue
		}
		for _, s := range l.Spans {
			if strings.TrimSpace(s.Text) == "" {
				continue
			}
			bodyRuns++
			if s.Bold {
				boldRuns++
			}
		}
	}
	if bodyRuns == 0 {
		t.Fatal("expected body runs, got 0")
	}
	frac := float64(boldRuns) / float64(bodyRuns)
	if frac >= 0.1 {
		t.Errorf("body_bold_run_fraction = %v (%v/%v bold), want < 0.1 (all-bold regression)", frac, boldRuns, bodyRuns)
	}
}
