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
)

// ctrlG is the Ctrl-G key event the room's dispatcher is offered.
func ctrlG() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone) }

// TestRoomCtrlGTypesGobotCommand pins the operator-requested [C-g] shortcut in
// the Channels room: with the composer's cursor at the very start of the entry
// line it types the chat-bot command in for the user. This is a deliberate
// Go-only addition — Python's room has no [C-g] binding at all.
func TestRoomCtrlGTypesGobotCommand(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")
	app.SetFocus(rw.editor)

	if text := rw.editor.GetText(); text != "" {
		t.Fatalf("the composer starts with %q, want empty", text)
	}
	if got, want := len([]rune(gobotCommandPrefix)), 11; got != want {
		t.Fatalf("gobotCommandPrefix is %v characters, want %v", got, want)
	}
	if gobotCommandPrefix != "/msg gobot " {
		t.Fatalf("gobotCommandPrefix = %q, want %q", gobotCommandPrefix, "/msg gobot ")
	}

	if got := rw.handleInput(ctrlG()); got != nil {
		t.Errorf("Ctrl-g was not consumed at the line start: returned %v", got)
	}
	if got, want := rw.editor.GetText(), "/msg gobot "; got != want {
		t.Errorf("composer = %q, want %q", got, want)
	}
	// The cursor must land after the command, so the next thing typed is the
	// question rather than a mangled command.
	if got, want := rw.editor.CursorPos(), 11; got != want {
		t.Errorf("cursor = %v, want %v", got, want)
	}
}

// TestRoomCtrlGOnlyFiresAtTheLineStart pins the guard the operator asked for:
// the shortcut works ONLY with the cursor at the very first entry position. Any
// other cursor position must leave the buffer untouched — inserting the command
// into a half-typed message would corrupt it — and must leave the key
// unconsumed so the room can bind it later.
func TestRoomCtrlGOnlyFiresAtTheLineStart(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")
	app.SetFocus(rw.editor)

	rw.editor.SetText("hello bob")
	rw.editor.SetCursorPos(5)
	if got := rw.handleInput(ctrlG()); got == nil {
		t.Error("Ctrl-g mid-text was consumed, want the event passed on")
	}
	if got, want := rw.editor.GetText(), "hello bob"; got != want {
		t.Errorf("composer = %q, want it untouched at %q", got, want)
	}
	if got, want := rw.editor.CursorPos(), 5; got != want {
		t.Errorf("cursor = %v, want it untouched at %v", got, want)
	}

	// At the very start with text already present the command goes in AT the
	// cursor, ahead of what is there.
	rw.editor.SetCursorPos(0)
	if got := rw.handleInput(ctrlG()); got != nil {
		t.Errorf("Ctrl-g at the line start was not consumed: returned %v", got)
	}
	if got, want := rw.editor.GetText(), "/msg gobot hello bob"; got != want {
		t.Errorf("composer = %q, want %q", got, want)
	}
	if got, want := rw.editor.CursorPos(), 11; got != want {
		t.Errorf("cursor = %v, want %v", got, want)
	}
}

// TestRoomCtrlGNeedsTheComposerFocused pins the other half of the guard: the
// command goes into the entry line the user is in, so a Ctrl-g from another pane
// must not write into a composer the user is not looking at. The shortcut bar
// advertises [C-g] in exactly the region where it fires.
func TestRoomCtrlGNeedsTheComposerFocused(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "test")

	app.SetFocus(rw.usersList)
	if got := rw.handleInput(ctrlG()); got == nil {
		t.Error("Ctrl-g was consumed while the users pane held focus")
	}
	if text := rw.editor.GetText(); text != "" {
		t.Errorf("the composer was written to from the users pane: %q", text)
	}

	// The body's own branch returns before the editor switch, so Ctrl-g there is
	// the message list's business.
	app.SetFocus(rw.messagesArea)
	if got := rw.handleInput(ctrlG()); got == nil {
		t.Error("Ctrl-g was consumed while the message body held focus")
	}
	if text := rw.editor.GetText(); text != "" {
		t.Errorf("the composer was written to from the message body: %q", text)
	}
}

// TestChannelsShortcutBarAdvertisesGobotCommand pins the bar text: the command
// is advertised at the END of the room's shortcut bar, the one that carries
// [C-d] Send.
func TestChannelsShortcutBarAdvertisesGobotCommand(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	rw := NewRoomWidget(app, "hub", "test")
	rw.OnFocusRegion = cd.setShortcutRegion
	cd.roomWidget = rw

	app.SetFocus(rw.editor)
	want := "[C-d] Send  [C-x] Leave  [F8] Collapse  [Tab] Complete Nick  [C-g] /msg gobot"
	if got := cd.GetShortcutText(); got != want {
		t.Errorf("editor-focused shortcut bar = %q, want %q", got, want)
	}

	// The cached-region fallback (no live room widget to query) must carry it
	// too, or the bar would lose the command whenever the room is rebuilt.
	cd.roomWidget = nil
	cd.shortcutFocus = "editor"
	if got := cd.GetShortcutText(); got != want {
		t.Errorf("cached-editor shortcut bar = %q, want %q", got, want)
	}
}
