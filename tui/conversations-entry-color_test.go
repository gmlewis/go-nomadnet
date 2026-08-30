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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// convStyleCell is one decoded cell: the rune and its style.
type convStyleCell struct {
	r     rune
	style tcell.Style
}

// drawConversationsListCells draws the display's conversation list on a
// simulation screen (the drawIndicativeListBox pattern) and returns the cell
// grid w×h plus the plain text of each row.
func drawConversationsListCells(t *testing.T, cd *ConversationsDisplay, w, h int) ([][]convStyleCell, []string) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(w, h)
	cd.ilb.SetRect(0, 0, w, h)
	cd.ilb.List.Focus(func(p tview.Primitive) {})
	cd.ilb.Draw(screen)
	screen.Sync()

	cells, width, _ := screen.GetContents()
	grid := make([][]convStyleCell, h)
	texts := make([]string, h)
	for y := range h {
		var b strings.Builder
		grid[y] = make([]convStyleCell, w)
		for x := range w {
			c := cells[y*width+x]
			r := c.Runes[0]
			if r == 0 {
				r = ' '
			}
			grid[y][x] = convStyleCell{r: r, style: c.Style}
			b.WriteRune(r)
		}
		texts[y] = b.String()
	}
	return grid, texts
}

// TestConversationsEntryColors pins the per-entry styling of the conversations
// list: Python styles each entry with ONE attribute across BOTH lines (name and
// relative time) via urwid AttrMap — list_normal for trusted, list_unknown,
// list_untrusted, and msg_notice_unread for a trusted conversation with unread
// messages (Conversations.py:1687-1756) — and the focus (selected) style
// replaces it on selection. The old Go list drew every name with the tview
// default foreground and every time line with the fork's green secondary
// default, and the selected entry alone matched Python (list_focus).
func TestConversationsEntryColors(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	colors := GetThemeColors(app.Theme)
	// Anchored to now so every fixture stays within the "Nh ago" band of
	// relativeTime at any wall clock — a fixed date (2026-08-29) silently
	// became "yesterday" when the calendar rolled over.
	base := time.Now().UTC()
	convs := []ConversationInfo{
		{SourceHash: "1111111111111111111111111111111111111111", DisplayName: "calm", TrustLevel: "trusted", LastTime: base.Add(-2 * time.Hour)},
		{SourceHash: "2222222222222222222222222222222222222222", DisplayName: "buzz", TrustLevel: "trusted", UnreadCount: 2, LastTime: base.Add(-1 * time.Hour)},
		{SourceHash: "3333333333333333333333333333333333333333", DisplayName: "stranger", TrustLevel: "unknown", LastTime: base.Add(-3 * time.Hour)},
		{SourceHash: "4444444444444444444444444444444444444444", DisplayName: "blockedme", TrustLevel: "untrusted", LastTime: base.Add(-4 * time.Hour)},
		{SourceHash: "5555555555555555555555555555555555555555", DisplayName: "caution", TrustLevel: "warning", LastTime: base.Add(-5 * time.Hour)},
		// Trailing fixtures to hold the (always-present) list selection so the
		// asserted entries above draw unselected and expose their entry attrs:
		{SourceHash: "6666666666666666666666666666666666666666", DisplayName: "zenhold", TrustLevel: "trusted", LastTime: base.Add(-6 * time.Hour)},
		{SourceHash: "7777777777777777777777777777777777777777", DisplayName: "idlehold", TrustLevel: "unknown", LastTime: base.Add(-7 * time.Hour)},
	}
	cd := NewConversationsDisplay(app, convs)

	// Trusted tab: calm (list_normal) + buzz (msg_notice_unread). The list
	// always has a current item and the focus style replaces the entry attr on
	// it (like Python's focus_style), so park the selection on the trailing
	// zenhold fixture to keep the asserted entries unselected.
	cd.list.SetCurrentItem(2)
	grid, texts := drawConversationsListCells(t, cd, 52, 40)
	assertEntryColor(t, grid, texts, "calm", colors["list_normal"])
	assertEntryColor(t, grid, texts, "buzz", colors["msg_notice_unread"])

	// Untrusted tab: unknown → list_unknown, untrusted → list_untrusted,
	// warning → the repo-established warning hue (Python references
	// "list_warning" but never defines it in the urwid palette — an SOT gap
	// that would fail urwid's screen attr lookup; trustPaletteHex in
	// announce-info.go established #ba4).
	cd.SetShowTrusted(false)
	cd.list.SetCurrentItem(3) // park the selection on idlehold (see above)
	grid, texts = drawConversationsListCells(t, cd, 52, 40)
	assertEntryColor(t, grid, texts, "stranger", colors["list_unknown"])
	assertEntryColor(t, grid, texts, "blockedme", colors["list_untrusted"])
	assertEntryColor(t, grid, texts, "caution", cubeHex3("#ba4"))
}

// assertEntryColor checks that the entry named displayName draws its name line
// and its relative-time line in entryFG, mirroring Python's single-attribute
// entry widget.
func assertEntryColor(t *testing.T, grid [][]convStyleCell, texts []string, name string, wantFG tcell.Color) {
	t.Helper()
	fgOf := func(s tcell.Style) tcell.Color { f, _, _ := s.Decompose(); return f }

	var nameRow int = -1
	for y, text := range texts {
		if strings.Contains(text, name) {
			nameRow = y
			break
		}
	}
	if nameRow < 0 {
		t.Fatalf("entry %q not found in the drawn list", name)
	}
	// The time line is the next row and carries "ago" for these fixtures.
	if timeText := texts[nameRow+1]; !strings.Contains(timeText, "ago") {
		t.Fatalf("row below %q = %q, want the relative-time line", name, timeText)
	}
	// A text cell (x=3 is past the trust glyph, into the name glyphs).
	checkFG := func(label string, y int) {
		f := fgOf(grid[y][3].style)
		if f != wantFG {
			t.Errorf("%v line fg = %v, want %v (list palette color)", label, f, wantFG)
		}
	}
	checkFG(name+" name", nameRow)
	checkFG(name+" time", nameRow+1)
}
