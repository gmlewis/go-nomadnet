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
	"github.com/gmlewis/tview"
)

// clickRoomComposer lays the room out and clicks the composer, the way a tap
// does on the appliance: it goes through the room's own mouse chain (the
// columns Flex, which is rw.widget) rather than calling the editor's handler
// directly, so the focus the click asks for is the one production records.
// It returns the primitive the field asked to focus.
func clickRoomComposer(t *testing.T, app *App, rw *RoomWidget) tview.Primitive {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(100, 37)

	widget := rw.Widget().(*tview.Flex)
	widget.SetRect(0, 0, 100, 37)
	widget.Draw(screen)

	x, y, w, _ := rw.editor.GetInnerRect()
	if w <= 1 {
		t.Fatalf("the composer was laid out %v cells wide, want room for a click", w)
	}

	var asked tview.Primitive
	setFocus := func(p tview.Primitive) {
		app.SetFocus(p)
		asked = p
	}
	ev := tcell.NewEventMouse(x+w/2, y, tcell.Button1, tcell.ModNone)
	consumed, _ := widget.MouseHandler()(tview.MouseLeftDown, ev, setFocus)
	if !consumed {
		t.Fatalf("the click on the composer at (%v,%v) was not consumed by the room", x+w/2, y)
	}
	return asked
}

// TestRoomComposerClickFocusesTheEditor pins what a tap on the composer must
// leave behind: the composer focused, under the identity the rest of the port
// knows it by.
//
// tview routes a click into the clicked widget's own handler, and a click on an
// InputField ends in the inner TextArea it wraps, whose handler focuses itself
// (tview textarea.go MouseHandler). Every focus check in the port is written
// against the ReadlineEdit — the room's editorHasFocus, the conversation's
// composerHasFocus, MainDisplay.fieldEditor — so the click must resolve to it.
func TestRoomComposerClickFocusesTheEditor(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")

	if asked := clickRoomComposer(t, app, rw); asked != tview.Primitive(rw.editor) {
		t.Errorf("a click on the composer asked to focus %T, want the *ReadlineEdit", asked)
	}
	if got := app.GetFocus(); got != tview.Primitive(rw.editor) {
		t.Errorf("after a click on the composer, app.GetFocus() = %T, want the *ReadlineEdit: "+
			"every focus comparison in the port is written against it", got)
	}
	if !rw.editorHasFocus() {
		t.Error("editorHasFocus() is false after a click on the composer; " +
			"the composer is what the click focused and what a keystroke goes to")
	}
}

// TestRoomCtrlGWorksAfterClickingTheComposer pins that the shortcut the room's
// own shortcut bar advertises works on a composer reached by tapping it.
//
// The shortcut is guarded (the command is typed only at the start of an empty
// line), and both halves of that guard — the focus and the cursor — hold after
// a tap: the caret sits at the line start.
func TestRoomCtrlGWorksAfterClickingTheComposer(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")

	clickRoomComposer(t, app, rw)

	if got := rw.editor.CursorPos(); got != 0 {
		t.Fatalf("the click left the composer's cursor at %v, want 0 — this test is about the focus", got)
	}
	if ret := rw.handleInput(ctrlG()); ret != nil {
		t.Errorf("Ctrl-g was not consumed after a click on the composer: returned %v", ret)
	}
	if got, want := rw.editor.GetText(), gobotCommandPrefix; got != want {
		t.Errorf("after a click on the composer, Ctrl-g typed %q, want %q", got, want)
	}
}

// TestComposerClickKeepsTheFieldsFirstRefusal pins the field-first key
// ordering: MainDisplay hands every key to the focused field before any page
// shortcut runs, so the readline editing keys reach the field the reader is
// typing in (MainDisplay.handleInput's fieldEditor). That hand-off is a
// *ReadlineEdit assertion, so a click must leave that identity in place.
func TestComposerClickKeepsTheFieldsFirstRefusal(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")

	clickRoomComposer(t, app, rw)

	md := &MainDisplay{app: app}
	fe := md.fieldEditor()
	switch {
	case fe == nil:
		t.Error("fieldEditor() = nil after a click on the composer: " +
			"the field must keep the key it is typed into")
	case fe != rw.editor:
		t.Error("fieldEditor() is not the room's composer after a click on it")
	}
}
