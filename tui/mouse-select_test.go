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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// fakeClipboard records WriteText calls for assertions.
type fakeClipboard struct {
	texts []string
}

// WriteText appends the text.
func (f *fakeClipboard) WriteText(text string) { f.texts = append(f.texts, text) }

// newSelectTest builds a tracker over a simulation screen pre-filled with the
// given rows, plus a fake clipboard.
func newSelectTest(t *testing.T, rows []string) (*selectionTracker, *fakeClipboard, tcell.Screen) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	// Lay each row out with real cell widths (wide runes take two columns).
	w := 0
	widths := make([][]int, len(rows))
	for y, row := range rows {
		x := 0
		for _, r := range row {
			rw := runewidth.RuneWidth(r)
			widths[y] = append(widths[y], rw)
			x += rw
		}
		if x > w {
			w = x
		}
	}
	h := len(rows)
	screen.SetSize(w+2, h+2)
	for y, row := range rows {
		x := 0
		for i, r := range []rune(row) {
			screen.SetContent(x, y+1, r, nil, tcell.StyleDefault)
			x += widths[y][i]
		}
	}
	screen.Show()

	fake := &fakeClipboard{}
	tr := newSelectionTracker(nil, tview.DoubleClickInterval)
	tr.screen = screen
	tr.app = &App{clipboard: fake}
	return tr, fake, screen
}

// fill paints the selection highlight the way the app-level after-draw hook
// would (assertions can then read the swapped styles).
func (s *selectionTracker) fill(screen tcell.Screen) { s.paintAfter(screen) }

// TestMouseDragSelectsRectangleAndCopies pins the drag flow: press, move,
// release — the on-screen text of the rectangular range is copied to the
// clipboard, rows are trimmed of trailing padding, and the drag motions plus
// the release are consumed so widgets never see them.
func TestMouseDragSelectsRectangleAndCopies(t *testing.T) {
	t.Parallel()

	tr, fake, screen := newSelectTest(t, []string{
		"✓ kMan_phone(open to chat)",
		"✓ MacMini",
		"✓ Linux-OMEN | ◷ 2 hops",
	})

	down := tcell.NewEventMouse(3, 1, tcell.ButtonPrimary, tcell.ModNone)
	ev, _ := tr.capture(down, tview.MouseLeftDown)
	if ev == nil {
		t.Fatal("MouseLeftDown must pass through to widgets")
	}
	// Motion with the button held: consumed + marks the selection.
	motion := tcell.NewEventMouse(12, 1, tcell.ButtonPrimary, tcell.ModNone)
	if ev, _ = tr.capture(motion, tview.MouseMove); ev != nil {
		t.Fatal("drag motion was not consumed")
	}
	if !tr.activeState() {
		t.Fatal("drag did not activate the selection")
	}
	// The highlight is painted after each frame.
	tr.fill(screen)

	// Release: consumed, and the clipboard holds the selected text.
	up := tcell.NewEventMouse(12, 1, tcell.ButtonNone, 0)
	if ev, _ = tr.capture(up, tview.MouseLeftUp); ev != nil {
		t.Fatal("selection release was not consumed (would fire a widget click)")
	}
	if len(fake.texts) != 1 {
		t.Fatalf("clipboard writes = %v, want 1", fake.texts)
	}
	if got, want := fake.texts[0], "Man_phone("; got != want {
		t.Errorf("copied text = %q, want %q", got, want)
	}
}

// TestMouseDragTopToBottom pins the "top to bottom" case: a multi-row drag
// copies every row's slice joined with newlines, trailing padding trimmed.
func TestMouseDragTopToBottom(t *testing.T) {
	t.Parallel()

	tr, fake, _ := newSelectTest(t, []string{
		"first row content",
		"second row content",
		"third row content",
	})

	tr.capture(tcell.NewEventMouse(0, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(16, 3, tcell.ButtonPrimary, tcell.ModNone), tview.MouseMove)
	tr.capture(tcell.NewEventMouse(16, 3, tcell.ButtonNone, 0), tview.MouseLeftUp)

	if len(fake.texts) != 1 {
		t.Fatalf("clipboard writes = %v, want 1", fake.texts)
	}
	// End column 16 (inclusive) clips the 18-char middle row at "conten".
	want := "first row content\nsecond row conten\nthird row content"
	if got := fake.texts[0]; got != want {
		t.Errorf("copied text = %q, want %q", got, want)
	}
}

// TestDoubleClickSelectsAndCopiesWord pins the double-click contract: a
// double-click on any cell of a displayed LXMF address selects AND copies the
// whole whitespace-delimited word with the prettyhexrep angle brackets
// dropped, so the pasted text is the bare address.
func TestDoubleClickSelectsAndCopiesWord(t *testing.T) {
	t.Parallel()

	addr := "2a6105f57145860441a62fe3b2a1352c"
	tr, fake, _ := newSelectTest(t, []string{
		"✓ MacMini",
		"  <" + addr + ">  (1)",
	})

	// Double-click on the middle of the hash (the fork synthesizes
	// MouseLeftDoubleClick for the second release).
	tr.capture(tcell.NewEventMouse(8, 2, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(8, 2, tcell.ButtonNone, 0), tview.MouseLeftUp)
	tr.capture(tcell.NewEventMouse(8, 2, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	if ev, _ := tr.capture(tcell.NewEventMouse(8, 2, tcell.ButtonNone, 0), tview.MouseLeftDoubleClick); ev != nil {
		t.Fatal("MouseLeftDoubleClick was not consumed")
	}

	if len(fake.texts) != 1 {
		t.Fatalf("clipboard writes = %v, want exactly one (select AND copy)", fake.texts)
	}
	if got := fake.texts[0]; got != addr {
		t.Errorf("double-click copied %q, want the bare address %q", got, addr)
	}
	// The HIGHLIGHT stays verbatim (the brackets are part of the selection).
	if tr.anchorX == tr.endX {
		t.Error("the word selection highlight collapsed to one cell")
	}
}

// TestDoubleClickOnNamePinsVerbatimCopy pins the complement: a word with no
// angle brackets is copied verbatim.
func TestDoubleClickOnNamePinsVerbatimCopy(t *testing.T) {
	t.Parallel()

	tr, fake, _ := newSelectTest(t, []string{"✓ Linux-OMEN | ◷ 2 hops"})

	tr.capture(tcell.NewEventMouse(3, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(3, 1, tcell.ButtonNone, 0), tview.MouseLeftUp)
	tr.capture(tcell.NewEventMouse(3, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(3, 1, tcell.ButtonNone, 0), tview.MouseLeftDoubleClick)

	if len(fake.texts) != 1 {
		t.Fatalf("clipboard writes = %v, want 1", fake.texts)
	}
	if got := fake.texts[0]; got != "Linux-OMEN" {
		t.Errorf("double-click copied %q, want %q", got, "Linux-OMEN")
	}
}

// TestTripleClickSelectsLine pins triple-click: the whole row is selected and
// copied (trailing padding trimmed).
func TestTripleClickSelectsLine(t *testing.T) {
	t.Parallel()

	tr, fake, _ := newSelectTest(t, []string{"✓ MacMini   "})

	for i := range 3 {
		tr.capture(tcell.NewEventMouse(2, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
		var action tview.MouseAction = tview.MouseLeftUp
		if i == 1 {
			action = tview.MouseLeftDoubleClick
		}
		tr.capture(tcell.NewEventMouse(2, 1, tcell.ButtonNone, 0), action)
	}

	// The double-click copy fires first, then the triple-click line copy
	// overwrites the clipboard — the FINAL content is the row.
	if len(fake.texts) != 2 {
		t.Fatalf("clipboard writes = %v, want 2 (word then line)", fake.texts)
	}
	if got := fake.texts[len(fake.texts)-1]; got != "✓ MacMini" {
		t.Errorf("triple-click copied %q, want %q (row, padding trimmed)", got, "✓ MacMini")
	}
}

// TestSingleClickPassesThrough pins the compat boundary: a plain click (no
// drag, no double-click) is forwarded unchanged and copies nothing, so every
// existing mouse interaction keeps working.
func TestSingleClickPassesThrough(t *testing.T) {
	t.Parallel()

	tr, fake, _ := newSelectTest(t, []string{"one two three"})

	down := tcell.NewEventMouse(1, 1, tcell.ButtonPrimary, tcell.ModNone)
	if ev, _ := tr.capture(down, tview.MouseLeftDown); ev != down {
		t.Fatal("single MouseLeftDown was consumed")
	}
	up := tcell.NewEventMouse(1, 1, tcell.ButtonNone, 0)
	if ev, _ := tr.capture(up, tview.MouseLeftUp); ev != up {
		t.Fatal("single MouseLeftUp was consumed")
	}
	if len(fake.texts) != 0 {
		t.Errorf("plain click copied %v; want no copies", fake.texts)
	}
	if tr.activeState() {
		t.Error("plain click must not leave a selection")
	}
}

// TestKeyDownClearsSelection pins the keyboard-abandons-selection behavior.
func TestKeyDownClearsSelection(t *testing.T) {
	t.Parallel()

	tr, fake, _ := newSelectTest(t, []string{"one two three"})

	tr.capture(tcell.NewEventMouse(0, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(5, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseMove)
	tr.capture(tcell.NewEventMouse(5, 1, tcell.ButtonNone, 0), tview.MouseLeftUp)
	if !tr.activeState() {
		t.Fatal("setup: selection should be active")
	}

	tr.clearOnKey()
	if tr.activeState() {
		t.Error("clearOnKey did not abandon the selection")
	}
	// And the abandoned selection does not re-copy.
	n := len(fake.texts)
	tr.capture(tcell.NewEventMouse(5, 1, tcell.ButtonNone, 0), tview.MouseLeftUp)
	if len(fake.texts) != n {
		t.Error("a stale Up after clear copied again")
	}
}

// TestWideRunesInSelection pins wide-rune handling: a CJK rune occupies two
// columns and is copied once (no duplicated or dropped characters).
func TestWideRunesInSelection(t *testing.T) {
	t.Parallel()

	tr, fake, _ := newSelectTest(t, []string{"日本語テスト"})

	tr.capture(tcell.NewEventMouse(0, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(9, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseMove)
	tr.capture(tcell.NewEventMouse(9, 1, tcell.ButtonNone, 0), tview.MouseLeftUp)

	if len(fake.texts) != 1 {
		t.Fatalf("clipboard writes = %v, want 1", fake.texts)
	}
	if got := fake.texts[0]; got != "日本語テス" {
		t.Errorf("wide-char selection = %q, want %q (complete runes only)", got, "日本語テス")
	}
}

// TestSelectionPaintSwapsColors pins the highlight painting: the after-draw
// hook swaps each selected cell's foreground/background and leaves the runes
// intact (so extraction still reads the original text).
func TestSelectionPaintSwapsColors(t *testing.T) {
	t.Parallel()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(10, 3)
	base := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	for x := range 6 {
		screen.SetContent(x, 1, rune('a'+x), nil, base)
	}
	screen.Show()

	tr := newSelectionTracker(nil, tview.DoubleClickInterval)
	tr.screen = screen
	tr.anchorX, tr.anchorY = 1, 1
	tr.endX, tr.endY = 3, 1
	tr.active = true
	tr.fill(screen)

	for x := 1; x <= 3; x++ {
		str, style, _ := screen.Get(x, 1)
		if want := string(rune('a' + x)); str != want {
			t.Errorf("cell (%v,1) rune = %q, want %q", x, str, want)
		}
		fg, bg, _ := style.Decompose()
		if fg != tcell.ColorBlack {
			t.Errorf("cell (%v,1) swapped fg = %v, want black (the original bg)", x, fg)
		}
		if bg != tcell.ColorWhite {
			t.Errorf("cell (%v,1) swapped bg = %v, want white (the original fg)", x, bg)
		}
	}
	// Outside the selection the style is untouched.
	if _, style, _ := screen.Get(0, 1); style != base {
		t.Errorf("cell (0,1) outside the selection changed: %v", style)
	}
}

// TestNormalizeCopiedWord pins the angle-bracket copy normalization edges.
func TestNormalizeCopiedWord(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"<2a6105f57145860441a62fe3b2a1352c>", "2a6105f57145860441a62fe3b2a1352c"},
		{"Linux-OMEN", "Linux-OMEN"},
		{"<not-a-pair", "<not-a-pair"},
		{"a>b<c>", "a>b<c>"},
		{"one\ntwo<>", "one\ntwo<>"},
	}
	for _, tt := range tests {
		if got := normalizeCopiedWord(tt.in); got != tt.want {
			t.Errorf("normalizeCopiedWord(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestClickCountTracking pins the double/triple window: same cell within the
// interval increments; a different cell resets.
func TestClickCountTracking(t *testing.T) {
	t.Parallel()

	tr := newSelectionTracker(nil, 500*time.Millisecond)
	tr.markClick(5, 5)
	tr.markClick(5, 5)
	if tr.clickCount != 2 {
		t.Errorf("clickCount after two same-cell clicks = %v, want 2", tr.clickCount)
	}
	tr.markClick(6, 5)
	if tr.clickCount != 1 {
		t.Errorf("clickCount after a different-cell click = %v, want 1", tr.clickCount)
	}
	tr.lastClickAt = time.Now().Add(-time.Second)
	tr.markClick(6, 5)
	if tr.clickCount != 1 {
		t.Errorf("clickCount after a stale click = %v, want 1", tr.clickCount)
	}
}

// clipboardSpy wraps a screen to capture SetClipboard payloads (the OSC 52
// path) while delegating everything else to the wrapped screen.
type clipboardSpy struct {
	tcell.Screen
	payload []byte
}

func (c *clipboardSpy) SetClipboard(data []byte) {
	c.payload = data
	c.Screen.SetClipboard(data)
}

// TestMouseDragAlsoPostsOSC52 is the regression test for fleet bug #11 (the
// glenn-mac-mini-m2 remote session): the system-clipboard write lands on the
// machine gonomadnet RUNS on — over SSH the remote pasteboard, which the
// local Cmd-V never sees. The selection must ALSO post OSC 52 through the
// terminal (tcell Screen.SetClipboard) so the escape travels app → tmux →
// outer terminal and sets the clipboard of the machine the user types on.
func TestMouseDragAlsoPostsOSC52(t *testing.T) {
	t.Parallel()

	tr, fake, screen := newSelectTest(t, []string{
		"✓ MacMini",
	})
	spy := &clipboardSpy{Screen: screen}
	tr.screen = spy

	tr.capture(tcell.NewEventMouse(0, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(9, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseMove)
	tr.capture(tcell.NewEventMouse(9, 1, tcell.ButtonNone, 0), tview.MouseLeftUp)

	if len(fake.texts) != 1 || fake.texts[0] != "✓ MacMini" {
		t.Fatalf("system clipboard writes = %v, want [✓ MacMini]", fake.texts)
	}
	if string(spy.payload) != "✓ MacMini" {
		t.Errorf("OSC 52 clipboard payload = %q, want %q (the terminal path must carry the selection)", spy.payload, "✓ MacMini")
	}
}

// TestDoubleClickAddressPostsOSC52 pins the double-click flow end to end: the
// displayed angle brackets are dropped on BOTH clipboard paths so the pasted
// value is the bare LXMF address.
func TestDoubleClickAddressPostsOSC52(t *testing.T) {
	t.Parallel()

	tr, fake, screen := newSelectTest(t, []string{
		"× <2a6105f57145860441a62fe3b2a1352c>",
	})
	spy := &clipboardSpy{Screen: screen}
	tr.screen = spy

	tr.capture(tcell.NewEventMouse(6, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDown)
	tr.capture(tcell.NewEventMouse(6, 1, tcell.ButtonPrimary, tcell.ModNone), tview.MouseLeftDoubleClick)

	if len(fake.texts) != 1 {
		t.Fatalf("clipboard writes = %v, want 1", fake.texts)
	}
	want := "2a6105f57145860441a62fe3b2a1352c"
	if fake.texts[0] != want {
		t.Errorf("system clipboard = %q, want bare address %q", fake.texts[0], want)
	}
	if string(spy.payload) != want {
		t.Errorf("OSC 52 payload = %q, want bare address %q (brackets must be dropped on both paths)", spy.payload, want)
	}
}
