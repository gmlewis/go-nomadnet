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
	"unicode/utf8"
)

// TestParseInlineMultibyteGlyph pins the UTF-8 regression: the inline parser
// previously iterated the line as BYTES (c := line[i]; part += string(c)), which
// mangles any multibyte UTF-8 rune. string(byte(0xe2)) == "â" ("â"), so a
// checkmark "✓" (U+2713, UTF-8 e2 9c 93) was rendered as three runes
// "â" + U+009C + U+0093, and the cast showed "â" (the two C1 controls dropped).
//
// Golden (Python MicronParser): a plain-text micron run preserves every input
// rune verbatim. Box-drawing "─" (e2 94 80) survives in the Go cast only because
// it is drawn by tview's border code, not the text path; the text path
// (parseInline) mangled every multibyte glyph. This test exercises the text
// path directly: plain text, escaped text, and the chars that appear in the
// Display Test "Unicode Glyphs" row.
func TestParseInlineMultibyteGlyph(t *testing.T) {
	t.Parallel()

	glyphs := "✓  ✕  ⚠  Ⓝ  ↓"
	lines := RenderToStyledLines(glyphs, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("RenderToStyledLines produced %d lines, want 1", len(lines))
	}
	// Concatenate the span text of the single rendered line.
	var b strings.Builder
	for _, span := range lines[0].Spans {
		b.WriteString(span.Text)
	}
	got := b.String()
	if got != glyphs {
		// Detail the divergence rune-by-rune to make the byte-mangling visible.
		t.Errorf("rendered glyph text diverges:\n got  %q\n want %q", got, glyphs)
		// Explicitly check the checkmark survived as U+2713 (not U+00E2 "â").
		if r := []rune(got); len(r) > 0 && r[0] != '✓' {
			t.Errorf("first rune = U+%04X (%q), want U+2713 (✓)", r[0], string(r[0]))
		}
	}
}

// TestParseInlineMultibyteEscaped pins the escape branch: "\✓" must yield the
// literal rune ✓ (Python escapes the next character), not the byte-mangled
// sequence. A backslash followed by a multibyte char must consume the WHOLE
// rune.
func TestParseInlineMultibyteEscaped(t *testing.T) {
	t.Parallel()

	// `= toggles literal mode so backslashes are real micron escapes... actually
	// escapes apply in normal mode. Use a plain line with an escaped glyph.
	lines := RenderToStyledLines(`\✓`, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("RenderToStyledLines produced %d lines, want 1", len(lines))
	}
	var b strings.Builder
	for _, span := range lines[0].Spans {
		b.WriteString(span.Text)
	}
	got := b.String()
	if want := "✓"; got != want {
		t.Errorf("escaped multibyte: got %q (runes %v), want %q", got, []rune(got), want)
	}
}

// TestParseInlineMultibyteRoundtrip guards against any future regression that
// reintroduces byte-iteration: it feeds a battery of multibyte runes through
// the renderer and asserts each round-trips in the span text.
func TestParseInlineMultibyteRoundtrip(t *testing.T) {
	t.Parallel()

	cases := []string{
		"✓", "✕", "⚠", "Ⓝ", "↓", "↑", "→", "●", "○", "■", "□",
		"─", "│", "┌", "┐", "└", "┘", "├", "┤", "┬", "┴", "┼",
		"╭", "╮", "╰", "╯", // rounded borders (Interfaces detail)
		"é", "ñ", "ü", "Ω", "α", "中", "文",
	}
	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			lines := RenderToStyledLines(in, ThemeDark)
			if len(lines) != 1 {
				t.Fatalf("RenderToStyledLines(%q) produced %d lines, want 1", in, len(lines))
			}
			var b strings.Builder
			for _, span := range lines[0].Spans {
				b.WriteString(span.Text)
			}
			got := b.String()
			if got != in {
				t.Errorf("roundtrip %q → %q (bytes %v), want identity; utf8.Valid(got)=%v",
					in, got, []byte(got), utf8.ValidString(got))
			}
		})
	}
}
