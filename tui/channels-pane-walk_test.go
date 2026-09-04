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

// The Channels Left/Right pane walk mirrors the Python original's urwid
// Columns focus movement: list ⇄ room ⇄ users, where the room's composer
// lets a plain Left/Right through ONLY at the text boundaries (urwid
// widget/edit.py keypress) and the room's frame part (body/footer,
// Channels.py:511-546) persists across focus leaves. The user-visible walk
// (Bug#4): the bright highlight sits on the focused pane's selected row, the
// left list drops to its darker off-focus highlight while the cursor is in
// the room, the Users pane bright-highlights its selected member, and every
// Left steps back through the same path.

// walkCD builds a ChannelsDisplay with a connected hub and an open room, the
// state the fleet runs when the user drives the pane walk.
func walkCD(t *testing.T) (*App, *ChannelsDisplay) {
	t.Helper()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)
	return app, cd
}

// walkKey dispatches one key through the channels display's real input chain
// (the same path a live keystroke takes: cd.content's capture runs first,
// then the focused child's handlers).
func walkKey(cd *ChannelsDisplay, key tcell.Key, mod tcell.ModMask) {
	ev := tcell.NewEventKey(key, 0, mod)
	cd.content.InputHandler()(ev, func(p tview.Primitive) { cd.app.SetFocus(p) })
}

// walkIsFocused reports which region holds focus, for assertions.
func walkRegion(app *App, cd *ChannelsDisplay) string {
	f := app.GetFocus()
	switch {
	case f == tview.Primitive(cd.ilb):
		return "list"
	case cd.roomWidget != nil && f == tview.Primitive(cd.roomWidget.usersList):
		return "users"
	case cd.roomWidget != nil && f == tview.Primitive(cd.roomWidget.messagesArea):
		return "body"
	case cd.roomWidget != nil && f == tview.Primitive(cd.roomWidget.editor):
		return "editor"
	}
	return "other"
}

// TestWalkListToRoomToUsers pins the user's Bug#4 sequence: from the
// Channels list, Right enters the room (the composer — the hardware cursor
// region), another Right enters the Users pane, and each Left steps back the
// same way.
func TestWalkListToRoomToUsers(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	app.SetFocus(cd.ilb)

	// Right from the list: into the room's composer.
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("Right from the list focus = %v, want the room composer", got)
	}

	// Right from the composer (empty text, cursor at the end): the Users pane.
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "users" {
		t.Fatalf("Right from the composer focus = %v, want the users pane", got)
	}

	// Left from the users pane: back to the composer; Left again: the list.
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("Left from the users focus = %v, want the room composer", got)
	}
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "list" {
		t.Fatalf("Left from the composer focus = %v, want the channels list", got)
	}
}

// TestWalkBoundaries pins the walk's ends: Left in the list and Right in the
// users pane are no-ops (Python's Columns drops the unhandled key at the
// boundaries), and Right from the list with no room open is a no-op too.
func TestWalkBoundaries(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	app.SetFocus(cd.ilb)

	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "list" {
		t.Fatalf("Left in the list moved focus to %v, want it to stay", got)
	}

	// Users pane is the right boundary.
	app.SetFocus(cd.roomWidget.usersList)
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "users" {
		t.Fatalf("Right in the users pane moved focus to %v, want it to stay", got)
	}

	// Placeholder (nothing open): Right from the list is a no-op.
	cd.paneMode = ""
	cd.ShowPlaceholder()
	app.SetFocus(cd.ilb)
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "list" {
		t.Fatalf("Right from the list with a placeholder moved focus to %v, want it to stay", got)
	}
}

// TestWalkComposerBoundary pins urwid Edit's boundary propagation (urwid
// widget/edit.py: Left leaves the composer only at position 0, Right only at
// the end of the text; mid-text the key moves the composer's cursor instead).
func TestWalkComposerBoundary(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	rw := cd.roomWidget
	rw.editor.SetText("hello")
	rw.editor.SetCursorPos(3) // mid-text
	app.SetFocus(rw.editor)

	// Right mid-text: the composer keeps focus and the cursor advances.
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("Right mid-text focus = %v, want the composer (cursor movement)", got)
	}
	if got := rw.editor.CursorPos(); got != 4 {
		t.Errorf("Right mid-text cursor = %v, want 4", got)
	}

	// Left mid-text: cursor movement, the composer keeps focus.
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := rw.editor.CursorPos(); got != 3 {
		t.Errorf("Left mid-text cursor = %v, want 3", got)
	}
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("Left mid-text focus = %v, want the composer", got)
	}

	// At the very end, Right walks into the users pane.
	rw.editor.SetCursorPos(5)
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "users" {
		t.Fatalf("Right at the text end focus = %v, want the users pane", got)
	}

	// At position 0, Left walks back to the channels list.
	rw.editor.SetCursorPos(0)
	app.SetFocus(rw.editor)
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "list" {
		t.Fatalf("Left at position 0 focus = %v, want the channels list", got)
	}
}

// TestWalkRoomPartPersists pins Python RoomFrame's persistent focus_part
// (Channels.py:511-546): the walk returns to the part the room was on — body
// → users → Left returns to the body; composer → users → Left returns to the
// composer.
func TestWalkRoomPartPersists(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	rw := cd.roomWidget

	// From the message body: Right to the users pane and back lands on the body.
	app.SetFocus(rw.messagesArea)
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "users" {
		t.Fatalf("Right from the body focus = %v, want the users pane", got)
	}
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "body" {
		t.Fatalf("Left from the users focus = %v, want the message body (the preserved part)", got)
	}

	// From the composer: the same walk returns to the composer.
	app.SetFocus(rw.editor)
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("Left from the users focus = %v, want the composer", got)
	}

	// The part also survives a round trip through the channels list.
	app.SetFocus(rw.messagesArea)
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	if got := walkRegion(app, cd); got != "list" {
		t.Fatalf("Left from the body focus = %v, want the channels list", got)
	}
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "body" {
		t.Fatalf("Right from the list focus = %v, want the message body (the preserved part)", got)
	}
}

// TestWalkUsersTabToComposer pins Python UsersBox.keypress tab
// (Channels.py:313-321): Tab from the users pane jumps to the room's footer
// composer.
func TestWalkUsersTabToComposer(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	app.SetFocus(cd.roomWidget.usersList)

	walkKey(cd, tcell.KeyTab, tcell.ModNone)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("Tab from the users pane focus = %v, want the composer", got)
	}
}

// TestWalkListLeftBoundaryNoScroll pins that the walk-boundary keys never
// reach the fork List's horizontal scrolling: with the list focused, Left and
// Right leave the rendered channels list byte-identical (no horizontal
// shift) and the focus unchanged.
func TestWalkListLeftBoundaryNoScroll(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	app.SetFocus(cd.ilb)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(120, 30)
	drawLeftPanel := func() string {
		screen.Clear()
		cd.widget.SetRect(0, 0, 120, 30)
		cd.widget.Draw(screen)
		screen.Sync()
		var sb strings.Builder
		for y := range 30 {
			for x := range 36 {
				str, _, _ := screen.Get(x, y)
				sb.WriteString(str)
			}
		}
		return sb.String()
	}

	// Left in the list is the walk's left boundary: no scroll, focus stays.
	before := drawLeftPanel()
	walkKey(cd, tcell.KeyLeft, tcell.ModNone)
	after := drawLeftPanel()
	if before != after {
		t.Error("the channels list scrolled horizontally on the Left boundary key; Python's Columns drops it unhandled")
	}
	if got := walkRegion(app, cd); got != "list" {
		t.Errorf("focus after the Left boundary key = %v, want the list", got)
	}

	// Right from the list with a room open walks INTO the room (the list
	// never scrolls horizontally on the way).
	walkKey(cd, tcell.KeyRight, tcell.ModNone)
	if got := walkRegion(app, cd); got != "editor" {
		t.Errorf("Right from the list focus = %v, want the room composer", got)
	}
}

// TestWalkShowRoomFocusesComposer pins Python _show_room's focus move
// (Channels.py:1841-1851): showing a room puts the cursor in its composer,
// and the channels list drops to its off-focus highlight.
func TestWalkShowRoomFocusesComposer(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("focus after ShowRoom = %v, want the room composer", got)
	}
	if cd.roomWidget.roomPart != "footer" {
		t.Errorf("room part after ShowRoom = %q, want footer", cd.roomWidget.roomPart)
	}
	// The ILB (the focused pane's list) is unfocused now — the walk's darker
	// off-focus highlight state.
	if cd.ilb.HasFocus() {
		t.Error("the channels list still holds focus after ShowRoom; Python's _show_room moves focus into the room column")
	}
}

// TestWalkShowRoomPreservesDraftAndPart covers a room switch: opening a
// DIFFERENT room replaces the widget and starts on the new room's composer
// (a fresh RoomWidget, Python _show_room builds a new RoomWidget per room),
// while re-showing the SAME room keeps its part (the /clear path).
func TestWalkShowRoomPreservesDraftAndPart(t *testing.T) {
	t.Parallel()

	app, cd := walkCD(t)
	rw := cd.roomWidget

	// Move to the body, then re-show the same room: the part persists.
	app.SetFocus(rw.messagesArea)
	cd.ShowRoom(0, "general", nil)
	if got := walkRegion(app, cd); got != "body" {
		t.Fatalf("focus after re-show = %v, want the preserved body part", got)
	}

	// A different room: a fresh widget starts on its composer.
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general", "lounge"}}})
	cd.ShowRoom(0, "lounge", nil)
	if got := walkRegion(app, cd); got != "editor" {
		t.Fatalf("focus after the room switch = %v, want the new room's composer", got)
	}
	if cd.roomWidget == rw {
		t.Error("the room switch reused the widget; Python builds a fresh RoomWidget per room")
	}
}
