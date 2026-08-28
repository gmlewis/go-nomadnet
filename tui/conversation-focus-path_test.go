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
	"github.com/rivo/tview"
)

// newFocusApp mounts a ConversationsDisplay in a fully wired app (Main
// display + dialogs root + simulation screen) so key events can be dispatched
// through the exact production chain: app-level capture (MainDisplay.
// handleInput) → root → bodyPages → content Flex capture → conversation frame
// capture → focused primitive.
func newFocusApp(t *testing.T, convs []ConversationInfo) (*App, *ConversationsDisplay, func()) {
	t.Helper()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	cd := NewConversationsDisplay(app, convs)
	cd.OnLoadMessages = func(string) []ConversationMessage {
		return []ConversationMessage{{
			Content:   "hello",
			Timestamp: time.Unix(1700000000, 0),
			State:     lxmfStateSent, SourceHash: []byte{1},
		}}
	}
	app.Main.SetDisplay("conversations", cd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("conversations")
	app.Main.Root().Draw(screen)
	redraw := func() { app.Main.Root().Draw(screen) }
	return app, cd, redraw
}

// TestA2RightFromListFocusesConversationColumn pins A2's Right path (Python
// urwid Columns.keypress: an unconsumed "right" moves focus to the next
// selectable column, Conversations.py:221-229): with a conversation open,
// Right from the conversation list focuses the conversation column at its
// current focus part (the composer — opening a conversation focuses the
// footer). Without an open conversation the empty placeholder column is not
// selectable, so Right is a no-op — exactly like Python.
func TestA2RightFromListFocusesConversationColumn(t *testing.T) {
	t.Parallel()

	const hash = "a2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	convs := []ConversationInfo{{SourceHash: hash, DisplayName: "A2 Peer", TrustLevel: "trusted"}}
	app, cd, redraw := newFocusApp(t, convs)
	right := tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)

	// No conversation open yet: the empty column is not selectable → no-op.
	app.SetFocus(cd.ilb)
	redraw()
	dispatchKey(app, app.GetRoot(), right)
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Fatalf("focus after Right with empty detail = %T, want the list (Python: no selectable column)", got)
	}

	// Open the conversation (Python display_conversation), return to the list.
	cd.DisplayConversation(hash)
	app.SetFocus(cd.ilb)
	redraw()
	dispatchKey(app, app.GetRoot(), right)
	if got := app.GetFocus(); got != cd.currentWidget.editor {
		t.Errorf("focus after Right from list = %T, want the composer (frame footer focus part)", got)
	}
}

// TestA2LeftFromMessageListFocusesList pins A2's Left path: Left from the
// conversation pane's message list bubbles to the Columns and focuses the
// conversations list (Python urwid Columns.keypress "left" candidates to the
// left; the ListBox does not consume Left/Right).
func TestA2LeftFromMessageListFocusesList(t *testing.T) {
	t.Parallel()

	const hash = "a2bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	convs := []ConversationInfo{{SourceHash: hash, DisplayName: "A2 Peer", TrustLevel: "trusted"}}
	app, cd, redraw := newFocusApp(t, convs)
	cd.DisplayConversation(hash)
	app.SetFocus(cd.currentWidget.messageList)
	redraw()

	left := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
	dispatchKey(app, app.GetRoot(), left)
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Errorf("focus after Left from message list = %T, want the conversations list", got)
	}
	// The shortcut bar must follow (Python shortcuts() focus_path dispatch).
	if got := cd.shortcutFocus; got != "list" {
		t.Errorf("shortcut region after Left = %q, want \"list\"", got)
	}
}

// TestA2LeftFromBannerButtons pins the banner-row traversal: Left/Right move
// between Trust/Block/Do nothing (Python banner Columns traversal), and Left
// from the FIRST button bubbles past the banner Columns to the outer Columns,
// focusing the conversations list.
func TestA2LeftFromBannerButtons(t *testing.T) {
	t.Parallel()

	const hash = "a2cccccccccccccccccccccccccccccccccccc"
	convs := []ConversationInfo{{SourceHash: hash, DisplayName: "A2 Peer", TrustLevel: "unknown"}}
	app, cd, redraw := newFocusApp(t, convs)
	cd.DisplayConversation(hash)
	cw := cd.currentWidget
	buttons := cw.BannerButtons()
	if len(buttons) != 3 {
		t.Fatalf("banner buttons = %v, want 3", len(buttons))
	}

	// Middle → Left → first button.
	app.SetFocus(buttons[1])
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if got := app.GetFocus(); got != buttons[0] {
		t.Fatalf("focus after Left from Block = %T, want Trust", got)
	}

	// First button → Left → the conversations list column.
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Errorf("focus after Left from leftmost banner button = %T, want the conversations list", got)
	}

	// Right walks forward: Trust → Block → Do nothing; rightmost Right dies.
	app.SetFocus(buttons[0])
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := app.GetFocus(); got != buttons[1] {
		t.Fatalf("focus after Right from Trust = %T, want Block", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := app.GetFocus(); got != buttons[2] {
		t.Fatalf("focus after Right from Block = %T, want Do nothing", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
}

// TestA6UpAtListTopFocusesTabBar pins A6: Up at the top of the conversation
// list focuses the TAB BAR (Python ConversationsArea.keypress "up" → Pile
// previous-selectable; the TabButtons are keyboard-focusable — live-verified
// on Python: Right+Enter switched to the Untrusted tab) — NOT the menubar.
// Only another Up from the tab bar reaches the menubar. Enter on a tab
// switches the filter.
func TestA6UpAtListTopFocusesTabBar(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "a6000000000000000000000000000000000000", DisplayName: "One", TrustLevel: "trusted"},
		{SourceHash: "a6100000000000000000000000000000000000", DisplayName: "Two", TrustLevel: "trusted"},
	}
	app, cd, redraw := newFocusApp(t, convs)
	app.SetFocus(cd.ilb)
	redraw()
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	dispatchKey(app, app.GetRoot(), up)
	if got := app.GetFocus(); got != tview.Primitive(cd.tabTrusted) {
		t.Fatalf("focus after Up at list top = %T, want the Trusted tab button", got)
	}

	// Another Up from the tab bar → menubar (Python: Pile "up" result →
	// main_display.frame.focus_position = "header").
	dispatchKey(app, app.GetRoot(), up)
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after Up from tab bar = %T, want the menu bar", got)
	}

	// Down from the tab bar returns to the list (Python: Pile next-selectable).
	app.SetFocus(cd.tabTrusted)
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Errorf("focus after Down from tab bar = %T, want the list", got)
	}

	// Left/Right move between the two tabs; Right from the rightmost tab with
	// no conversation open dies (no selectable column to the right).
	app.SetFocus(cd.tabTrusted)
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.tabUntrusted) {
		t.Fatalf("focus after Right from Trusted = %T, want Untrusted tab", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.tabTrusted) {
		t.Errorf("focus after Left from Untrusted = %T, want Trusted tab", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))

	// Tab activation (Enter) switches the filter, matching Python's
	// TabButton on_press → _set_filter.
	app.SetFocus(cd.tabUntrusted)
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if cd.showTrusted {
		t.Error("Enter on the Untrusted tab did not switch the filter")
	}
}

// TestA6UntrustedCheckboxInFocusPath pins the checkbox part of A6: on the
// untrusted tab the "Show blocked" checkbox sits between the tab bar and the
// list in the Pile (Python _apply_pile_layout, Conversations.py:316-318), so
// Up from the list reaches the CHECKBOX first, then the tab bar.
func TestA6UntrustedCheckboxInFocusPath(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "a6200000000000000000000000000000000000", DisplayName: "Unk", TrustLevel: "unknown"},
	}
	app, cd, redraw := newFocusApp(t, convs)
	cd.SetShowTrusted(false)
	redraw()
	if !cd.pileHasItem(cd.showBlockedCheckbox) {
		t.Fatal("untrusted tab layout must include the Show blocked checkbox")
	}

	app.SetFocus(cd.ilb)
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.showBlockedCheckbox) {
		t.Fatalf("focus after Up at list top (untrusted) = %T, want the Show blocked checkbox", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.tabTrusted) {
		t.Errorf("focus after Up from checkbox = %T, want the tab bar", got)
	}
	// Down walks back: tab → checkbox → list.
	app.SetFocus(cd.tabTrusted)
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.showBlockedCheckbox) {
		t.Fatalf("focus after Down from tab = %T, want the checkbox", got)
	}
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Errorf("focus after Down from checkbox = %T, want the list", got)
	}
}

// TestA5DownAtLastEntryIsNoOp pins A5 with the LIVE-VERIFIED Python behavior
// (probe 2026-08-27 on the local Python instance: with the list selection on
// the last entry, an extra Down leaves the highlight and the list shortcut
// bar unchanged — the key bubbles up urwid's Pile/Columns chain and is
// dropped by the MainFrame; urwid 4.0.3 Columns.keypress only traverses
// columns for LEFT/RIGHT). Go must therefore also keep focus on the list.
func TestA5DownAtLastEntryIsNoOp(t *testing.T) {
	t.Parallel()

	convs := []ConversationInfo{
		{SourceHash: "a5000000000000000000000000000000000000", DisplayName: "One", TrustLevel: "trusted"},
		{SourceHash: "a5100000000000000000000000000000000000", DisplayName: "Two", TrustLevel: "trusted"},
		{SourceHash: "a5200000000000000000000000000000000000", DisplayName: "Three", TrustLevel: "trusted"},
	}
	app, cd, redraw := newFocusApp(t, convs)
	app.SetFocus(cd.ilb)
	redraw()
	last := len(convs) - 1
	cd.list.SetCurrentItem(last)

	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if got := cd.list.GetCurrentItem(); got != last {
		t.Errorf("selection after Down at last = %v, want %v (unchanged)", got, last)
	}
	if got := app.GetFocus(); got != tview.Primitive(cd.ilb) {
		t.Errorf("focus after Down at last = %T, want the list (unchanged)", got)
	}
}

// TestA6EmptyListUpGoesToMenubar pins the empty-list branch of
// ConversationsArea.keypress (Python ilb.body_is_empty → main display header,
// Conversations.py:1800-1802): with no conversations at all, Up from the list
// skips the tab bar and focuses the menubar.
func TestA6EmptyListUpGoesToMenubar(t *testing.T) {
	t.Parallel()

	app, cd, redraw := newFocusApp(t, nil)
	app.SetFocus(cd.ilb)
	redraw()
	dispatchKey(app, app.GetRoot(), tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after Up on empty list = %T, want the menu bar", got)
	}
}

// TestA3UpAtMessageListTopGoesToMenubar pins A3 through the full dispatch
// chain: with a TRUSTED peer open (no banner), Up at the message-list top
// collapses focus to the menubar (Python ConversationFrame.keypress
// "up" + top_is_visible → main_display.frame.focus_position = "header",
// Conversations.py:1845-1852). Before the fix the flat TextView was unknown
// to bodyListAtTop and Up was a dead key.
func TestA3UpAtMessageListTopGoesToMenubar(t *testing.T) {
	t.Parallel()

	const hash = "a3000000000000000000000000000000000000"
	convs := []ConversationInfo{{SourceHash: hash, DisplayName: "A3 Peer", TrustLevel: "trusted"}}
	app, cd, redraw := newFocusApp(t, convs)
	cd.DisplayConversation(hash)
	lb := cd.currentWidget.messageList
	lb.SetFocusIndex(0)
	app.SetFocus(lb)
	redraw()

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	dispatchKey(app, app.GetRoot(), up)
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after Up at message-list top = %T, want the menu bar", got)
	}
}
