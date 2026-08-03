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

// TestDisplayTestGradientBars pins V-DisplayTest-Gradient: the four Display
// Test gradient bars render as a run of space cells whose backgrounds step
// through the 3-hex `B<RGB>` (and `Bg<NN>` grayscale) color ramp, with the
// PLAIN foreground and NO underline. Golden values are the Python MicronParser
// color semantics (MicronParser.py make_output `B` branch + make_style):
//   - `B100`..`Bf00` (red ramp)   → bg #110000..#ff0000 (each 3-hex digit doubled)
//   - `B010`..`B0f0` (green ramp) → bg #001100..#00ff00
//   - `B001`..`B00f` (blue ramp)  → bg #000011..#0000ff
//   - `Bg06`..`Bg99` (grayscale)  → bg from the gNN grayscale table
//
// The foreground is the plain style fg (#dddddd dark / #222222 light), and
// Underline/Bold/Italic must all be false on every bar span — the prior A4
// underline leak onto the bar is gone (see tui/underline-leak_test.go).
//
// The final `Bf00` (etc.) sets bg then immediately “ `b “ resets with no
// intervening text, so it produces no visible span; hence 14 visible spans
// per color ramp (1..e), not 15.
func TestDisplayTestGradientBars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		line string
		// wantBG lists the bg of each visible (non-empty-text) span, in order.
		wantBG []string
	}{
		{
			"red",
			"`B100 `B200 `B300 `B400 `B500 `B600 `B700 `B800 `B900 `Ba00 `Bb00 `Bc00 `Bd00 `Be00 `Bf00`b",
			[]string{"#110000", "#220000", "#330000", "#440000", "#550000", "#660000",
				"#770000", "#880000", "#990000", "#aa0000", "#bb0000", "#cc0000",
				"#dd0000", "#ee0000"},
		},
		{
			"green",
			"`B010 `B020 `B030 `B040 `B050 `B060 `B070 `B080 `B090 `B0a0 `B0b0 `B0c0 `B0d0 `B0e0 `B0f0`b",
			[]string{"#001100", "#002200", "#003300", "#004400", "#005500", "#006600",
				"#007700", "#008800", "#009900", "#00aa00", "#00bb00", "#00cc00",
				"#00dd00", "#00ee00"},
		},
		{
			"blue",
			"`B001 `B002 `B003 `B004 `B005 `B006 `B007 `B008 `B009 `B00a `B00b `B00c `B00d `B00e `B00f`b",
			[]string{"#000011", "#000022", "#000033", "#000044", "#000055", "#000066",
				"#000077", "#000088", "#000099", "#0000aa", "#0000bb", "#0000cc",
				"#0000dd", "#0000ee"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines := RenderToStyledLines(tc.line, ThemeDark)
			if len(lines) != 1 {
				t.Fatalf("RenderToStyledLines produced %v lines, want 1", len(lines))
			}
			// Collect the visible (non-empty-text) spans.
			var got []struct {
				text, fg, bg string
				under        bool
			}
			for _, s := range lines[0].Spans {
				if s.Text == "" {
					continue
				}
				got = append(got, struct {
					text, fg, bg string
					under        bool
				}{s.Text, s.FG, s.BG, s.Underline})
			}
			if len(got) != len(tc.wantBG) {
				var dump strings.Builder
				for _, g := range got {
					dump.WriteString(" [")
					dump.WriteString(g.text)
					dump.WriteString("/")
					dump.WriteString(g.bg)
					dump.WriteString("]")
				}
				t.Fatalf("visible spans = %v, want %v: %s", len(got), len(tc.wantBG), dump.String())
			}
			for i, g := range got {
				if g.text != " " {
					t.Errorf("span %v text = %q, want a single space (gradient bar cell)", i, g.text)
				}
				if g.bg != tc.wantBG[i] {
					t.Errorf("span %v bg = %s, want %s", i, g.bg, tc.wantBG[i])
				}
				if g.fg != "#dddddd" {
					t.Errorf("span %v fg = %s, want #dddddd (plain dark fg)", i, g.fg)
				}
				if g.under {
					t.Errorf("span %v carries Underline=true (A4 underline leak)", i)
				}
			}
		})
	}
}
