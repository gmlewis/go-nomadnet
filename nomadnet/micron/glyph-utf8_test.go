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

// TestParseInlineMultibyteGlyph pins that the inline parser decodes whole
// runes rather than bytes: a plain-text micron run preserves every input rune
// verbatim, as Python MicronParser does. It exercises the text path directly
// with the characters that appear in the Display Test "Unicode Glyphs" row.
func TestParseInlineMultibyteGlyph(t *testing.T) {
	t.Parallel()

	glyphs := "✓  ✕  ⚠  Ⓝ  ↓"
	lines := RenderToStyledLines(glyphs, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("RenderToStyledLines produced %v lines, want 1", len(lines))
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
// literal rune ✓ (Python escapes the next character), and a backslash followed
// by a multibyte char must consume the whole rune.
func TestParseInlineMultibyteEscaped(t *testing.T) {
	t.Parallel()

	lines := RenderToStyledLines(`\✓`, ThemeDark)
	if len(lines) != 1 {
		t.Fatalf("RenderToStyledLines produced %v lines, want 1", len(lines))
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

// TestParseInlineMultibyteRoundtrip feeds a battery of multibyte runes through
// the renderer and asserts each round-trips unchanged in the span text.
func TestParseInlineMultibyteRoundtrip(t *testing.T) {
	t.Parallel()

	cases := []string{
		"✓", "✕", "⚠", "Ⓝ", "↓", "↑", "→", "●", "○", "■", "□",
		"─", "│", "┌", "┐", "└", "┘", "├", "┤", "┬", "┴", "┼",
		"╭", "╮", "╰", "╯", // rounded borders (Interfaces detail)
		"é", "ñ", "ü", "Ω", "α", "中", "文",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			lines := RenderToStyledLines(in, ThemeDark)
			if len(lines) != 1 {
				t.Fatalf("RenderToStyledLines(%q) produced %v lines, want 1", in, len(lines))
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
