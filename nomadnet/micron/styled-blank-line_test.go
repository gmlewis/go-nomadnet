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

// TestBlankLineCarriesFormattingState pins the blank-line parity rule:
// Python markup_to_attrmaps renders an empty line as urwid.Text("") wrapped in
// AttrMap(make_style(state)) with the LIVE state (MicronParser.py:117-120), so
// a blank line inside an active `F<color>`/`B<color>`/formatting run carries
// that state, not the theme plain defaults. Found by the ttp-index.mu corpus
// fixture (a blank line between a literal block and `f kept #dddddd in Go that
// Python rendered #ffdd00).
func TestBlankLineCarriesFormattingState(t *testing.T) {
	t.Parallel()
	// The bare `Ffd0 line toggles fg state and renders no visible row.
	markup := "`Ffd0\ncolored line\n\nplain after blank"
	lines := RenderToStyledLines(markup, ThemeDark)
	if len(lines) != 3 {
		t.Fatalf("RenderToStyledLines produced %d lines, want 3", len(lines))
	}
	if got := lines[0].Spans[0].FG; got != "#ffdd00" {
		t.Errorf("`Ffd0 line fg = %q, want #ffdd00", got)
	}
	if got := lines[1].Spans[0].FG; got != "#ffdd00" {
		t.Errorf("blank line fg = %q, want #ffdd00 (carried live state)", got)
	}
	if got := lines[1].Spans[0].Text; got != "" {
		t.Errorf("blank line text = %q, want empty", got)
	}
}
