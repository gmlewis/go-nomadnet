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
	"github.com/rivo/tview"
)

// TestBorderTitleAlignmentUrwidParity verifies that the border title
// alignment matches urwid's left-heavy centering: when the slack
// (available - title) is odd, the extra dash goes to the LEFT. Python
// nomadnet (urwid) renders `┌───...─── Topics ───...──┐` with
// left = right + 1 at odd widths; tview previously centered it
// (left == right). The fix changes tview's Print centering to
// (maxWidth - textWidth + 1) / 2, matching urwid's ceil-based left margin.
//
// Golden values captured from Python urwid LineBox at widths 43, 44, 45:
//
//	width=43: left=17 right=16
//	width=44: left=17 right=17
//	width=45: left=18 right=17
func TestBorderTitleAlignmentUrwidParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		width     int
		wantLeft  int
		wantRight int
	}{
		{43, 17, 16},
		{44, 17, 17},
		{45, 18, 17},
	}

	for _, c := range cases {
		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatalf("screen.Init: %v", err)
		}
		screen.SetSize(c.width, 5)

		tv := tview.NewTextView()
		tv.SetBorder(true)
		tv.SetTitle(" Topics ")
		tv.SetRect(0, 0, c.width, 5)
		tv.Draw(screen)
		screen.Show()

		cells, _, _ := screen.GetContents()
		var row0 strings.Builder
		for i, cell := range cells {
			if i >= c.width {
				break
			}
			if len(cell.Runes) > 0 {
				row0.WriteRune(cell.Runes[0])
			} else {
				row0.WriteByte(' ')
			}
		}
		s := row0.String()
		screen.Fini()

		tIdx := strings.Index(s, "T")
		sIdx := strings.Index(s, "s")
		if tIdx < 0 || sIdx < 0 {
			t.Errorf("width=%d: title not found in %q", c.width, s)
			continue
		}
		gotLeft := strings.Count(s[:tIdx], "─")
		gotRight := strings.Count(s[sIdx+1:], "─")

		if gotLeft != c.wantLeft || gotRight != c.wantRight {
			t.Errorf("width=%d: left=%d right=%d, want left=%d right=%d (urwid parity)\n  %q",
				c.width, gotLeft, gotRight, c.wantLeft, c.wantRight, s)
		}
	}
}
