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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestMessageListBoxPerMessageEntries pins A4: the conversation message list
// is a per-message selectable ListBox (Python: messagelist =
// IndicativeListBox(message_widgets), one LXMessageWidget Pile per message,
// Conversations.py:2287) — NOT a flat text blob. Each message is its own
// entry; Up/Down move focus message-by-message.
func TestMessageListBoxPerMessageEntries(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cw := NewConversationWidget(app, "aabb1122")

	now := time.Unix(1700000000, 0)
	msgs := []ConversationMessage{
		{Content: "first", Timestamp: now, State: lxmfStateSent, SourceHash: []byte{1}},
		{Content: "second", Timestamp: now.Add(time.Minute), State: lxmfStateSent, SourceHash: []byte{1}},
		{Content: "third", Timestamp: now.Add(2 * time.Minute), State: lxmfStateSent, SourceHash: []byte{1}},
	}
	cw.SetMessages(msgs)

	lb := cw.messageList
	if got := lb.EntryCount(); got != 3 {
		t.Fatalf("entry count = %v, want 3 (one entry per LXMessageWidget)", got)
	}

	// Python update_message_widgets positions the IndicativeListBox at
	// len(message_widgets)-1: the NEWEST message is focused (Conversations.py:2287).
	if got := lb.FocusIndex(); got != 2 {
		t.Errorf("initial focus = %v, want 2 (newest message, position=len-1)", got)
	}
}

// TestMessageListBoxFocusMovement pins the per-message Up/Down focus path
// (urwid ListBox semantics verified against urwid 4.0.3 in /tmp: focus moves
// one item per key; Up at the first item returns the key unhandled; Down at
// the last item returns the key unhandled — ConversationFrame intercepts both
// cases before the list sees them, Conversations.py:1845-1870).
func TestMessageListBoxFocusMovement(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	now := time.Unix(1700000000, 0)
	msgs := make([]ConversationMessage, 4)
	for i := range msgs {
		msgs[i] = ConversationMessage{
			Content: "line " + itoa(i), Timestamp: now.Add(time.Duration(i) * time.Minute),
			State: lxmfStateSent, SourceHash: []byte{1},
		}
	}
	cw.SetMessages(msgs)
	lb := cw.messageList
	lb.SetRect(0, 0, 60, 10)

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	handle := func(ev *tcell.EventKey) {
		h := lb.InputHandler()
		h(ev, func(p tview.Primitive) {})
	}

	// Start at the newest (index 3). Down at the last entry must not move the
	// focus (Python: the Frame's bottom_is_visible branch owns the transition
	// to the editor; the ListBox itself returns "down" without moving).
	handle(down)
	if got := lb.FocusIndex(); got != 3 {
		t.Errorf("focus after Down at last = %v, want 3", got)
	}

	handle(up)
	if got := lb.FocusIndex(); got != 2 {
		t.Errorf("focus after Up = %v, want 2 (Up inside the list moves the focus)", got)
	}

	// Walk to the top; Up at the first entry does not move the focus.
	for i := lb.FocusIndex(); i > 0; i = lb.FocusIndex() {
		handle(up)
	}
	if got := lb.FocusIndex(); got != 0 {
		t.Fatalf("focus after walking up = %v, want 0", got)
	}
	handle(up)
	if got := lb.FocusIndex(); got != 0 {
		t.Errorf("focus after Up at top = %v, want 0 (boundary: stays)", got)
	}
}

// TestMessageListBoxAutoscrollFollowsFocus pins the autoscroll contract
// (urwid ListBox change_focus): after each Up the focused message must be
// fully visible in the viewport, with the scroll offset following the focus
// (Python messagelist IndicativeListBox: per-message scroll steps).
func TestMessageListBoxAutoscrollFollowsFocus(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	now := time.Unix(1700000000, 0)
	msgs := make([]ConversationMessage, 12)
	for i := range msgs {
		msgs[i] = ConversationMessage{
			Content: "message body line " + itoa(i), Timestamp: now.Add(time.Duration(i) * time.Minute),
			State: lxmfStateSent, SourceHash: []byte{1},
		}
	}
	cw.SetMessages(msgs)
	lb := cw.messageList
	lb.SetRect(0, 0, 60, 6)

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	h := lb.InputHandler()

	// After SetMessages the view is at the bottom (newest focused).
	if got := lb.FocusIndex(); got != 11 {
		t.Fatalf("initial focus = %v, want 11", got)
	}
	for step := 10; step >= 0; step-- {
		h(up, func(p tview.Primitive) {})
		if got := lb.FocusIndex(); got != step {
			t.Fatalf("focus after Up = %v, want %v", got, step)
		}
		top, bottom, ok := lb.FocusedEntryRows()
		if !ok {
			t.Fatalf("no focused entry at index %v", step)
		}
		off := lb.Offset()
		if top < off || bottom > off+6 {
			t.Fatalf("focused entry [%v,%v) not visible in viewport [off=%v,+6) after Up (autoscroll broken)", top, bottom, off)
		}
	}
	if got := lb.Offset(); got != 0 {
		t.Errorf("offset after walking to the top = %v, want 0", got)
	}
}

// TestMessageListBoxTopBottomVisible pins the IndicativeListBox visibility
// flags the ConversationFrame's keypress branches on
// (Conversations.py:1855-1866): top_is_visible ⇔ nothing is scrolled off the
// top; bottom_is_visible ⇔ the list end is reached (nothing below the focused
// end, or the focus is on the last message — urwid get_next(pos)==None).
func TestMessageListBoxTopBottomVisible(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	now := time.Unix(1700000000, 0)
	msgs := make([]ConversationMessage, 8)
	for i := range msgs {
		msgs[i] = ConversationMessage{
			Content: "body " + itoa(i), Timestamp: now.Add(time.Duration(i) * time.Minute),
			State: lxmfStateSent, SourceHash: []byte{1},
		}
	}
	cw.SetMessages(msgs)
	lb := cw.messageList
	lb.SetRect(0, 0, 60, 6)

	if !lb.BottomIsVisible() {
		t.Error("fresh list focused at the newest message: bottom must be visible")
	}
	if lb.TopIsVisible() {
		t.Error("fresh 8-message list in a 6-row viewport: top must NOT be visible")
	}

	// Walk focus to the first message: the autoscroll brings the top into view.
	lb.SetFocusIndex(0)
	if !lb.TopIsVisible() {
		t.Error("focus on first message: top must be visible (autoscroll aligned it)")
	}
	if lb.BottomIsVisible() && lb.FocusIndex() != lb.EntryCount()-1 {
		t.Error("scrolled to top with 8 messages in 6 rows: bottom must NOT be visible")
	}

	// Focus on the LAST message ⇒ bottom_is_visible even when that message is
	// taller than the viewport (urwid: get_next(pos)==(None,None)).
	tall := []ConversationMessage{{
		Content: strings.Repeat("tall\n", 12), Timestamp: now,
		State: lxmfStateSent, SourceHash: []byte{1},
	}}
	cw.SetMessages(tall)
	lb2 := cw.messageList
	lb2.SetRect(0, 0, 60, 4)
	if !lb2.BottomIsVisible() {
		t.Error("single tall message focused: bottom_is_visible must be true (nothing after it)")
	}
}

// TestConversationFocusPathEditorToMessages pins A1's focus path (Python
// MessageEdit.keypress "up" at cursor y==0 → frame.focus_position = "body",
// Conversations.py:1816-1825) and the body→footer Down (Python
// ConversationFrame.keypress "down" at bottom_is_visible → footer,
// Conversations.py:1866-1867): Up from the minimal editor focuses the message
// list; Down at the message-list bottom focuses the editor again.
func TestConversationFocusPathEditorToMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	cw := NewConversationWidget(app, "aabb1122")
	// A trusted peer hides the trust banner (Python has_visible_trust_banner).
	cw.TrustLevel = "trusted"
	cw.refreshTrustBanner()
	now := time.Unix(1700000000, 0)
	cw.SetMessages([]ConversationMessage{
		{Content: "hello", Timestamp: now, State: lxmfStateSent, SourceHash: []byte{1}},
	})

	// Focus the editor directly; the frame capture is invoked via its
	// InputHandler, mirroring the runtime dispatch order (app capture →
	// content Flex capture → frame capture → focused primitive).
	app.SetFocus(cw.editor)
	cw.editor.SetRect(0, 0, 60, 1)
	cw.messageList.SetRect(0, 0, 60, 10)

	editor := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	frameCapture := cw.frame.GetInputCapture()
	ret := frameCapture(editor)
	if ret != nil {
		t.Fatalf("Up in the minimal editor was not consumed by the conversation frame (ret=%v)", ret)
	}
	if got := app.GetFocus(); got != cw.messageList {
		t.Errorf("focus after Up from editor = %T, want the message list (Python: frame.focus_position='body')", got)
	}

	// Focus the FIRST message (Python's top_is_visible) — Up must collapse to
	// the menu bar (no trust banner for a known-trusted peer is irrelevant
	// here; the banner path is covered by TestConversationUpAtTopBanner).
	lb := cw.messageList
	lb.SetFocusIndex(0)
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	if ret := frameCapture(up); ret != nil {
		t.Fatalf("Up at message-list top was not consumed (ret=%v)", ret)
	}
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after Up at message-list top = %T, want the menu bar", got)
	}

	// Down at the message-list bottom returns focus to the editor (Python:
	// ConversationFrame "down" + bottom_is_visible → footer).
	app.SetFocus(cw.messageList)
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	ret = frameCapture(down)
	if ret != nil {
		t.Errorf("Down at message-list bottom was not consumed (ret=%v)", ret)
	}
	if got := app.GetFocus(); got != cw.editor {
		t.Errorf("focus after Down at message-list bottom = %T, want the editor (Python: focus_position='footer')", got)
	}
}

// TestConversationUpAtTopBanner pins A3/A7: with the trust banner visible,
// Up at the message-list top focuses the banner's FIRST button (Trust) —
// Python ConversationFrame.keypress sets _header_pile.focus_position = 1 and
// focus_position = "header" (Conversations.py:1854-1862). Up again from the
// banner reaches the menu bar; Down from the banner returns to the body.
func TestConversationUpAtTopBanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	cw := NewConversationWidget(app, "aabb1122")
	cw.TrustLevel = "unknown" // banner visible
	cw.refreshTrustBanner()
	now := time.Unix(1700000000, 0)
	cw.SetMessages([]ConversationMessage{
		{Content: "hi", Timestamp: now, State: lxmfStateSent, SourceHash: []byte{1}},
	})

	app.SetFocus(cw.messageList)
	lb := cw.messageList
	lb.SetFocusIndex(0)
	cw.messageList.SetRect(0, 0, 60, 10)

	frameCapture := cw.frame.GetInputCapture()
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	if ret := frameCapture(up); ret != nil {
		t.Fatalf("Up at message-list top with banner was not consumed (ret=%v)", ret)
	}

	banner := cw.BannerButtons()
	if len(banner) != 3 {
		t.Fatalf("banner buttons = %v, want 3 (Trust/Block/Do nothing)", len(banner))
	}
	if got := app.GetFocus(); got != banner[0] {
		t.Errorf("focus after Up at top with banner = %T, want the Trust button (header pile focus 1)", got)
	}

	// Up again from the banner → menu bar (Python: Frame header "up" →
	// main_display.frame.focus_position = "header").
	if ret := frameCapture(up); ret != nil {
		t.Fatalf("Up from banner was not consumed (ret=%v)", ret)
	}
	if got := app.GetFocus(); got != app.Main.menuBar {
		t.Errorf("focus after Up from banner = %T, want the menu bar", got)
	}

	// Down from the banner → back to the message list (Python: Frame header
	// "down" → focus_position = "body").
	app.SetFocus(banner[1])
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if ret := frameCapture(down); ret != nil {
		t.Fatalf("Down from banner was not consumed (ret=%v)", ret)
	}
	if got := app.GetFocus(); got != cw.messageList {
		t.Errorf("focus after Down from banner = %T, want the message list", got)
	}
}
