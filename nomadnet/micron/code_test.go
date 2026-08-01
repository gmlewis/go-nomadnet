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

// TestInlineCodeSpan asserts a markdown inline-code span — which
// MarkdownToMicron emits as `BT383838`Fddd...`f`b (util.py CODE_BG_INLINE /
// CODE_FG / CODE_RESET) — renders as a styled span with the code fg (#dddddd)
// and inline-code bg (#383828), followed by a reset to the plain style for the
// trailing text. Golden micron captured from Python format_block.
func TestInlineCodeSpan(t *testing.T) {
	t.Parallel()

	// Python: "Some `BT383838`Fdddinline code`f`b here."
	markup := "`BT383838`Fdddinline code`f`b here."
	lines := RenderToStyledLines(markup, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	spans := lines[0].Spans
	// span[0] = "inline code" (code colors), span[1] = " here." (plain).
	if len(spans) != 2 {
		t.Fatalf("len(spans) = %v, want 2: %+v", len(spans), spans)
	}
	if spans[0].Text != "inline code" {
		t.Errorf("span[0].Text = %q, want %q", spans[0].Text, "inline code")
	}
	if spans[0].FG != "#dddddd" {
		t.Errorf("span[0].FG = %q, want #dddddd (CODE_FG `Fddd)", spans[0].FG)
	}
	if spans[0].BG != "#383838" {
		t.Errorf("span[0].BG = %q, want #383838 (CODE_BG_INLINE `BT383838)", spans[0].BG)
	}
	if spans[1].Text != " here." {
		t.Errorf("span[1].Text = %q, want %q", spans[1].Text, " here.")
	}
	if spans[1].BG != "default" {
		t.Errorf("span[1].BG = %q, want default (reset by `b)", spans[1].BG)
	}
}

// TestCodeBlock asserts a fenced markdown code block — which MarkdownToMicron
// emits as a `BT282828`Fddd color-setup line, a `= literal block containing the
// code, and a `f`b reset — renders exactly the literal code lines under the
// code fg (#dddddd) / block bg (#282828), with the setup/toggle/reset lines
// producing no visible output. Golden micron captured from Python format_block.
func TestCodeBlock(t *testing.T) {
	t.Parallel()

	// Python output for a fenced ``` block:
	//   `BT282828`Fddd
	//   `=
	//   def foo():
	//       return 1
	//   `=
	//   `f`b
	markup := "`BT282828`Fddd\n`=\ndef foo():\n    return 1\n`=\n`f`b"
	lines := RenderToStyledLines(markup, ThemeDark)
	// Only the two literal code lines are visible.
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %v, want 2 (literal lines only): %+v", len(lines), lines)
	}
	want := []string{"def foo():", "    return 1"}
	for i, w := range want {
		if textOf(lines[i]) != w {
			t.Errorf("lines[%v] = %q, want %q", i, textOf(lines[i]), w)
		}
		if lines[i].Spans[0].FG != "#dddddd" {
			t.Errorf("lines[%v].FG = %q, want #dddddd (CODE_FG)", i, lines[i].Spans[0].FG)
		}
		if lines[i].Spans[0].BG != "#282828" {
			t.Errorf("lines[%v].BG = %q, want #282828 (CODE_BG)", i, lines[i].Spans[0].BG)
		}
	}
}

// TestLiteralBlockRawTags asserts that inside a `= literal block, micron tags
// are NOT interpreted — they appear as literal text. This is the "source code"
// display use case (MicronParser.py:595-597).
func TestLiteralBlockRawTags(t *testing.T) {
	t.Parallel()

	markup := "`=\n`Ff00not a color`f\n`="
	lines := RenderToStyledLines(markup, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	if textOf(lines[0]) != "`Ff00not a color`f" {
		t.Errorf("literal text = %q, want tags uninterpreted", textOf(lines[0]))
	}
}

// TestLiteralBlockEscapedToggle asserts \`= inside a literal block renders as
// the literal two-char sequence `= (Python make_output: line == "\\`=" → "`=").
func TestLiteralBlockEscapedToggle(t *testing.T) {
	t.Parallel()

	markup := "`=\n\\`=\n`="
	lines := RenderToStyledLines(markup, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %v, want 1", len(lines))
	}
	if textOf(lines[0]) != "`=" {
		t.Errorf("escaped literal toggle = %q, want %q", textOf(lines[0]), "`=")
	}
}

// textOf concatenates a line's span text.
func textOf(l *StyledLine) string {
	var sb strings.Builder
	for _, s := range l.Spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}
