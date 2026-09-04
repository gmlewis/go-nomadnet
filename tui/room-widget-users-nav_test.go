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
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// redrawRoomWidget draws the room widget onto the screen after key/mouse
// dispatches so the List's scroll-following Draw and highlight settle.
func redrawRoomWidget(t *testing.T, rw *RoomWidget, screen tcell.Screen) {
	t.Helper()
	widget := rw.Widget().(*tview.Flex)
	widget.Draw(screen)
}

// usersSel dispatches keyboard events to the users list (the pane holds
// focus; the room's captures pass these keys through) and redraws.
func usersSel(t *testing.T, rw *RoomWidget, screen tcell.Screen, keys ...tcell.Key) {
	t.Helper()
	handler := rw.usersList.InputHandler()
	setFocus := func(p tview.Primitive) {}
	for _, key := range keys {
		handler(tcell.NewEventKey(key, 0, tcell.ModNone), setFocus)
	}
	redrawRoomWidget(t, rw, screen)
}

// usersWheel dispatches one wheel notch to the users list's mouse handler
// and redraws. pos is the event position inside the list's rect.
func usersWheel(t *testing.T, rw *RoomWidget, screen tcell.Screen, down bool) {
	t.Helper()
	x, y, _, _ := rw.usersList.GetInnerRect()
	action := tview.MouseScrollUp
	buttons := tcell.WheelUp
	if down {
		action = tview.MouseScrollDown
		buttons = tcell.WheelDown
	}
	ev := tcell.NewEventMouse(x+3, y+1, buttons, tcell.ModNone)
	rw.usersList.MouseHandler()(action, ev, func(p tview.Primitive) {})
	redrawRoomWidget(t, rw, screen)
}

// usersListHeight returns the users list's visible row count.
func usersListHeight(rw *RoomWidget) int {
	_, _, _, h := rw.usersList.GetInnerRect()
	return h
}

// highlightRow returns the users-pane row index (screen row minus the list
// top) whose cells carry the list_focus background, or -1.
func highlightRow(screen tcell.Screen) int {
	_, wantBg := ListFocusColors(ThemeDark)
	for y := 1; y < 36; y++ {
		_, _, bg := usersCell(screen, y, 1)
		if bg == wantBg {
			return y
		}
	}
	return -1
}

// membersN builds n members with distinct sortable nicks.
func membersN(n int) []ChannelMember {
	members := make([]ChannelMember, 0, n)
	for i := range n {
		members = append(members, ChannelMember{
			Nick:   fmt.Sprintf("user%02d", i),
			Hash:   fmt.Sprintf("%02x", i) + strings.Repeat("aa", 15),
			Online: true,
		})
	}
	return members
}

// TestRoomWidgetUsersNavSelection pins the users pane's keyboard selection
// movement (tview.List navigation on the focused pane): Down/Up move one row
// (wrapping at the ends, the fork's wrapAround default), PageUp/PageDown
// jump by the pane's page height, and Home/End jump to the ends.
func TestRoomWidgetUsersNavSelection(t *testing.T) {
	t.Parallel()

	members := membersN(40)
	screen, rw := renderUsersPane(t, members)

	// The pane's raw page height drives the PageUp/PageDown expectations; it
	// is smaller than the member count so a page jump really moves.
	page := usersListHeight(rw)
	if page <= 0 {
		t.Fatalf("users list height = %v, want > 0", page)
	}

	cases := []struct {
		name string
		keys []tcell.Key
		want int
	}{
		{"down moves one row", []tcell.Key{tcell.KeyDown, tcell.KeyDown}, 2},
		{"up moves one row back", []tcell.Key{tcell.KeyDown, tcell.KeyDown, tcell.KeyUp}, 1},
		{"down at the end wraps to the first row", []tcell.Key{tcell.KeyEnd, tcell.KeyDown}, 0},
		{"up at the first row wraps to the last row", []tcell.Key{tcell.KeyUp}, len(members) - 1},
		{"home jumps to the first row", []tcell.Key{tcell.KeyEnd, tcell.KeyHome}, 0},
		{"end jumps to the last row", []tcell.Key{tcell.KeyEnd}, len(members) - 1},
		{"page down jumps a page", []tcell.Key{tcell.KeyPgDn}, page},
		{"page up jumps back a page", []tcell.Key{tcell.KeyEnd, tcell.KeyPgUp}, len(members) - 1 - page},
	}
	for _, tc := range cases {
		rw.usersList.SetCurrentItem(0)
		redrawRoomWidget(t, rw, screen)
		usersSel(t, rw, screen, tc.keys...)
		if got := rw.usersList.GetCurrentItem(); got != tc.want {
			t.Errorf("%v: selection = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRoomWidgetUsersNavHighlightFollows pins scroll-following: the
// highlighted row IS the selection and stays visible while the pane scrolls
// — with more members than rows, End scrolls the last member into view
// highlighted (and the first off-screen), Home scrolls back to the first.
func TestRoomWidgetUsersNavHighlightFollows(t *testing.T) {
	t.Parallel()

	const n = 60
	screen, rw := renderUsersPane(t, membersN(n))

	rw.usersList.SetCurrentItem(0)
	usersSel(t, rw, screen, tcell.KeyEnd)
	if got := rw.usersList.GetCurrentItem(); got != n-1 {
		t.Fatalf("End selection = %v, want %v", got, n-1)
	}

	// The highlighted (selected) row is visible on screen, and it is the
	// LAST member's row: its text carries the list_focus background.
	hiY := highlightRow(screen)
	if hiY < 0 {
		t.Fatal("no highlighted row on screen after End")
	}
	if row := usersRowText(screen, hiY); !strings.Contains(row, fmt.Sprintf("user%02d", n-1)) {
		t.Errorf("highlighted row %v = %q, want the last member", hiY, row)
	}
	// The selection scrolled out the first member: it must be gone.
	for y := 1; y < 36; y++ {
		if strings.Contains(usersRowText(screen, y), "user00") {
			t.Errorf("first member still visible after End (no scroll-following) at row %v", y)
		}
	}

	// Home scrolls back: the first member is highlighted and visible again.
	usersSel(t, rw, screen, tcell.KeyHome)
	hiY = highlightRow(screen)
	if hiY < 0 {
		t.Fatal("no highlighted row on screen after Home")
	}
	if row := usersRowText(screen, hiY); !strings.Contains(row, "user00") {
		t.Errorf("highlighted row %v = %q, want the first member", hiY, row)
	}
}

// TestRoomWidgetUsersWheelScrolls pins the users pane's wheel behavior: a
// notch moves the SELECTION by the wheel multiplier (so the highlighted row
// follows the scroll) and a notch at the boundary is a no-op that cannot
// drift the viewport off the selection.
//
// Mutates the package-global mouseWheelLines, so it runs sequentially.
func TestRoomWidgetUsersWheelScrolls(t *testing.T) {
	orig := mouseWheelLines
	t.Cleanup(func() { mouseWheelLines = orig })
	SetMouseWheelLines(3)

	const n = 12
	screen, rw := renderUsersPane(t, membersN(n))

	// Down from the top: the selection lands 3 rows down and the highlight
	// sits on it.
	usersWheel(t, rw, screen, true)
	if got := rw.usersList.GetCurrentItem(); got != 3 {
		t.Fatalf("wheel down selection = %v, want 3", got)
	}
	hiY := highlightRow(screen)
	if hiY < 0 || !strings.Contains(usersRowText(screen, hiY), "user03") {
		t.Errorf("wheel-down highlight row %v = %q, want user03", hiY, usersRowText(screen, hiY))
	}

	// A second notch lands on 6; up moves back to 3.
	usersWheel(t, rw, screen, true)
	if got := rw.usersList.GetCurrentItem(); got != 6 {
		t.Errorf("second wheel down selection = %v, want 6", got)
	}
	usersWheel(t, rw, screen, false)
	if got := rw.usersList.GetCurrentItem(); got != 3 {
		t.Errorf("wheel up selection = %v, want 3", got)
	}

	// Overshoot clamps at the end and further notches are no-ops that keep
	// the viewport pinned (the last member stays highlighted on screen).
	for range 10 {
		usersWheel(t, rw, screen, true)
	}
	if got := rw.usersList.GetCurrentItem(); got != n-1 {
		t.Errorf("wheel-down clamp selection = %v, want %v", got, n-1)
	}
	hiY = highlightRow(screen)
	if hiY < 0 || !strings.Contains(usersRowText(screen, hiY), fmt.Sprintf("user%02d", n-1)) {
		t.Errorf("boundary wheel highlight row %v = %q, want the last member", hiY, usersRowText(screen, hiY))
	}
	usersWheel(t, rw, screen, true)
	if got := rw.usersList.GetCurrentItem(); got != n-1 {
		t.Errorf("wheel at the bottom moved the selection to %v, want %v", got, n-1)
	}

	// Up at the top clamps to the first row and stays there.
	for range 10 {
		usersWheel(t, rw, screen, false)
	}
	if got := rw.usersList.GetCurrentItem(); got != 0 {
		t.Errorf("wheel-up clamp selection = %v, want 0", got)
	}
}

// TestRoomWidgetUsersClickSelectsAndDialog pins the left-click path: a click
// on a member row moves the selection (and the highlight) onto it and fires
// the same OnMemberClick user-info dialog path as keyboard activation
// (Python show_user_info, Channels.py:2119).
func TestRoomWidgetUsersClickSelectsAndDialog(t *testing.T) {
	t.Parallel()

	members := membersN(5)
	screen, rw := renderUsersPane(t, members)

	var gotNick, gotHash string
	rw.OnMemberClick = func(nick, hash string) { gotNick, gotHash = nick, hash }

	// The member rows start at the pane row right under the count row; the
	// click targets member 2's row inside the list's inner rect.
	lx, ly, _, _ := rw.usersList.GetInnerRect()
	ev := tcell.NewEventMouse(lx+3, ly+2, tcell.ButtonPrimary, tcell.ModNone)
	consumed, _ := rw.usersList.MouseHandler()(tview.MouseLeftClick, ev, func(p tview.Primitive) {})
	if !consumed {
		t.Fatal("left click on a member row was not consumed")
	}
	redrawRoomWidget(t, rw, screen)

	if got := rw.usersList.GetCurrentItem(); got != 2 {
		t.Errorf("click selection = %v, want 2", got)
	}
	hiY := highlightRow(screen)
	if hiY < 0 || !strings.Contains(usersRowText(screen, hiY), "user02") {
		t.Errorf("click highlight row %v = %q, want user02", hiY, usersRowText(screen, hiY))
	}
	if rw.OnMemberClick == nil {
		t.Fatal("OnMemberClick unset")
	}
	if gotNick != "user02" || gotHash != members[2].Hash {
		t.Errorf("OnMemberClick = (%q, %q), want (user02, %q)", gotNick, gotHash, members[2].Hash)
	}
}

// TestRoomWidgetUsersEnterOpensDialog pins the Enter activation path: with
// the pane focused and a member selected, Enter fires the SAME OnMemberClick
// user-info dialog path the click signal uses (no duplicate dialog code).
func TestRoomWidgetUsersEnterOpensDialog(t *testing.T) {
	t.Parallel()

	members := membersN(5)
	screen, rw := renderUsersPane(t, members)

	var gotNick, gotHash string
	fired := false
	rw.OnMemberClick = func(nick, hash string) { fired = true; gotNick, gotHash = nick, hash }

	rw.usersList.SetCurrentItem(3)
	usersSel(t, rw, screen, tcell.KeyEnter)

	if !fired {
		t.Fatal("Enter on the selected member did not fire OnMemberClick")
	}
	if gotNick != "user03" || gotHash != members[3].Hash {
		t.Errorf("OnMemberClick = (%q, %q), want (user03, %q)", gotNick, gotHash, members[3].Hash)
	}
}

// TestRoomWidgetUsersSelectionSurvivesRebuild pins Python
// _refresh_users_pane's prev_focus_key restore (Channels.py:708-724): a
// membership change re-selects the SAME member by identity hash, and a
// departed member falls back to the first row.
func TestRoomWidgetUsersSelectionSurvivesRebuild(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	rw := NewRoomWidget(app, "hub1", "general")

	rw.SetMembers([]ChannelMember{
		{Nick: "alice", Hash: "aa11"},
		{Nick: "bob", Hash: "bb22"},
		{Nick: "carol", Hash: "cc33"},
	})
	rw.usersList.SetCurrentItem(1)

	// The same members with new arrivals: bob keeps the selection even
	// though his row index moved.
	rw.SetMembers([]ChannelMember{
		{Nick: "dave", Hash: "dd44"},
		{Nick: "bob", Hash: "bb22"},
		{Nick: "erin", Hash: "ee55"},
		{Nick: "carol", Hash: "cc33"},
	})
	if got := rw.usersList.GetCurrentItem(); got != 1 || rw.members[got].Hash != "bb22" {
		t.Errorf("selection after rebuild = %v (%q), want bob's row", got, rw.members[max(got, 0)].Nick)
	}

	// bob leaves: the selection falls back to the first row.
	rw.SetMembers([]ChannelMember{
		{Nick: "dave", Hash: "dd44"},
		{Nick: "erin", Hash: "ee55"},
	})
	if got := rw.usersList.GetCurrentItem(); got != 0 {
		t.Errorf("selection after the member left = %v, want 0", got)
	}
}

// TestChannelsRoomFocusUsersPane pins the room's keyboard pane walk: Right
// from the message body focuses the users pane (where the selection keys
// scroll it) and Left from the users pane steps back into the body — while
// Left from elsewhere in the room still leaves for the channels list.
func TestChannelsRoomFocusUsersPane(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)

	// Right from the body: focus lands on the users pane.
	app.SetFocus(cd.roomWidget.messagesArea)
	cd.handleInput(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if !cd.roomWidget.usersPaneHasFocus() {
		t.Fatalf("Right from the body focus = %T, want the users pane", app.GetFocus())
	}

	// Left from the users pane: back into the message body.
	cd.handleInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if app.GetFocus() != tview.Primitive(cd.roomWidget.messagesArea) {
		t.Fatalf("Left from the users pane focus = %T, want the message body", app.GetFocus())
	}

	// Left from the body still leaves the room for the channels list
	// (Python's columns focus walk, unchanged).
	cd.handleInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if app.GetFocus() != tview.Primitive(cd.ilb) {
		t.Fatalf("Left from the body focus = %T, want the channels list", app.GetFocus())
	}
}
