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

// TestMessageListBoxRowScrolling pins the scrolling model verified against
// the live nomadnet 1.2.8 source of truth: the LXMessageWidgets are NOT
// selectable, so each Up/Down shifts the viewport exactly ONE row (urwid
// ListBox _keypress_up/down shift_focus fallback) — NOT one message per
// keypress. The focus follows the viewport edge; the keys at the scroll
// boundaries are declined so the ConversationFrame capture can apply the
// banner/menu (up) and composer (down) transitions.
func TestMessageListBoxRowScrolling(t *testing.T) {
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
	lb.SetRect(0, 0, 60, 6) // viewport height 4 after the two indicator bars

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	handle := func(ev *tcell.EventKey) {
		lb.InputHandler()(ev, func(p tview.Primitive) {})
	}

	startOffset := lb.Offset()
	if got := startOffset; got != lb.maxOffset() {
		t.Fatalf("fresh list offset = %v, want %v (bottom-aligned on the newest message)", got, lb.maxOffset())
	}

	// Down at the exact bottom is declined for the frame's composer transition.
	handle(down)
	if got := lb.Offset(); got != startOffset {
		t.Errorf("offset after Down at bottom = %v, want %v (bottom boundary)", got, startOffset)
	}

	// Each Up scrolls exactly one row.
	handle(up)
	if got := lb.Offset(); got != startOffset-1 {
		t.Errorf("offset after one Up = %v, want %v", got, startOffset-1)
	}
	if got := lb.FocusIndex(); got != lb.entryAtRow(lb.Offset()) {
		t.Errorf("focus after Up = %v, want the top visible entry %v", got, lb.entryAtRow(lb.Offset()))
	}

	// Walk to the top one row at a time; Up at the top is declined.
	for lb.Offset() > 0 {
		handle(up)
	}
	if got := lb.Offset(); got != 0 {
		t.Fatalf("offset after walking up = %v, want 0", got)
	}
	handle(up)
	if got := lb.Offset(); got != 0 {
		t.Errorf("offset after Up at top = %v, want 0 (top boundary)", got)
	}

	// Down walks back one row at a time.
	handle(down)
	if got := lb.Offset(); got != 1 {
		t.Errorf("offset after first Down from top = %v, want 1", got)
	}
}

// TestMessageListBoxRowScrollCounts pins the exact keystroke counts measured
// against the Python 1.2.8 source of truth (the tmux panel A/B comparison):
// reaching the top takes exactly one Up per conversation row above the
// viewport, and returning to the bottom one Down per row — message-boundary
// hops (3-4 rows per keypress) are a regression.
func TestMessageListBoxRowScrollCounts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	now := time.Unix(1700000000, 0)
	msgs := make([]ConversationMessage, 20) // one line each → 4 rendered rows per message
	for i := range msgs {
		msgs[i] = ConversationMessage{
			Content: "msg " + itoa(i), Timestamp: now.Add(time.Duration(i) * time.Minute),
			State: lxmfStateSent, SourceHash: []byte{1},
		}
	}
	cw.SetMessages(msgs)
	lb := cw.messageList
	lb.SetRect(0, 0, 60, 8)

	total, vh := lb.total, lb.viewHeight()
	ups := lb.maxOffset()
	for i := range ups {
		lb.InputHandler()(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone), func(p tview.Primitive) {})
		if got := lb.Offset(); got != ups-1-i {
			t.Fatalf("offset after %v Ups = %v, want %v", i+1, got, ups-1-i)
		}
	}
	if !lb.TopIsVisible() {
		t.Error("after maxOffset Ups the top must be visible")
	}
	downs := 0
	for lb.Offset() < total-vh {
		lb.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(p tview.Primitive) {})
		downs++
		if downs > ups+1 {
			t.Fatal("Down never reached the exact bottom")
		}
	}
	if downs != ups {
		t.Errorf("Downs to return to the bottom = %v, want %v (same as the Ups to the top)", downs, ups)
	}
	if !lb.BottomIsVisible() {
		t.Error("after walking back down, the exact bottom must be visible (composer handoff ready)")
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
