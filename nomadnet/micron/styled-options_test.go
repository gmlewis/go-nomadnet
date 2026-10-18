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

import "testing"

// TestRenderToStyledLinesWithDefaultColors pins the parse state's starting color pair,
// which Python's default_state(fg, bg) takes and falls back to the theme's plain style
// for when the caller passes None (MicronParser.py:41-45,63-64). Two callers need an
// explicit value: a page's `#!fg=`/`#!bg=` directives (Browser.py:1269,1348,1513) and a
// message body (Conversations.py:2715-2721).
func TestRenderToStyledLinesWithDefaultColors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opts   StyledOptions
		wantFG string
		wantBG string
		markup string
	}{
		{
			name:   "a plain run takes the configured foreground",
			opts:   StyledOptions{DefaultFG: "f00"},
			markup: "hello",
			wantFG: "#ff0000",
			wantBG: "default",
		},
		{
			name:   "a plain run takes the configured background",
			opts:   StyledOptions{DefaultBG: "00f"},
			markup: "hello",
			wantFG: "#dddddd",
			wantBG: "#0000ff",
		},
		{
			name:   "six-digit colors are accepted too",
			opts:   StyledOptions{DefaultFG: "12ab34"},
			markup: "hello",
			wantFG: "#12ab34",
			wantBG: "default",
		},
		{
			name:   "a `f reset returns to the configured foreground, not the theme plain",
			opts:   StyledOptions{DefaultFG: "0f0"},
			markup: "`fhello",
			wantFG: "#00ff00",
			wantBG: "default",
		},
		{
			name:   "a `b reset returns to the configured background",
			opts:   StyledOptions{DefaultBG: "0f0"},
			markup: "`bhello",
			wantFG: "#dddddd",
			wantBG: "#00ff00",
		},
		{
			name:   "an empty option leaves the theme's plain style in place",
			opts:   StyledOptions{},
			markup: "hello",
			wantFG: "#dddddd",
			wantBG: "default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lines := RenderToStyledLinesWith(tc.markup, ThemeDark, tc.opts)
			if len(lines) == 0 || len(lines[0].Spans) == 0 {
				t.Fatalf("no spans rendered for %q", tc.markup)
			}
			got := lines[0].Spans[0]
			if got.FG != tc.wantFG || got.BG != tc.wantBG {
				t.Errorf("first span FG/BG = %v/%v, want %v/%v", got.FG, got.BG, tc.wantFG, tc.wantBG)
			}
		})
	}
}

// TestRenderToStyledLinesWithZeroOptionsMatchesThePlainForm pins the compatibility rule:
// the two older entry points must behave exactly as the option-taking form with a zero
// StyledOptions, so adding the parameter could not change any existing render.
func TestRenderToStyledLinesWithZeroOptionsMatchesThePlainForm(t *testing.T) {
	t.Parallel()

	markup := ">Heading\n\nplain `!bold`! text\n`Ff00colored\n"
	plain := RenderToStyledLines(markup, ThemeDark)
	withOpts := RenderToStyledLinesWith(markup, ThemeDark, StyledOptions{})
	viaViewer := RenderToStyledLinesFor(markup, ThemeDark, Viewer{})

	if len(plain) != len(withOpts) || len(plain) != len(viaViewer) {
		t.Fatalf("line counts differ: %v / %v / %v", len(plain), len(withOpts), len(viaViewer))
	}
	for i := range plain {
		if len(plain[i].Spans) != len(withOpts[i].Spans) {
			t.Fatalf("line %v: span counts differ (%v vs %v)", i, len(plain[i].Spans), len(withOpts[i].Spans))
		}
		for j := range plain[i].Spans {
			if plain[i].Spans[j] != withOpts[i].Spans[j] {
				t.Errorf("line %v span %v: %+v, want %+v", i, j, withOpts[i].Spans[j], plain[i].Spans[j])
			}
			if plain[i].Spans[j] != viaViewer[i].Spans[j] {
				t.Errorf("line %v span %v: RenderToStyledLinesFor differs: %+v, want %+v",
					i, j, viaViewer[i].Spans[j], plain[i].Spans[j])
			}
		}
	}
}
