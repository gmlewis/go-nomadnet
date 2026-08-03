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

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// TestStyledLinesToTviewTextUnderlineNoLeak pins the fix for the sticky
// underline leak seen in go_session-002.cast's Guide "Markup & Color Display
// Test" topic: the line "The following line should contain a red gradient
// bar:" rendered UNDERLINED even though the micron renderer produced it with
// Underline=false.
//
// Root cause: tview's color-tag reset [-:-:-] (strings.go parseTag) resets the
// foreground, background, and the bold/italic attribute MASK, but it does NOT
// clear the separate Underline toggle, which is set by the lowercase 'u' tag
// and only cleared by the uppercase 'U' tag. So once any span emits :u, every
// following run — plain indent spaces, spans whose tag has no attribute field,
// divider chars — inherits underline until an explicit :U. The micron
// renderer is correct; the leak is in StyledLinesToTviewText, which must
// track the latched underline state and emit 'U' to turn it off.
//
// Golden (python_session.cast / Guide.py TOPIC_DISPLAYTEST): a line that
// closes underline with a second `_` toggle and a “ full reset is followed
// by a plain "The following line should contain a red gradient bar:" line
// that is NOT underlined.
func TestStyledLinesToTviewTextUnderlineNoLeak(t *testing.T) {
	t.Parallel()

	// Exact Display Test snippet: underline opened then closed on one line,
	// full reset, blank, then a plain line that must NOT inherit underline.
	markup := "`r`_And this one should be underlined, aligned right`_\n``\n\nThe following line should contain a red gradient bar:"
	lines := micron.RenderToStyledLines(markup, micron.ThemeLight)
	out, _ := StyledLinesToTviewText(lines, 80)

	// (1) Converter level: the underlined run emits :u; the following plain
	// run must emit an explicit uppercase :U (or a tag containing U) so that
	// tview turns the latched underline toggle OFF. Without it, tview
	// renders the plain line underlined.
	underlinedAt := strings.Index(out, ":u]")
	if underlinedAt < 0 {
		t.Fatalf("underlined run :u] not found in output: %q", out)
	}
	afterUnderline := out[underlinedAt:]
	plainAt := strings.Index(afterUnderline, "The following line")
	if plainAt < 0 {
		t.Fatalf("plain line not found after underlined run in: %q", out)
	}
	before := afterUnderline[:plainAt]
	// The tag immediately preceding the plain text must carry an uppercase U
	// to clear the latched underline toggle (the [-:-:-] reset does not).
	if !strings.Contains(before, ":U]") {
		t.Errorf("plain line after underlined run is not cleared with an explicit :U tag (underline will leak): %q", before)
	}

	// (2) End-to-end: render through a real tview TextView to a simulation
	// screen and assert the plain line's cells carry NO underline attribute.
	// This is the golden proof matching the Python source-of-truth.
	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(90, 6)

	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetRegions(true)
	tv.SetText(out)
	tv.SetRect(0, 0, 90, 6)
	tv.Draw(screen)

	// Scan every cell for the plain-line text and confirm none is underlined.
	target := "The following line should contain a red gradient bar:"
	runes := []rune(target)
	found := 0
	for y := 0; y < 6; y++ {
		for x := 0; x < 90; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			if c != runes[found] {
				found = 0
				continue
			}
			found++
			if found == len(runes) {
				// We matched the full plain line; verify none of its cells
				// carried the underline attribute.
				for i := 0; i < len(runes); i++ {
					_, _, attr := styleAt(screen, x-i, y)
					if attr&tcell.AttrUnderline != 0 {
						t.Errorf("plain line cell (%v,%v) carries AttrUnderline — underline leaked from previous underlined run (tview [-:-:-] does not reset underline)", x-i, y)
					}
				}
				return
			}
		}
	}
	t.Errorf("plain-line text %q not found on the rendered screen", target)
}

// styleAt returns the tcell Style of the cell at (x,y), decomposed.
func styleAt(screen tcell.Screen, x, y int) (c tcell.Color, bg tcell.Color, attr tcell.AttrMask) {
	r, _, style, _ := screen.GetContent(x, y)
	_ = r
	c, bg, attr = style.Decompose()
	return
}
