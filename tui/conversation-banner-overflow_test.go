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
	"testing"

	"github.com/gmlewis/tcell/v2"
)

// cautionBg is the "msg_header_caution" background. Both themes map
// msg_header_caution_bg to cubeHex3("#fd3") (palette.go:93/146).
func cautionBg() tcell.Color { return cubeHex3("#fd3") }

// cautionOutside draws the conversation frame at w x h and reports where its
// caution background lands. Any caution cell outside the frame's own inner
// rect is a cell painted over whatever the page put below the conversation
// panel — the reported "yellow message overflows its rendering area" defect.
func cautionOutside(t *testing.T, cw *ConversationWidget, w, h int) (outside, total, firstRow, lastRow int) {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, h)

	cw.frame.SetRect(0, 0, w, h)
	cw.frame.Draw(screen)
	screen.Show()

	cells, cols, _ := screen.GetContents()
	bg := cautionBg()
	firstRow, lastRow = -1, -1
	for i, cell := range cells {
		if _, cellBg, _ := cell.Style.Decompose(); cellBg != bg {
			continue
		}
		total++
		row, col := i/cols, i%cols
		if row < 1 || row > h-2 || col < 1 || col > w-2 {
			outside++
		}
		if firstRow < 0 || row < firstRow {
			firstRow = row
		}
		if row > lastRow {
			lastRow = row
		}
	}
	return outside, total, firstRow, lastRow
}

// newCautionConversation builds a ConversationWidget for a peer whose identity
// is not known, so the footer is the identity-unknown caution banner
// (check_editor_allowed). The frame is laid out at w x h BEFORE the footer is
// built, which is what pins the banner's height in the broken version.
func newCautionConversation(t *testing.T, w, h int) *ConversationWidget {
	t.Helper()

	app := newTestApp()
	cw := NewConversationWidget(app, "7acc095f0e83182feb58c888d090a3cc")
	cw.OnEditorAllowed = func(string) bool { return false }
	cw.frame.SetRect(0, 0, w, h)
	cw.buildFooter()
	return cw
}

// TestConversationCautionBannerNeverOverflowsFrame is the regression test for
// the reported defect: the yellow identity-unknown banner is a FOOTER swap
// (Python AttrMap+Padding+Text(CENTER), Conversations.py:2195-2214), so it must
// stay inside the conversation panel, and it must reach the panel's bottom
// inner row because it is the frame's last child.
//
// The defect needs two sizes, which is why a single-size draw never caught it:
// the appliance starts the client at 80x24 and RESIZEs it to the real grid
// afterwards, so the banner's fixed height is computed once at the narrow
// width. At the device's real grid the right pane is 54 columns (52 inner) and
// 23 rows, where the banner wraps to 13 rows — but at the ~38 columns the
// 80x24 phase leaves the pane it wraps to 16, and the stale 16-row height then
// paints three rows below the panel, over the command footer.
func TestConversationCautionBannerNeverOverflowsFrame(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		builtW, builtH int // where the footer was built
		drawnW, drawnH int // where the frame is drawn
	}{
		{"device: 80x24 phase then the real grid", 40, 20, 54, 23},
		{"built and drawn at the real grid", 54, 23, 54, 23},
		{"shrunk after a wide build", 80, 24, 54, 23},
		{"grown after a narrow build", 30, 15, 54, 23},
		{"short frame at the device width", 54, 12, 54, 12},
		{"narrow pane", 40, 20, 40, 20},
		{"very short frame", 54, 8, 54, 8},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			cw := newCautionConversation(t, c.builtW, c.builtH)
			outside, total, first, last := cautionOutside(t, cw, c.drawnW, c.drawnH)

			if total == 0 {
				t.Fatal("no caution background painted: the banner is not the footer")
			}
			if outside != 0 {
				t.Errorf("caution banner overflows the frame: %v of %v caution cells outside the %vx%v inner rect (banner rows %v..%v, inner rows 1..%v)",
					outside, total, c.drawnW-2, c.drawnH-2, first, last, c.drawnH-2)
			}
			// The banner is the frame's last child, so its last row is the
			// frame's last inner row: the footer must sit flush with the
			// panel bottom rather than float above it.
			if want := c.drawnH - 2; last != want {
				t.Errorf("caution banner ends at row %v, want the frame's last inner row %v", last, want)
			}
			if first < 1 {
				t.Errorf("caution banner starts at row %v, above the frame's first inner row", first)
			}
		})
	}
}

// TestConversationCautionBannerHeightFollowsTheLiveWidth pins the height
// contract directly: the row count the footer occupies must be recomputed at
// the width the frame is actually drawn at, not at whatever width was current
// when the banner was built.
func TestConversationCautionBannerHeightFollowsTheLiveWidth(t *testing.T) {
	t.Parallel()

	// Built where the banner wraps to 16 rows, then laid out where it wraps to
	// 13. The frame's footer slot must end up at the live count.
	cw := newCautionConversation(t, 38, 24)

	cautionOutside(t, cw, 54, 23)

	want := len(urwidSpaceWrap(cw.editorAllowedBannerText(), 52))
	if cw.footerRows != want {
		t.Errorf("footer rows after a draw at width 52 = %v, want %v (the live wrap count)",
			cw.footerRows, want)
	}
}
