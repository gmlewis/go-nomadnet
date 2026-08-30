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

// drawUrwidPrimitiveRow renders p on a fresh single-row simulation screen of
// the given width and returns the joined cell text of the row.
func drawUrwidPrimitiveRow(t *testing.T, p tview.Primitive, w int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(w, 1)
	p.SetRect(0, 0, w, 1)
	p.Draw(screen)
	var bu strings.Builder
	for x := range w {
		c, _, _, _ := cellContent(screen, x, 0)
		bu.WriteRune(c)
	}
	return bu.String()
}

// step is one expected focus-walk destination in the Peer Info modal
// navigation tests.
type step struct {
	typ  string
	desc string
}

// cursorRecorder wraps a tcell.Screen to capture ShowCursor calls (the plain
// tcell API exposes no cursor getter, so tests record them explicitly).
type cursorRecorder struct {
	tcell.Screen
	curX, curY int
	shown      bool
}

func (c *cursorRecorder) ShowCursor(x, y int) {
	c.curX, c.curY = x, y
	c.shown = true
}

// drawFocusedAt draws w on a simulation screen with focus set, wrapping the
// screen so the test can read back the ShowCursor position (cursor parity).
func drawFocusedAt(t *testing.T, p tview.Primitive, focus bool, width, height int) (*cursorRecorder, error) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	rec := &cursorRecorder{Screen: screen}
	screen.SetSize(width, height)
	p.SetRect(0, 0, width, height)
	if focus {
		p.Focus(func(pr tview.Primitive) {})
	}
	p.Draw(rec)
	return rec, nil
}

// TestUrwidCheckBoxRenderPinsPins urwid's CheckBox rendering ("[X] label" /
// "[ ] label", Conversations.py:890 urwid.CheckBox("Pin to top")) for the Peer
// Info "Pin to top" checkbox. tview.Checkbox renders only the bare label, so
// the dialog uses the faithful UrwidCheckBox port.
func TestUrwidCheckBoxRenderPinsCheckedUnchecked(t *testing.T) {
	t.Parallel()
	const label = "Pin to top"

	// Row = state glyph + space + label (4-column indicator, reserve_columns).
	wantSuffix := " " + label
	cases := []struct {
		checked bool
		glyph   string
	}{
		{true, "[X]"},
		{false, "[ ]"},
	}
	for _, tc := range cases {
		cb := NewUrwidCheckBox(label, tc.checked)
		got := strings.TrimRight(drawUrwidPrimitiveRow(t, cb, 20), " ")
		if !strings.HasPrefix(got, tc.glyph) || !strings.HasSuffix(got, wantSuffix) {
			t.Errorf("checked=%v render row = %q, want %q%s", tc.checked, got, tc.glyph, wantSuffix)
		}
	}
}

// TestUrwidCheckBoxFocusedCursorAtMiddleCell pins the hardware cursor on the
// middle cell of the state glyph (urwid SelectableIcon position 1) when the
// checkbox has focus, and no cursor when it does not.
func TestUrwidCheckBoxFocusedCursorAtMiddleCell(t *testing.T) {
	t.Parallel()
	cb := NewUrwidCheckBox("Pin to top", true)

	rec, err := drawFocusedAt(t, cb, true, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.shown || rec.curX != 1 || rec.curY != 0 {
		t.Errorf("focused ShowCursor = (%d,%d) shown=%v, want (1,0)", rec.curX, rec.curY, rec.shown)
	}

	// A fresh, never-focused checkbox must not show a cursor.
	cb2 := NewUrwidCheckBox("Pin to top", true)
	rec2, err := drawFocusedAt(t, cb2, false, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rec2.shown {
		t.Errorf("unfocused checkbox must not show a cursor (got (%d,%d))", rec2.curX, rec2.curY)
	}
}

// TestUrwidCheckBoxSpaceEnterToggle verifies Space/Enter toggle the state
// (urwid CheckBox.keypress maps " " and "enter" to toggle) and other runes are
// ignored; the change callback fires on user toggles.
func TestUrwidCheckBoxSpaceEnterToggle(t *testing.T) {
	t.Parallel()
	var changes []bool
	cb := NewUrwidCheckBox("Pin to top", true)
	cb.SetChangedFunc(func(checked bool) { changes = append(changes, checked) })

	h := cb.InputHandler()
	h(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(tview.Primitive) {})
	if cb.IsChecked() {
		t.Error("Space must uncheck a checked checkbox")
	}
	h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if !cb.IsChecked() {
		t.Error("Enter must toggle the checkbox back on")
	}
	h(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone), func(tview.Primitive) {})
	if !cb.IsChecked() {
		t.Error("non-space runes must be ignored")
	}
	if len(changes) != 2 || changes[0] || !changes[1] {
		t.Errorf("changes = %v, want [false true]", changes)
	}
}

// TestUrwidCheckBoxClickToggles pins left-click toggling (urwid CheckBox
// mouse handler).
func TestUrwidCheckBoxClickToggles(t *testing.T) {
	t.Parallel()
	cb := NewUrwidCheckBox("Pin to top", false)
	cb.SetRect(0, 0, 20, 1)

	ev := tcell.NewEventMouse(10, 0, tcell.ButtonPrimary, tcell.ModNone)
	mh := cb.MouseHandler()
	consumed, _ := mh(tview.MouseLeftClick, ev, func(tview.Primitive) {})
	if !consumed || !cb.IsChecked() {
		t.Errorf("left click consumed=%v checked=%v, want true/true", consumed, cb.IsChecked())
	}
}

// TestRadioButtonFocusedCursorAtMiddleCell pins the hardware cursor on the
// middle cell of the "(X)"/"( )" glyph for a focused RadioButton (urwid
// SelectableIcon cursor position 1) — the Peer Info modal's radio rows were
// previously invisible-cursor.
func TestRadioButtonFocusedCursorAtMiddleCell(t *testing.T) {
	t.Parallel()
	group := &DialogRadioGroup{}
	rb := NewRadioButton(group, "Trusted", true, false)
	cb := NewRadioButton(group, "Unknown", false, true) //nolint:staticcheck // group-quirk parity

	rec, err := drawFocusedAt(t, rb, true, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.shown || rec.curX != 1 || rec.curY != 0 {
		t.Errorf("focused ShowCursor = (%d,%d) shown=%v, want (1,0)", rec.curX, rec.curY, rec.shown)
	}

	rec, err = drawFocusedAt(t, cb, false, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rec.shown {
		t.Errorf("unfocused radio must not show a cursor (got (%d,%d))", rec.curX, rec.curY)
	}
}

// TestWireDialogNavDoesNotWrap pins urwid Pile traversal without wrap-around:
// Down on the last item and Up on the first item leave the focus in place
// (Python: urwid/widpile.py Pile.keypress returns the key unhandled at the
// edges; nothing else in the modal consumes up/down). The wrap-around was the
// Peer Info modal's original top-to-bottom focus bug.
func TestWireDialogNavDoesNotWrap(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	first := NewUrwidButton("First")
	mid := NewUrwidButton("Mid")
	last := NewUrwidButton("Last")
	items := []tview.Primitive{first, mid, last}
	wireDialogNav(app, nil, items)

	if app.GetFocus() != first {
		t.Fatalf("initial focus = %T, want the first item", app.GetFocus())
	}

	pressKeyThroughFocus(app, tcell.KeyUp)
	if got := app.GetFocus(); got != first {
		t.Errorf("Up at the first item moved focus to %T, want stay", got)
	}

	pressKeyThroughFocus(app, tcell.KeyDown)
	pressKeyThroughFocus(app, tcell.KeyDown)
	if got := app.GetFocus(); got != last {
		t.Fatalf("two Downs from the first item = %T, want the last item", got)
	}

	pressKeyThroughFocus(app, tcell.KeyDown)
	if got := app.GetFocus(); got != last {
		t.Errorf("Down at the last item moved focus to %T, want stay (no wrap)", got)
	}

	pressKeyThroughFocus(app, tcell.KeyUp)
	if got := app.GetFocus(); got != mid {
		t.Errorf("Up from the last item = %T, want the mid item", got)
	}
}

// TestPeerInfoDialogNavWalkAndEdges drives the real Peer Info modal: Down
// walks field → radios → checkbox → notes → action buttons → Save/Back, no
// wrap at the bottom, and Up returns through every widget.
func TestPeerInfoDialogNavWalkAndEdges(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)
	entry := PeerInfoEntry{
		SourceHash:  "712ffbfdb82c7fe60d0c5fa163ad2955",
		DisplayName: "glenn-mac-mini-m2",
		TrustLevel:  TrustTrusted,
		Pinned:      true,
	}
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{}, func(PeerInfoEntry) {})

	// Focus order after the pre-filled Name field: Copy, three trust radios,
	// two delivery radios, pin checkbox, notes, ping, block, LXMF, Save, Back.
	// Down at the last item must NOT wrap back to the top.
	wantSteps := []step{
		{"readline", "Copy"},
		{"radio", "trust 1"},
		{"radio", "trust 2"},
		{"radio", "trust 3"},
		{"radio", "delivery 1"},
		{"radio", "delivery 2"},
		{"checkbox", "pin"},
		{"readline", "Notes"},
		{"button", "Ping"},
		{"button", "Block"},
		{"button", "LXMF"},
		{"button", "Save"},
		{"button", "Back"},
	}
	for i, ws := range wantSteps {
		got := pressKeyThroughFocus(app, tcell.KeyDown)
		if !assertStep(t, got, ws) {
			t.Errorf("Down#%d focus = %T desc=%q, want %s", i+1, got, describePrimitive(got), ws.desc)
		}
	}
	if got := pressKeyThroughFocus(app, tcell.KeyDown); !isButton(got, "Back") {
		t.Errorf("Down at the last item wrapped to %T, want stay on Back", got)
	}
	if got := pressKeyThroughFocus(app, tcell.KeyUp); !isButton(got, "Save") {
		t.Errorf("Up from Back = %T, want Save", got)
	}
}

func assertStep(t *testing.T, p tview.Primitive, ws step) bool {
	t.Helper()
	switch ws.typ {
	case "readline":
		_, ok := p.(*ReadlineEdit)
		return ok
	case "radio":
		_, ok := p.(*RadioButton)
		return ok
	case "checkbox":
		_, ok := p.(*UrwidCheckBox)
		return ok
	case "button":
		return isButton(p, ws.desc)
	}
	return false
}

func describePrimitive(p tview.Primitive) string {
	switch v := p.(type) {
	case *ReadlineEdit:
		return "ReadlineEdit" + v.GetText()
	case *RadioButton:
		return "RadioButton " + v.Label()
	case *UrwidCheckBox:
		return "UrwidCheckBox"
	case *UrwidButton:
		return "UrwidButton " + v.Label()
	default:
		return "other"
	}
}

// TestPeerInfoDialogButtonRowLeftRight pins Left/Right movement within the
// horizontal button rows (Python urwid Columns.keypress: left/right move the
// focus between the Save/Back buttons; the Peer Info rows previously ignored
// them entirely).
func TestPeerInfoDialogButtonRowLeftRight(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)
	cd.ShowPeerInfoDialog(PeerInfoEntry{SourceHash: "712ffbfdb82c7fe60d0c5fa163ad2955"}, PeerInfoDialogHooks{}, func(PeerInfoEntry) {})

	// Walk to the Save button (12 Down presses from the initial Name field).
	for range 12 {
		pressKeyThroughFocus(app, tcell.KeyDown)
	}
	save := app.GetFocus()
	if !isButton(save, "Save") {
		t.Fatalf("focus after 12 Downs = %T, want Save", save)
	}

	// Left/Right must be routed through the button row (urwidColumns), which
	// finds the focused child and moves column focus. Down/Up arrive through
	// the same handler for the vertical pile walk.
	layout := cd.listSlotOverlay.Dialog().Content().(*tview.Flex)
	buttonsRow := layout.GetItem(17)
	buttonsRow.InputHandler()(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if got := app.GetFocus(); !isButton(got, "Back") {
		t.Errorf("Right in the Save/Back row = %T, want Back", got)
	}
	buttonsRow.InputHandler()(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if got := app.GetFocus(); !isButton(got, "Save") {
		t.Errorf("Left in the Save/Back row = %T, want Save", got)
	}

	// The actions row likewise moves Ping → Block → LXMF. Up from the Save
	// button first backtracks through the LXMF, Block, Ping buttons (the Pile
	// focus order runs straight through the buttons row), then Left/Right
	// exercise the row.
	actionsRow := layout.GetItem(14).(*urwidColumns)
	pressKeyThroughFocus(app, tcell.KeyUp)
	if got := app.GetFocus(); !isButton(got, "LXMF") {
		t.Fatalf("Up from Save = %T, want LXMF", got)
	}
	actionsRow.InputHandler()(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if got := app.GetFocus(); !isButton(got, "Block") {
		t.Errorf("Left in the Ping/Block/LXMF row = %T, want Block", got)
	}
	actionsRow.InputHandler()(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if got := app.GetFocus(); !isButton(got, "Ping") {
		t.Errorf("Left in the Ping/Block/LXMF row = %T, want Ping", got)
	}
	actionsRow.InputHandler()(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if got := app.GetFocus(); !isButton(got, "Block") {
		t.Errorf("Right in the Ping/Block/LXMF row = %T, want Block", got)
	}
}

// TestPeerInfoDialogSaveDismissesAndPersists pins Save semantics (Python
// confirmed(), Conversations.py:905-929): Enter on Save fires onSave with the
// edited entry, dismisses the modal, and Back dismisses without onSave.
func TestPeerInfoDialogSaveDismissesAndPersists(t *testing.T) {
	t.Parallel()
	saves := 0
	runDialog := func(t *testing.T) *ConversationsDisplay {
		app := newTestApp()
		app.Glyphs = GetGlyphSet(GlyphUnicode)
		cd := NewConversationsDisplay(app, nil)
		cd.ShowPeerInfoDialog(
			PeerInfoEntry{SourceHash: "712ffbfdb82c7fe60d0c5fa163ad2955"},
			PeerInfoDialogHooks{},
			func(PeerInfoEntry) { saves++ },
		)
		return cd
	}

	t.Run("Save", func(t *testing.T) {
		cd := runDialog(t)
		for range 12 {
			pressKeyThroughFocus(cd.app, tcell.KeyDown)
		}
		if !isButton(cd.app.GetFocus(), "Save") {
			t.Fatalf("focus = %T, want Save", cd.app.GetFocus())
		}
		pressKeyThroughFocus(cd.app, tcell.KeyEnter)
		if cd.listSlotOverlay != nil {
			t.Error("Enter on Save must dismiss the modal (still open)")
		}
		if saves != 1 {
			t.Errorf("onSave fired %d times, want 1", saves)
		}
	})

	t.Run("Back", func(t *testing.T) {
		cd := runDialog(t)
		for range 13 {
			pressKeyThroughFocus(cd.app, tcell.KeyDown)
		}
		if !isButton(cd.app.GetFocus(), "Back") {
			t.Fatalf("focus = %T, want Back", cd.app.GetFocus())
		}
		pressKeyThroughFocus(cd.app, tcell.KeyEnter)
		if cd.listSlotOverlay != nil {
			t.Error("Enter on Back must dismiss the modal (still open)")
		}
		if saves != 1 {
			t.Errorf("onSave fired %d times after Back, want still 1", saves)
		}
	})
}

// TestPeerInfoDialogUnknownPeerSection pins the unknown-peer section's PACK
// layout (Python Conversations.py:946-961): divider + centered g["info"] glyph
// line + blank + 4-row explainer (trailing "\n") + "Query network for keys"
// button + divider — 9 rows total, so the unknown-peer dialog is 8 rows taller
// than the known-peer one.
func TestPeerInfoDialogUnknownPeerSection(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)
	cd.ShowPeerInfoDialog(
		PeerInfoEntry{SourceHash: "712ffbfdb82c7fe60d0c5fa163ad2955"},
		PeerInfoDialogHooks{IsKnown: func(string) bool { return false }},
		func(PeerInfoEntry) {},
	)

	texts := dialogRowTexts(cd.listSlotOverlay.Dialog())
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "The identity of this peer is not known") {
		t.Error("unknown-peer dialog missing the explainer text")
	}
	if !dialogHasButton(cd.listSlotOverlay.Dialog(), "Query network for keys") {
		t.Error("unknown-peer dialog missing the Query network for keys button")
	}

	// The section must render the info glyph on its own centered line above
	// the explainer (Python urwid.Text(g["info"]+"\n", align=CENTER)).
	lines := strings.Split(joined, "\n")
	glyphIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == app.Glyphs["info"] {
			glyphIdx = i
			break
		}
	}
	if glyphIdx == -1 {
		t.Errorf("unknown-peer dialog missing the centered %q info glyph line", app.Glyphs["info"])
	} else if glyphIdx+1 >= len(lines) || strings.TrimSpace(lines[glyphIdx+1]) != "" {
		t.Errorf("row after the info glyph = %q, want the blank line from the trailing newline", lines[glyphIdx+1])
	}

	// Dialog height = 17 fixed rows + 9 known-section rows + 2 border. The
	// overlay computes the dialog rect during Draw, so render it first.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	screen.SetSize(52, 40)
	cd.listSlotOverlay.SetRect(0, 0, 52, 40)
	cd.listSlotOverlay.Draw(screen)
	want := 17 + 9 + 2
	if got := cdHeight(cd); got != want {
		t.Errorf("unknown-peer dialog height = %d, want %d (17+9+2)", got, want)
	}
}

// cdHeight returns the on-screen height of the conversations list-slot dialog.
func cdHeight(cd *ConversationsDisplay) int {
	_, _, _, h := cd.listSlotOverlay.Dialog().GetRect()
	return h
}

// TestPeerInfoDialogCheckboxPinsFieldValues pins that the Pin checkbox and the
// Notes field are live widgets whose values reach onSave (Python confirmed()
// reads cb_pin.state and e_notes.get_edit_text(), Conversations.py:919-920).
func TestPeerInfoDialogCheckboxPinsFieldValues(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)
	var gotEntry PeerInfoEntry
	cd.ShowPeerInfoDialog(
		PeerInfoEntry{SourceHash: "712ffbfdb82c7fe60d0c5fa163ad2955", Pinned: true, Notes: "hello notes"},
		PeerInfoDialogHooks{},
		func(e PeerInfoEntry) { gotEntry = e },
	)

	layout := cd.listSlotOverlay.Dialog().Content().(*tview.Flex)
	cbPin := layout.GetItem(11).(*UrwidCheckBox)
	if !cbPin.IsChecked() {
		t.Error("Pin checkbox should pre-check from the entry's pinned=true")
	}
	eNotes := layout.GetItem(12).(*ReadlineEdit)
	if eNotes.GetText() != "hello notes" {
		t.Errorf("Notes field = %q, want %q", eNotes.GetText(), "hello notes")
	}

	// Toggle Pin off and edit Notes, then Save via the keyboard path.
	for range 12 {
		pressKeyThroughFocus(app, tcell.KeyDown)
	}
	pressKeyThroughFocus(app, tcell.KeyEnter)
	if !gotEntry.Pinned {
		t.Error("saved entry Pinned = false, want true")
	}
	if gotEntry.Notes != "hello notes" {
		t.Errorf("saved entry Notes = %q, want %q", gotEntry.Notes, "hello notes")
	}
}
