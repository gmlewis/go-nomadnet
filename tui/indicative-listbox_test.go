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
	for y := range h {
		var b strings.Builder
		for x := range w {
			c, _, _, _ := cellContent(screen, x, y)
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
	for i := range 10 {
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
	for i := range 10 {
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

// TestIndicativeListBoxCursorTracksHighlightTwoLine pins the hardware-cursor
// parity for two-line list items (the Conversations list): Python renders each
// entry as one urwid Text (name + "\n  time"), so urwid's focus cursor sits on
// the FIRST row of the current entry and every Up/Down moves it exactly one
// CONVERSATION (two physical rows — live capture "py_*": both name and time
// lines share the list_focus style). The old ILB handed ShowCursor the item
// INDEX as a row offset (row = current-offset), so with 2-row items the cursor
// landed a conversation below each press and drifted one row per keypress
// against the highlight (live: highlight row 16 vs cursor (1,10), then 14 vs
// (1,9), then 12 vs (1,8) across three Ups).
//
// It also pins the single-line case (Guide / Saved Nodes lists): one row per
// item, cursor exactly on the highlighted row.
func TestIndicativeListBoxCursorTracksHighlightTwoLine(t *testing.T) {
	t.Parallel()

	t.Run("secondary text on: cursor on the current item's first row", func(t *testing.T) {
		t.Parallel()
		list := tview.NewList() // showSecondaryText defaults to true
		for i := range 4 {
			list.AddItem("conv"+itoa(i), "  2w ago", 0, nil)
		}
		list.SetCurrentItem(2)
		ilb := NewIndicativeListBox(list)
		cx, cy, visible := cursorOf(t, ilb, 24, 9)
		if !visible {
			t.Fatal("cursor not visible, want it on the highlighted name row")
		}

		// List area = rows 1..7 (height 9 → listRect row 1, height 7); each
		// item occupies 2 rows (main + secondary). Item 2's MAIN line (the
		// highlighted row, where Python's entry-cursor sits) is at
		// listY + 2*2 = 5.
		if cy != 5 {
			t.Errorf("cursor y = %v, want 5 (listY=1 + item 2's first of 2 rows)", cy)
		}
		if cx != 0 {
			t.Errorf("cursor x = %v, want 0 (listX)", cx)
		}
	})

	t.Run("secondary text off: cursor row equals the item index", func(t *testing.T) {
		t.Parallel()
		list := tview.NewList()
		list.ShowSecondaryText(false)
		for i := range 10 {
			list.AddItem("item"+itoa(i), "", 0, nil)
		}
		list.SetCurrentItem(3)
		ilb := NewIndicativeListBox(list)
		_, cy, visible := cursorOf(t, ilb, 24, 9)
		if !visible {
			t.Fatal("cursor not visible, want it on the highlighted row")
		}
		// List area = rows 1..7 (height 9 → listRect row 1, height 7); item 3
		// (one row per item, fully visible) is at listY+3 = 4.
		if cy != 4 {
			t.Errorf("cursor y = %v, want 4 (listY=1 + item 3)", cy)
		}
	})
}

// cursorOf draws ilb on a simulation screen of w×h and returns the hardware
// cursor position and visibility after the draw.
func cursorOf(t *testing.T, ilb *IndicativeListBox, w, h int) (int, int, bool) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, h)
	ilb.SetRect(0, 0, w, h)
	ilb.List.Focus(func(p tview.Primitive) {})
	ilb.Draw(screen)
	screen.Sync()
	x, y, visible := screen.GetCursor()
	return x, y, visible
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

// newWheelILB builds an IndicativeListBox of n items laid out on a simulation
// screen of w×h, settled with a Draw so currentItem/itemOffset are valid. It
// returns the box, its mouse handler and the screen (for any follow-up Draws).
// Callers fire wheel events via the handler with an event positioned over the
// list body.
func newWheelILB(t *testing.T, n, w, h int) (*IndicativeListBox, func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive), tcell.Screen) {
	t.Helper()
	list := tview.NewList()
	list.ShowSecondaryText(false)
	for i := range n {
		list.AddItem("item"+itoa(i), "", 0, nil)
	}
	ilb := NewIndicativeListBox(list)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(w, h)
	ilb.SetRect(0, 0, w, h)
	ilb.List.Focus(func(p tview.Primitive) {})
	ilb.Draw(screen) // settle currentItem=0, itemOffset=0
	screen.Sync()
	return ilb, ilb.MouseHandler(), screen
}

// TestIndicativeListBoxWheelMovesHighlight pins the wheel fix: one wheel
// delivery moves the highlight (currentItem) by mouseWheelLines rows via
// SetCurrentItem — arrow-key semantics — instead of tview's default
// itemOffset-only viewport move that the Draw keep-current-visible clamp
// cancels at the edges. This is the fix for the "wheel stuck at the highlight"
// bug on the Network Announce Stream / Saved Nodes lists.
//
// The rows-per-notch MULTIPLIER (GONOMADNET_WHEEL_LINES) is applied inside this
// handler — the per-primitive multiplier — so with it pinned to 3, one delivery
// moves exactly 3 rows. (TextView-based scroll regions apply the same
// multiplier through applyWheelMultiplier; lists apply it directly because they
// don't use tview TextView's trackEnd, so a single SetCurrentItem jump is safe.)
//
// Mutates the package-global mouseWheelLines, so it runs sequentially (no
// t.Parallel) and restores the prior value on cleanup.
func TestIndicativeListBoxWheelMovesHighlight(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	SetMouseWheelLines(3)

	const w, h = 20, 7 // list area height = 5; 20 items don't all fit
	ilb, handler, screen := newWheelILB(t, 20, w, h)
	list := ilb.List
	ev := func() *tcell.EventMouse { return tcell.NewEventMouse(w/2, h/2, tcell.ButtonNone, tcell.ModNone) }
	setFocus := func(p tview.Primitive) {}

	if got := list.GetCurrentItem(); got != 0 {
		t.Fatalf("initial currentItem = %v, want 0", got)
	}

	// Down over the list body: highlight moves 3 rows per delivery (the pinned
	// multiplier), via SetCurrentItem.
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); !consumed {
		t.Error("MouseScrollDown: consumed=false, want true")
	}
	if got := list.GetCurrentItem(); got != 3 {
		t.Errorf("after 1 down: currentItem = %v, want 3", got)
	}

	// Second down: 6.
	handler(tview.MouseScrollDown, ev(), setFocus)
	if got := list.GetCurrentItem(); got != 6 {
		t.Errorf("after 2 down: currentItem = %v, want 6", got)
	}

	// Up: back to 3.
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); !consumed {
		t.Error("MouseScrollUp: consumed=false, want true")
	}
	if got := list.GetCurrentItem(); got != 3 {
		t.Errorf("after up: currentItem = %v, want 3", got)
	}

	// After a Draw the viewport must keep the highlight visible (the Draw
	// keep-current-visible clamp follows the highlight — never stuck).
	ilb.Draw(screen)
	cur := list.GetCurrentItem()
	off, _ := list.GetOffset()
	_, _, _, listH := ilb.listRect()
	if cur < off || cur > off+listH-1 {
		t.Errorf("highlight %v not visible after Draw: offset=%v listH=%v", cur, off, listH)
	}
}

// TestIndicativeListBoxWheelBoundaryNoOp pins the boundary guard: a wheel
// delivery at the top (scrolling up) or bottom (scrolling down) declines to
// consume so tview skips the no-op redraw, while a mid-list delivery still
// consumes and moves the highlight by the pinned multiplier (3). Mirrors
// TestScrollBarWheelMultiplier's shape.
//
// Mutates the package-global mouseWheelLines, so it runs sequentially.
func TestIndicativeListBoxWheelBoundaryNoOp(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	SetMouseWheelLines(3)

	const w, h = 20, 7
	ilb, handler, screen := newWheelILB(t, 20, w, h)
	list := ilb.List
	ev := func() *tcell.EventMouse { return tcell.NewEventMouse(w/2, h/2, tcell.ButtonNone, tcell.ModNone) }
	setFocus := func(p tview.Primitive) {}

	// At the top (item 0), scrolling up is a no-op: must NOT consume and the
	// highlight must stay put.
	list.SetCurrentItem(0)
	ilb.Draw(screen) // keep event.Position inside the settled rect
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); consumed {
		t.Error("ScrollUp at top: consumed=true, want false (no-op should skip redraw)")
	}
	if got := list.GetCurrentItem(); got != 0 {
		t.Errorf("ScrollUp at top moved highlight to %v, want 0", got)
	}

	// At the bottom (item 19), scrolling down is a no-op.
	list.SetCurrentItem(19)
	ilb.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); consumed {
		t.Error("ScrollDown at bottom: consumed=true, want false (no-op should skip redraw)")
	}
	if got := list.GetCurrentItem(); got != 19 {
		t.Errorf("ScrollDown at bottom moved highlight to %v, want 19", got)
	}

	// From the middle, both consume and move by the multiplier (3).
	list.SetCurrentItem(10)
	ilb.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollDown, ev(), setFocus); !consumed {
		t.Error("ScrollDown at mid: consumed=false, want true")
	}
	if got := list.GetCurrentItem(); got != 13 {
		t.Errorf("ScrollDown at mid moved highlight to %v, want 13", got)
	}

	list.SetCurrentItem(10)
	ilb.Draw(screen)
	if consumed, _ := handler(tview.MouseScrollUp, ev(), setFocus); !consumed {
		t.Error("ScrollUp at mid: consumed=false, want true")
	}
	if got := list.GetCurrentItem(); got != 7 {
		t.Errorf("ScrollUp at mid moved highlight to %v, want 7", got)
	}
}
