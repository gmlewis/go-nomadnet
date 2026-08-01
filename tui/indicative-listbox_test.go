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

// drawIndicativeListBox renders an IndicativeListBox on a simulation screen and
// returns the joined cell text (rows of runes), so the indicator bars and list
// items can be asserted against the Python IndicativeListBox layout.
func drawIndicativeListBox(t *testing.T, ilb *IndicativeListBox, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, h)
	ilb.SetRect(0, 0, w, h)
	// tview primitives need to be drawn via the application to set focus state,
	// but for a bare draw we can call Draw directly. Focus the wrapped List so
	// the current-item highlight renders.
	ilb.List.Focus(func(p tview.Primitive) {})
	ilb.Draw(screen)
	screen.Sync()

	rows := make([]string, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			c, _, _, _ := screen.GetContent(x, y)
			b.WriteRune(c)
		}
		rows[y] = strings.TrimRight(b.String(), " ")
	}
	return rows
}

// TestIndicativeListBoxBothEndsExposed checks that a list whose items all fit
// shows "───" on both the top and bottom bars (Python: both ends visible).
func TestIndicativeListBoxBothEndsExposed(t *testing.T) {
	t.Parallel()
	list := tview.NewList()
	list.ShowSecondaryText(false)
	for _, s := range []string{"Introduction", "Concepts", "Channels"} {
		list.AddItem(s, "", 0, nil)
	}
	ilb := NewIndicativeListBox(list)
	rows := drawIndicativeListBox(t, ilb, 24, 7) // 3 items, height 7 → all fit

	// Top bar (row 0) and bottom bar (row 6) are centered "───".
	if !strings.Contains(rows[0], "───") {
		t.Errorf("top bar = %q, want ───", rows[0])
	}
	if !strings.Contains(rows[6], "───") {
		t.Errorf("bottom bar = %q, want ───", rows[6])
	}
	// Items occupy rows 1..3.
	if rows[1] != "Introduction" {
		t.Errorf("row 1 = %q, want Introduction", rows[1])
	}
	if rows[2] != "Concepts" {
		t.Errorf("row 2 = %q, want Concepts", rows[2])
	}
	if rows[3] != "Channels" {
		t.Errorf("row 3 = %q, want Channels", rows[3])
	}
}

// TestIndicativeListBoxBottomCovered checks that when more items exist than
// fit, the bottom bar shows "▼" (items hidden below) while the top bar shows
// "───" (first item visible at the top).
func TestIndicativeListBoxBottomCovered(t *testing.T) {
	t.Parallel()
	list := tview.NewList()
	list.ShowSecondaryText(false)
	for i := 0; i < 10; i++ {
		list.AddItem("item"+itoa(i), "", 0, nil)
	}
	ilb := NewIndicativeListBox(list)
	rows := drawIndicativeListBox(t, ilb, 24, 6) // list area = 4 rows, 10 items

	if !strings.Contains(rows[0], "───") {
		t.Errorf("top bar = %q, want ─── (top exposed)", rows[0])
	}
	if !strings.Contains(rows[5], "▼") {
		t.Errorf("bottom bar = %q, want ▼ (bottom covered)", rows[5])
	}
}

// TestIndicativeListBoxScrolledDown shows ▲ at the top (items hidden above)
// and ─── at the bottom once the last item is scrolled into view.
func TestIndicativeListBoxScrolledDown(t *testing.T) {
	t.Parallel()
	list := tview.NewList()
	list.ShowSecondaryText(false)
	for i := 0; i < 10; i++ {
		list.AddItem("item"+itoa(i), "", 0, nil)
	}
	list.SetCurrentItem(9) // jump to last item → tview scrolls the bottom into view
	ilb := NewIndicativeListBox(list)
	rows := drawIndicativeListBox(t, ilb, 24, 6)

	if !strings.Contains(rows[0], "▲") {
		t.Errorf("top bar = %q, want ▲ (top covered after scrolling down)", rows[0])
	}
	if !strings.Contains(rows[6-1], "───") {
		t.Errorf("bottom bar = %q, want ─── (bottom exposed at last item)", rows[5])
	}
}

// TestIndicativeListBoxEmptyList shows ─── on both bars for an empty list.
func TestIndicativeListBoxEmptyList(t *testing.T) {
	t.Parallel()
	ilb := NewIndicativeListBox(tview.NewList())
	rows := drawIndicativeListBox(t, ilb, 24, 5)
	if !strings.Contains(rows[0], "───") {
		t.Errorf("top bar = %q, want ─── (empty)", rows[0])
	}
	if !strings.Contains(rows[4], "───") {
		t.Errorf("bottom bar = %q, want ─── (empty)", rows[4])
	}
}

// itoa is a tiny strconv.Itoa stand-in to keep the test file import-light.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
