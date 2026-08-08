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
)

// TestHalfBlock5x4Glyphs asserts the ported glyph table matches urwid's
// HalfBlock5x4Font for a representative sample of characters. Expected rows
// (including trailing spaces, exactly W runes wide) were captured from urwid
// 4.0.3's HalfBlock5x4Font.render — the read-only Python source of truth.
func TestHalfBlock5x4Glyphs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ch   rune
		w    int
		rows []string
	}{
		{'A', 5, []string{"▄▀▀▄ ", "█▄▄█ ", "█  █ ", "▀  ▀ "}},
		{'B', 5, []string{"█▀▀▄ ", "█▄▄▀ ", "█  █ ", "▀▀▀  "}},
		{'1', 5, []string{" ▄█  ", "  █  ", "  █  ", " ▀▀▀ "}},
		{'0', 5, []string{"▄▀▀▄ ", "█  █ ", "█  █ ", " ▀▀  "}},
		{' ', 2, []string{"  ", "  ", "  ", "  "}},
		{'!', 2, []string{"█ ", "█ ", "▀ ", "▀ "}},
		{'I', 2, []string{"█ ", "█ ", "█ ", "▀ "}},
		{'M', 6, []string{"█▄ ▄█ ", "█ ▀ █ ", "█   █ ", "▀   ▀ "}},
	}

	halffontOnce.Do(loadHalffont)
	for _, c := range cases {
		g, ok := halffontGlyphs[c.ch]
		if !ok {
			t.Errorf("glyph %q missing from table", c.ch)
			continue
		}
		if g.W != c.w {
			t.Errorf("glyph %q width = %v, want %v", c.ch, g.W, c.w)
		}
		if len(g.Rows) != len(c.rows) {
			t.Errorf("glyph %q rows = %v, want %v", c.ch, g.Rows, c.rows)
			continue
		}
		for r := range c.rows {
			if g.Rows[r] != c.rows[r] {
				t.Errorf("glyph %q row %v = %q, want %q", c.ch, r, g.Rows[r], c.rows[r])
			}
			if rc := []rune(g.Rows[r]); len(rc) != c.w {
				t.Errorf("glyph %q row %v rune count = %v, want %v", c.ch, r, len(rc), c.w)
			}
		}
	}
}

// TestHalfBlock5x4Render asserts the multi-glyph concatenation matches
// urwid.BigText.render for several strings (trailing spaces trimmed, as the
// splash displays them). Expected values captured from urwid.
func TestHalfBlock5x4Render(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"A", "A", []string{"▄▀▀▄", "█▄▄█", "█  █", "▀  ▀"}},
		{"AB", "AB", []string{"▄▀▀▄ █▀▀▄", "█▄▄█ █▄▄▀", "█  █ █  █", "▀  ▀ ▀▀▀"}},
		{"0", "0", []string{"▄▀▀▄", "█  █", "█  █", " ▀▀"}},
		{"!", "!", []string{"█", "█", "▀", "▀"}},
		{
			"Nomad Network",
			"Nomad Network",
			[]string{
				"██ █                    █   ██ █       █                 █",
				"█▐▌█ ▄▀▀▄ █▀▄▀▄  ▀▀▄ ▄▀▀█   █▐▌█ ▄▀▀▄ ▀█▀ █ ▄ █ ▄▀▀▄ █▀▀ █ ▄▀",
				"█ ██ █  █ █ █ █ ▄▀▀█ █  █   █ ██ █▀▀   █  ▐▌█▐▌ █  █ █   █▀▄",
				"▀  ▀  ▀▀  ▀   ▀  ▀▀▀  ▀▀▀   ▀  ▀  ▀▀    ▀  ▀ ▀   ▀▀  ▀   ▀  ▀",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := halfBlock5x4RenderTrimmed(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("render %q returned %v rows, want %v", c.in, len(got), len(c.want))
			}
			for r := range c.want {
				if got[r] != c.want[r] {
					t.Errorf("render %q row %v =\n  %q\nwant\n  %q", c.in, r, got[r], c.want[r])
				}
			}
		})
	}
}

// TestHalfBlock5x4Width asserts the rendered width (urwid.BigText.pack) for
// several strings: the sum of per-glyph widths, unknown chars contributing 0.
func TestHalfBlock5x4Width(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int
	}{
		{"A", 5},
		{"AB", 10},
		{"!", 2},
		{" ", 2},
		{"I", 2},
		// "Nomad Network" per-glyph widths: N5 o5 m6 a5 d5 sp2 N5 e5 t4 w6 o5 r4 k5 = 62.
		{"Nomad Network", 62},
	}

	for _, c := range cases {
		if got := halfBlock5x4Width(c.in); got != c.want {
			t.Errorf("width(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestHalfBlock5x4RenderUnknownCharsSkipped asserts characters not in the font
// contribute nothing (matching urwid.BigText.render, which only appends glyphs
// with non-zero char_width).
func TestHalfBlock5x4RenderUnknownCharsSkipped(t *testing.T) {
	t.Parallel()

	// 'A' is in the font; '~' is in the font too. Use a control char (\x01)
	// which is definitely absent.
	got := halfBlock5x4RenderTrimmed("A\x01A")
	want := halfBlock5x4RenderTrimmed("AA")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("render with unknown char differs from without; got %v want %v", got, want)
	}
}
