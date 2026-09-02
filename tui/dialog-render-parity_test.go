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

// Regression tests for the 2026-09-01 fleet usability batch (bugs #2–#5):
// wheel scrolling on the [ Interfaces ] page, the Ctrl-p QR dialog's cut-off
// bottom and dead mouse Close, the chopped ingest-result message, the chopped
// no-trusted-nodes sync dialog, and the fixed-height confirm dialog.

// drawRows renders a primitive on a wxh simulation screen and returns the
// joined cell text per row (the headless parity capture technique).
func drawRows(t *testing.T, p tview.Primitive, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
	screen.Sync()
	rows := make([]string, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// TestInterfacesListWheelMovesFocus is the regression test for the fleet
// report "mouse-wheel scrolling does NOT work on the [ Interfaces ] page":
// Python's urwid ListBox maps the wheel to "up"/"down" keypresses, so the
// FOCUSED item moves and the viewport follows (Interfaces.py:2911
// SimpleFocusListWalker). The previous offset-only wheel was a no-op — Draw's
// scrollIntoView re-pins the viewport to the focused item every frame — so the
// wheel never scrolled the list.
func TestInterfacesListWheelMovesFocus(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	infos := make([]InterfaceInfo, 0, 13)
	for i := range 13 {
		infos = append(infos, InterfaceInfo{
			Name: "if" + string(rune('a'+i)), Type: "TCPClientInterface", Status: "disconnected", Enabled: true,
		})
	}
	id := NewInterfacesDisplay(app, infos)
	id.listBox.SetRect(0, 0, 100, 35)
	mh := id.listBox.MouseHandler()
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	// Focus the first item with a click (the wheel then moves the focus).
	click := tcell.NewEventMouse(5, 3, tcell.Button1, tcell.ModNone)
	if consumed, _ := mh(tview.MouseLeftClick, click, setFocus); !consumed {
		t.Fatalf("click not consumed")
	}
	if got := id.SelectedIndex(); got != 0 {
		t.Fatalf("after click SelectedIndex = %d, want 0", got)
	}

	// Wheel down ×5: the focus advances one item per notch.
	for i := range 5 {
		dn := tcell.NewEventMouse(5, 10, tcell.WheelDown, tcell.ModNone)
		if consumed, _ := mh(tview.MouseScrollDown, dn, setFocus); !consumed {
			t.Fatalf("wheel down %d not consumed", i)
		}
	}
	if got := id.SelectedIndex(); got != 5 {
		t.Errorf("after 5 wheel-downs SelectedIndex = %d, want 5 (urwid wheel = down keypress)", got)
	}

	// Wheel up returns toward the top.
	for range 2 {
		up := tcell.NewEventMouse(5, 3, tcell.WheelUp, tcell.ModNone)
		mh(tview.MouseScrollUp, up, setFocus)
	}
	if got := id.SelectedIndex(); got != 3 {
		t.Errorf("after 5 down + 2 up, SelectedIndex = %d, want 3", got)
	}

	// The wheel at the top boundary does not wrap or panic.
	for range 10 {
		up := tcell.NewEventMouse(5, 3, tcell.WheelUp, tcell.ModNone)
		mh(tview.MouseScrollUp, up, setFocus)
	}
	if got := id.SelectedIndex(); got != 0 {
		t.Errorf("SelectedIndex after wheel-up at top = %d, want 0", got)
	}
}

// TestIngestResultMessageNotChopped is the regression test for the chopped
// ingest error ("Could ingest LXM from URI data. Check your inp"): urwid PACK
// wraps the message at the dialog's inner width and grows the Pile, so the
// full text must render on its wrapped rows with the OK row inside the border.
func TestIngestResultMessageNotChopped(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	cd.ShowIngestResult(IngestError)
	if cd.listSlotOverlay == nil {
		t.Fatalf("ingest result dialog not shown")
	}
	rows := contentRows(t, cd, 80, 24)
	if !anyRowContains(rows, "Check your") {
		t.Fatalf("ingest error message truncated: %q", rows)
	}
	if !anyRowContains(rows, "input.") {
		t.Errorf("ingest error message missing its wrapped tail %q (rows: %v)", "input.", rows)
	}
	// The OK button row must sit INSIDE the dialog border: the border's bottom
	// corner is drawn on a row after the OK row.
	okRow, borderRow := -1, -1
	for i, r := range rows {
		if okRow < 0 && strings.Contains(r, "< OK") {
			okRow = i
		}
		if okRow >= 0 && strings.Contains(r, "└") {
			borderRow = i
			break
		}
	}
	if okRow < 0 {
		t.Fatalf("OK button not rendered: %v", rows)
	}
	if borderRow < 0 || borderRow <= okRow {
		t.Errorf("OK row %d not above the dialog border row %d (chopped bottom)", okRow, borderRow)
	}
}

// TestSyncDialogNoTrustedNodesFullHeight is the regression test for the same
// sizing-bug class on the Message Sync dialog's no-trusted-nodes variant: the
// layout was sized from GetItemCount (items, not rows), chopping the dialog
// five rows short and overflowing the wrapped explainer over the border.
func TestSyncDialogNoTrustedNodesFullHeight(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)
	cd.ShowSyncDialog("", nil, SyncDialogHooks{}, nil)
	if cd.listSlotOverlay == nil {
		t.Fatalf("sync dialog not shown")
	}
	rows := contentRows(t, cd, 80, 24)
	// The explainer's LAST line must render above the dialog's bottom border.
	// (urwid's space wrap splits the final sentence as "…or the manually" /
	// "selected one." — matching the running Python build's render — so the
	// assertion targets the last wrapped line, not the full phrase.)
	if !anyRowContains(rows, "selected one.") {
		t.Errorf("sync explainer text cut off (rows: %v)", rows)
	}
	closeRow, borderRow := -1, -1
	for i, r := range rows {
		if strings.Contains(r, "< Close") {
			closeRow = i
		}
		if strings.Contains(r, "└") {
			borderRow = i
		}
	}
	if closeRow < 0 {
		t.Fatalf("Close button not rendered — dialog chopped: %v", rows)
	}
	if borderRow < 0 {
		t.Fatalf("sync dialog bottom border not drawn — dialog chopped")
	}
	if borderRow <= closeRow {
		t.Errorf("Close row %d renders on/after the dialog bottom border row %d — dialog too short", closeRow, borderRow)
	}
}

// TestConfirmDialogSizesToWrappedMessage pins ShowConfirmDialog against the
// fixed 3-row message cap: callers pass arbitrary-length text (e.g. "Could
// not open LXMF link: <error>"), which urwid PACK wraps and grows for.
func TestConfirmDialogSizesToWrappedMessage(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	long := strings.Repeat("word ", 30)
	app.Dialogs.ShowConfirmDialog(long, nil, nil)
	if app.Dialogs.Count() != 1 {
		t.Fatalf("confirm dialog not on the stack")
	}
	rows := drawRows(t, app.Dialogs.Pages(), 80, 24)
	joined := strings.Join(rows, "\n")
	// 30 "word " words cannot fit one line at the dialog width: the tail must
	// be present, which the fixed 3-row cap chopped.
	if !anyRowContains(rows, "word word") {
		t.Fatalf("confirm dialog not rendered: %v", rows)
	}
	if got := strings.Count(joined, "word"); got < 30 {
		t.Errorf("long confirm message truncated: %d of 30 words visible", got)
	}
}

// TestQRDialogBottomInsideBorder is the regression test for the Ctrl-p QR
// dialog's cut-off bottom (bug #4) and dead mouse Close (bug #3): the PACK
// height omitted the 2 border rows, so the Close button rendered ON the
// border row with a collapsed rect that could never receive a click.
func TestQRDialogBottomInsideBorder(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)
	cd.ShowMyQRDialog("2a6105f57145860441a62fe3b2a1352c")
	if cd.fullSlotOverlay == nil {
		t.Fatalf("QR dialog not shown")
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(156, 37)
	cd.outer.SetRect(0, 0, 156, 37)
	cd.outer.Draw(screen)
	screen.Sync()

	closeRow, borderRow := -1, -1
	for y := range 37 {
		var b strings.Builder
		for x := range 156 {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		if closeRow < 0 && strings.Contains(b.String(), "< Close") {
			closeRow = y
		}
		if strings.Contains(b.String(), "└") && borderRow < 0 && closeRow >= 0 {
			// The QR dialog's own bottom border (first └ row at/after the
			// Close row's search window).
			borderRow = y
		}
	}
	if closeRow < 0 {
		t.Fatalf("Close button not rendered")
	}
	if borderRow < 0 {
		t.Fatalf("QR dialog bottom border not rendered — dialog chopped")
	}
	if borderRow <= closeRow {
		t.Errorf("Close row %d sits on/after the dialog bottom border row %d — PACK height is 2 rows short", closeRow, borderRow)
	}

	// A mouse click on the rendered Close button must dismiss the dialog
	// (previously the button's rect collapsed, so clicks never hit it). The
	// Close button is the centered 12-wide column of the button row, so click
	// at the dialog's horizontal center on the button row (one above the
	// bottom border, after the trailing blank row).
	dialog := cd.fullSlotOverlay.Dialog()
	dx, dy, dw, dh := dialog.GetRect()
	clickY := dy + dh - 3
	ev := tcell.NewEventMouse(dx+dw/2, clickY, tcell.Button1, tcell.ModNone)
	consumed, _ := dialog.MouseHandler()(tview.MouseLeftClick, ev, func(p tview.Primitive) {})
	if !consumed {
		t.Fatalf("mouse click on the QR Close button was not consumed (dead click target)")
	}
	if cd.fullSlotOverlay != nil {
		t.Errorf("clicking the Close button did not dismiss the QR dialog")
	}
}
