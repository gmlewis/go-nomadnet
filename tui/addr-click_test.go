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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// testAddr is a full 16-byte Reticulum hash in lowercase hex — the shape every
// destination, identity and hub address has.
const testAddr = "ef289bf86e1f8108940a364a1c579f93"

// TestAddrSpan pins the rule that decides which text on a row is an address:
// RNS.prettyhexrep's <hex> form, or a bare 32-hex run standing alone between
// spaces. Nothing shorter qualifies — a truncated label is not something another
// device can be sent.
func TestAddrSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		text       string
		want       string
		start, end int
		ok         bool
	}{
		{
			name: "bracketed address",
			text: "Addr  : <" + testAddr + ">",
			want: testAddr, start: 8, end: 42, ok: true,
		},
		{
			name: "bare address at the end of the row",
			text: "Addr : " + testAddr,
			want: testAddr, start: 7, end: 39, ok: true,
		},
		{
			name: "centered line's indented address",
			text: "        <" + testAddr + ">",
			want: testAddr, start: 8, end: 42, ok: true,
		},
		{
			name: "half hash is not an address",
			text: "Addr : aabbccdd11223344",
		},
		{
			name: "short bracketed hash is not an address",
			text: "LXMF Addr : <deadbeef>",
		},
		{
			name: "truncated label is not an address",
			text: "<ef289bf86e1f8108…>",
		},
		{
			name: "browse URL's node part is not a bare address",
			text: "- " + testAddr + ":index.mu",
		},
		{
			name: "color tag before the address is not one",
			text: "[#bbbbbb]Address  : " + testAddr,
			want: testAddr, start: 20, end: 52, ok: true,
		},
		{
			name: "no address at all",
			text: "Name  : Alice",
		},
		{
			name: "empty row",
			text: "",
		},
		{
			name: "the first address wins",
			text: "<" + testAddr + "> <aabbccdd11223344ffeeddccbbaa9988>",
			want: testAddr, start: 0, end: 34, ok: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, start, end, ok := addrSpan(tc.text)
			if ok != tc.ok {
				t.Fatalf("addrSpan(%q) ok = %v, want %v", tc.text, ok, tc.ok)
			}
			if !ok {
				return
			}
			if got != tc.want || start != tc.start || end != tc.end {
				t.Errorf("addrSpan(%q) = (%q, %v, %v), want (%q, %v, %v)",
					tc.text, got, start, end, tc.want, tc.start, tc.end)
			}
		})
	}
}

// drawAddrRow lays a row out and draws it, so tview populates the region rects
// that a click resolves against.
func drawAddrRow(t *testing.T, p tview.Primitive, w, h int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
}

// clickPrimitive delivers a left press and click at (x, y) the way tview's event
// loop does: the same setFocus the application passes down.
func clickPrimitive(app *App, p tview.Primitive, x, y int) {
	setFocus := func(prim tview.Primitive) { app.SetFocus(prim) }
	ev := tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)
	mh := p.MouseHandler()
	mh(tview.MouseLeftDown, ev, setFocus)
	mh(tview.MouseLeftClick, ev, setFocus)
}

// addrTestApp builds an app with a recording clipboard and a focused primitive
// that the click must not displace permanently.
func addrTestApp(t *testing.T) (*App, *fakeClipboard, tview.Primitive) {
	t.Helper()
	app := newTestApp()
	fake := &fakeClipboard{}
	app.clipboard = fake
	// The app-level selection tracker is absent in tests, so a click reaches the
	// row's own handler directly.
	prev := tview.NewBox()
	app.SetFocus(prev)
	return app, fake, prev
}

// TestAddrRowClickCopiesAndOpensQRModal pins the whole feature on a plain row:
// a click on the address puts the BARE hex on the clipboard (no angle brackets,
// as normalizeCopiedWord does for a double-clicked address) and opens the modal
// that carries the QR code, and dismissing the modal hands the keyboard back to
// whatever had it before the click rather than leaving it on the text row.
func TestAddrRowClickCopiesAndOpensQRModal(t *testing.T) {
	t.Parallel()
	app, fake, prev := addrTestApp(t)

	row := tview.NewTextView().SetDynamicColors(false)
	MakeAddrClickable(app, "Node Address", row, "Addr : "+testAddr)
	drawAddrRow(t, row, 60, 1)

	// "Addr : " is seven columns, so the address begins at column 7.
	clickPrimitive(app, row, 7, 0)

	if len(fake.texts) != 1 || fake.texts[0] != testAddr {
		t.Fatalf("clipboard writes = %q, want exactly [%q]", fake.texts, testAddr)
	}
	if got := len(app.Dialogs.stack); got != 1 {
		t.Fatalf("open dialogs = %v, want 1", got)
	}
	app.Dialogs.DismissTop()
	if got := app.GetFocus(); got != prev {
		t.Errorf("focus after dismissing = %v, want the pre-click focus", got)
	}
}

// TestAddrRowClickOnLabelDoesNothing pins that only the address is a target: a
// click on the label copies nothing and opens nothing.
func TestAddrRowClickOnLabelDoesNothing(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)

	row := tview.NewTextView().SetDynamicColors(false)
	MakeAddrClickable(app, "Node Address", row, "Addr : "+testAddr)
	drawAddrRow(t, row, 60, 1)

	clickPrimitive(app, row, 2, 0)

	if len(fake.texts) != 0 {
		t.Errorf("clipboard writes = %q, want none", fake.texts)
	}
	if got := len(app.Dialogs.stack); got != 0 {
		t.Errorf("open dialogs = %v, want 0", got)
	}
}

// TestAddrRowRendersUnchanged pins the "no visual change" rule: making a row
// clickable must not alter a single cell of it. The region tags are zero-width,
// so the rendered text is the same as before.
func TestAddrRowRendersUnchanged(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	const text = "Addr : " + testAddr

	plain := tview.NewTextView().SetDynamicColors(false)
	plain.SetText(text)
	wired := tview.NewTextView().SetDynamicColors(false)
	MakeAddrClickable(app, "Node Address", wired, text)

	if got, want := wired.GetText(true), plain.GetText(true); got != want {
		t.Errorf("wired row text = %q, want the unmodified %q", got, want)
	}
}

// TestAddrRowKeepsAnExistingMouseCapture pins that wiring a row does not cost it
// a capture it already had: the Hub Info view installs a wheel multiplier before
// its address click is wired, and scrolling must survive. It also pins that
// refreshing the text does NOT re-wire: each refresh that composed another layer
// would deliver one wheel notch to the multiplier once per refresh, so the count
// after three refreshes is the assertion that matters.
func TestAddrRowKeepsAnExistingMouseCapture(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	tv := tview.NewTextView().SetDynamicColors(true)
	wheelCalls := 0
	tv.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action != tview.MouseScrollUp && action != tview.MouseScrollDown {
			return action, event // the wheel multiplier passes everything else along
		}
		wheelCalls++
		return tview.MouseConsumed, nil
	})

	MakeAddrClickable(app, "Hub Address", tv, "  Address  : "+testAddr)
	for range 3 {
		SetAddrRowText(tv, "  Address  : "+testAddr)
	}

	setFocus := func(tview.Primitive) {}
	ev := tcell.NewEventMouse(0, 0, tcell.WheelDown, tcell.ModNone)
	mh := tv.MouseHandler()
	consumed, _ := mh(tview.MouseScrollDown, ev, setFocus)

	if !consumed {
		t.Error("wheel notch was not consumed, want the row's own capture to handle it")
	}
	if wheelCalls != 1 {
		t.Errorf("wheel capture calls = %v, want exactly 1 (a re-wired row nests one layer per refresh)", wheelCalls)
	}
}

// TestAddrRowRefreshCopiesTheNewAddress pins that a row whose text is replaced
// stays clickable and copies what it now shows — the announce-driven refresh
// paths (Local Peer Info, Hub Info, the conversation peer bar) all do this.
func TestAddrRowRefreshCopiesTheNewAddress(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)
	const newerAddr = "00112233445566778899aabbccddeeff"

	tv := tview.NewTextView().SetDynamicColors(false)
	MakeAddrClickable(app, "Node Address", tv, "Addr : "+testAddr)
	SetAddrRowText(tv, "Addr : "+newerAddr)
	drawAddrRow(t, tv, 60, 1)

	clickPrimitive(app, tv, 7, 0)

	if len(fake.texts) != 1 || fake.texts[0] != newerAddr {
		t.Errorf("clipboard writes = %q, want exactly [%q]", fake.texts, newerAddr)
	}
}

// TestShowAddressActionsModalShowsAddressAndCopy pins what the modal says: the
// address in text below the code (so it can be typed by hand when no camera can
// read the code) and the confirmation that the click already copied it. The
// angle brackets the panels display the address in are not copied.
func TestShowAddressActionsModalShowsAddressAndCopy(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)

	app.ShowAddressActions("Node Address", "<"+testAddr+">")

	if len(fake.texts) != 1 || fake.texts[0] != testAddr {
		t.Fatalf("clipboard writes = %q, want exactly the bare [%q]", fake.texts, testAddr)
	}
	if n := len(app.Dialogs.stack); n != 1 {
		t.Fatalf("open dialogs = %v, want 1", n)
	}
	rows := strings.Join(dialogRowTexts(app.Dialogs.stack[0].dialog), "\n")
	for _, want := range []string{"< " + testAddr + " >", "Copied to clipboard"} {
		if !strings.Contains(rows, want) {
			t.Errorf("modal rows:\n%v\nmissing %q", rows, want)
		}
	}
}

// TestShowAddressActionsWithoutAnAddressDoesNothing pins the guard: nothing is
// copied and no modal opens when there is no address to act on.
func TestShowAddressActionsWithoutAnAddressDoesNothing(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)

	app.ShowAddressActions("Node Address", "  ")

	if len(fake.texts) != 0 {
		t.Errorf("clipboard writes = %q, want none", fake.texts)
	}
	if n := len(app.Dialogs.stack); n != 0 {
		t.Errorf("open dialogs = %v, want 0", n)
	}
}

// TestCenterAddrLineClickCopiesAndOpensQRModal pins the same feature on the
// centered LXMF propagation line, whose widget draws its own urwid wrap: the
// click is resolved against that wrap (including the centering pad) and consumed
// before the widget could take focus.
func TestCenterAddrLineClickCopiesAndOpensQRModal(t *testing.T) {
	t.Parallel()
	app, fake, prev := addrTestApp(t)

	// 8 leading spaces (the indent the wrapped PN line carries) + the bracketed
	// address: 42 cells centered in 50 columns puts the row's first cell at
	// column 4, so the address's "<" is at column 12.
	line := NewUrwidCenterText("        <" + testAddr + ">")
	line.SetRect(0, 0, 50, 1)
	MakeCenterAddrClickable(app, "Node Address", line)

	clickPrimitive(app, line, 12, 0)

	if len(fake.texts) != 1 || fake.texts[0] != testAddr {
		t.Fatalf("clipboard writes = %q, want exactly [%q]", fake.texts, testAddr)
	}
	if got := len(app.Dialogs.stack); got != 1 {
		t.Fatalf("open dialogs = %v, want 1", got)
	}
	app.Dialogs.DismissTop()
	if got := app.GetFocus(); got != prev {
		t.Errorf("focus after dismissing = %v, want the pre-click focus", got)
	}
}

// TestCenterAddrLineClickOffTheAddressDoesNothing pins the hit test around the
// address: the centering pad to its left and the indent cells inside the row are
// not targets.
func TestCenterAddrLineClickOffTheAddressDoesNothing(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)

	line := NewUrwidCenterText("        <" + testAddr + ">")
	line.SetRect(0, 0, 50, 1)
	MakeCenterAddrClickable(app, "Node Address", line)

	clickPrimitive(app, line, 3, 0) // one cell left of the row's first cell

	if len(fake.texts) != 0 {
		t.Errorf("clipboard writes = %q, want none", fake.texts)
	}
	if got := len(app.Dialogs.stack); got != 0 {
		t.Errorf("open dialogs = %v, want 0", got)
	}
}

// TestCenterAddrLineClickOnWrappedRow pins the wrapped case: the propagation
// address lands on the line's second row, and a click there must still resolve —
// the row that carries the address is not the row that starts it.
func TestCenterAddrLineClickOnWrappedRow(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)

	// 34 columns: urwid's space wrap breaks after "is", leaving the bracketed
	// address alone on row 2 (34 cells, so no centering pad).
	const text = "LXMF Propagation Node Address is <" + testAddr + ">"
	if got := urwidSpaceWrap(text, 34); len(got) != 2 || got[1] != "<"+testAddr+">" {
		t.Fatalf("wrap = %q, want the address alone on row 2", got)
	}

	line := NewUrwidCenterText(text)
	line.SetRect(0, 0, 34, 2)
	MakeCenterAddrClickable(app, "Node Address", line)

	clickPrimitive(app, line, 0, 1)

	if len(fake.texts) != 1 || fake.texts[0] != testAddr {
		t.Fatalf("clipboard writes = %q, want exactly [%q]", fake.texts, testAddr)
	}
	if got := len(app.Dialogs.stack); got != 1 {
		t.Errorf("open dialogs = %v, want 1", got)
	}
}
