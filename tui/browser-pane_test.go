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

	"github.com/rivo/tview"
)

// TestBrowserPaneDisconnectedLayout pins the Network right-pane browser's
// disconnected/boot state against Python's Browser.build_display
// (Browser.py:472-488): a LineBox titled "Remote Node" wrapping a BrowserFrame
// whose body is a MIDDLE-filled, centered "Disconnected\n<arrow_l>  <arrow_r>"
// (browser_inactive fg #444). Header/footer are empty (0 rows), so the body
// fills the pane and the 2-line content is vertically centered (urwid
// Filler(MIDDLE) floors the top → tview two-equal-weight spacers match).
// Horizontal centering is ceil-left (urwid Text(CENTER) → centeredText).
func TestBrowserPaneDisconnectedLayout(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bp := NewBrowserPane(app)

	rows := renderPrimitive(t, bp.Widget(), 28, 22)

	// Row 0: the border with the "Remote Node" title.
	if !strings.Contains(rows[0], "Remote Node") {
		t.Errorf("row 0 = %q, want it to contain 'Remote Node' title", rows[0])
	}
	// Inner rect is 26 wide (28 - 2 borders), 20 tall (22 - 2 borders).
	// Vertical: Filler(MIDDLE) top = (20-2)/2 = 9 → content at inner rows 9,10
	// → absolute rows 10, 11.
	// "Disconnected" (12 cols) ceil-left in 26: (26-12+1)/2 = 7 → 'D' at col 8.
	if c := cellAt(rows, 8, 10); c != "D" {
		t.Errorf("'D' at (8,10) = %q, want 'D' (row10=%q)", c, rows[10])
	}
	if !strings.Contains(rows[10], "Disconnected") {
		t.Errorf("row 10 = %q, want 'Disconnected'", rows[10])
	}
	// "<arrow_l>  <arrow_r>" (4 cols) ceil-left in 26: (26-4+1)/2 = 11 → '<' at col 12.
	arrowL := app.Glyphs["arrow_l"]
	if c := cellAt(rows, 12, 11); c != arrowL {
		t.Errorf("arrow_l at (12,11) = %q, want %q (row11=%q)", c, arrowL, rows[11])
	}
	// Rows above (1-9) and below (12-20) are blank inside the border.
	for _, y := range []int{1, 5, 9, 12, 16, 20} {
		if strings.TrimSpace(rows[y]) != "│                          │" {
			t.Errorf("row %v = %q, want empty border row", y, rows[y])
		}
	}
}

func TestFormatRemoteNodeTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want string
	}{
		{"", "Remote Node"},
		{"nomadnetwork://44f0dbf2ec1c2ac47277995475217aed/page/status.mu", "<44f0dbf2ec1c2ac47277995475217aed>"},
		{"44f0dbf2ec1c2ac47277995475217aed", "<44f0dbf2ec1c2ac47277995475217aed>"},
		{"my-node-alias", "my-node-alias"},
	}

	for _, tt := range tests {
		if got := FormatRemoteNodeTitle(tt.url); got != tt.want {
			t.Errorf("FormatRemoteNodeTitle(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestBrowserPaneLoadURLTitle(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	bp := NewBrowserPane(app)

	bp.LoadURL("nomadnetwork://44f0dbf2ec1c2ac47277995475217aed/page/status.mu")
	flex, ok := bp.Widget().(*tview.Flex)
	if !ok {
		t.Fatalf("expected *tview.Flex")
	}
	if got := flex.GetTitle(); got != " <44f0dbf2ec1c2ac47277995475217aed> " {
		t.Errorf("title after LoadURL = %q, want ' <44f0dbf2ec1c2ac47277995475217aed> '", got)
	}
}
