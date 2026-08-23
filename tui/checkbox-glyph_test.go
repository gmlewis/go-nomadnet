// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestCheckboxGlyphFormat verifies that newUrwidCheckbox renders the urwid
// CheckBox glyph format: `[X] label` (checked) / `[ ] label` (unchecked).
// Python's urwid.CheckBox renders `[X] test` / `[ ] test` (urwid source:
// CheckBox states = {True: "[X]", False: "[ ]"}). The Go tview.Checkbox
// default renders `label X` (label first, indicator after) which does not
// match.
func TestCheckboxGlyphFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		checked bool
		want    string
	}{
		{"checked", true, "[X] test"},
		{"unchecked", false, "[ ] test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cb := newUrwidCheckbox("test", tc.checked)
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(20, 1)
			cb.SetRect(0, 0, 20, 1)
			cb.Draw(screen)

			// Read the first 8 characters of the rendered output.
			got := ""
			for x := 0; x < 8; x++ {
				str, _, _ := screen.Get(x, 0)
				for _, r := range str {
					got += string(r)
					break
				}
			}
			if got != tc.want {
				t.Errorf("checkbox render = %q, want %q", got, tc.want)
			}
		})
	}
}
