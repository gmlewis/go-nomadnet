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
)

// renderConversationsContent draws the ConversationsDisplay content Flex on an
// 80x24 simulation screen and returns the joined cell text per row. It mirrors
// the headless parity capture (tooling/tui-parity) so layout parity with the
// Python original (Conversations.py:205-236) can be pinned in a unit test.
func renderConversationsContent(t *testing.T) []string {
	t.Helper()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	cd.content.SetRect(0, 0, 80, 24)
	cd.content.Draw(screen)
	screen.Sync()

	rows := make([]string, 24)
	for y := 0; y < 24; y++ {
		var b strings.Builder
		for x := 0; x < 80; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// TestConversationsTwoPaneLayout pins the two-pane Conversations layout against
// the Python ground truth (Conversations.py:205-236, captured at 80x24): a
// bordered left pane titled "Conversations" (width 52) holding the Trusted/
// Untrusted tab buttons, IndicativeListBox "───" scroll indicators and a "Last
// sync: never" footer; and a separate bordered right pane showing "No
// conversation selected". There is NO outer border around the two panes.
func TestConversationsTwoPaneLayout(t *testing.T) {
	t.Parallel()
	rows := renderConversationsContent(t)

	// Left pane: bordered, titled "Conversations", occupying columns 0..51.
	if !strings.Contains(rows[0], "Conversations") {
		t.Errorf("left pane title %q missing from top border", "Conversations")
	}
	if got := runeAt(rows[0], 0); got != '┌' {
		t.Errorf("left pane top-left corner col 0 = %q, want ┌", got)
	}
	if got := runeAt(rows[0], 51); got != '┐' {
		t.Errorf("left pane top-right corner col 51 = %q, want ┐ (pane width 52)", got)
	}
	// Right pane: bordered, untitled, occupying columns 52..79.
	if got := runeAt(rows[0], 52); got != '┌' {
		t.Errorf("right pane top-left corner col 52 = %q, want ┌", got)
	}
	if got := runeAt(rows[0], 79); got != '┐' {
		t.Errorf("right pane top-right corner col 79 = %q, want ┐", got)
	}

	// Tab buttons (row 1): "[ Trusted (0)" and "[ Untrusted (0)".
	if !strings.Contains(rows[1], "[ Trusted (0)") {
		t.Errorf("trusted tab button missing; row 1 = %q", rows[1])
	}
	if !strings.Contains(rows[1], "[ Untrusted (0)") {
		t.Errorf("untrusted tab button missing; row 1 = %q", rows[1])
	}

	// IndicativeListBox scroll indicators: "───" appears (centered) above and
	// below the list area.
	indicators := 0
	for _, r := range rows {
		if strings.Contains(r, "───") {
			indicators++
		}
	}
	if indicators < 2 {
		t.Errorf("found %d ─── indicator rows, want at least 2 (top+bottom)", indicators)
	}

	// "Last sync: never" footer near the bottom of the left pane.
	if !anyRowContains(rows, "Last sync: never") {
		t.Errorf("sync footer %q missing", "Last sync: never")
	}

	// Right pane empty state: "No conversation selected" (with a leading blank
	// line, so it appears on row 2 inside the bordered pane).
	if !anyRowContains(rows, "No conversation selected") {
		t.Errorf("right-pane empty state %q missing", "No conversation selected")
	}

	// No outer border: row 0 col 0 is the LEFT pane corner (┌), not a full-width
	// outer border. The cell at col 51 is ┐ (left pane) and col 52 is ┌ (right
	// pane) — i.e. two adjacent pane borders, not one continuous outer border.
	if got := runeAt(rows[0], 50); got == '─' && runeAt(rows[0], 51) == '─' {
		t.Errorf("col 50-51 both ─: looks like a continuous outer border, want two panes")
	}
}

// runeAt returns the rune at column x in row r (counting runes, not bytes).
func runeAt(row string, x int) rune {
	rs := []rune(row)
	if x < 0 || x >= len(rs) {
		return 0
	}
	return rs[x]
}

// anyRowContains reports whether any row contains substr.
func anyRowContains(rows []string, substr string) bool {
	for _, r := range rows {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}
