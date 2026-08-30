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

// TestMenuDownRestoresBodyFocus pins the Down-from-menu behavior measured
// against the live nomadnet 1.2.8 source of truth: urwid's Frame keeps the
// body's INTERNAL focus while the header owns focus (Main.py MenuColumns
// keypress sets frame.focus_position = "body" and re-dispatches the key), so a
// Down from the menu bar returns to the widget the body had focused — the
// open conversation's scrolled message list — and that same keypress scrolls
// it one row. The regression was FocusBody cascading to the content area's
// default focus chain, which landed on the LEFT conversations list: the key
// count to walk back down the conversation changed and the cursor sat in the
// wrong pane.
func TestMenuDownRestoresBodyFocus(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	md := app.Main

	now := time.Unix(1700000000, 0)
	msgs := make([]ConversationMessage, 8)
	for i := range msgs {
		msgs[i] = ConversationMessage{
			Content: "body " + itoa(i), Timestamp: now.Add(time.Duration(i) * time.Minute),
			State: lxmfStateSent, SourceHash: []byte{1},
		}
	}
	cw := NewConversationWidget(app, "aabb1122")
	cw.SetMessages(msgs)
	lb := cw.messageList
	lb.SetRect(0, 0, 60, 6)

	// Body focus sits mid-scroll on the message list.
	app.SetFocus(lb)
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	lb.InputHandler()(up, func(p tview.Primitive) {})
	lb.InputHandler()(up, func(p tview.Primitive) {})
	before := lb.Offset()
	if before == 0 {
		t.Fatalf("precondition: list not scrolled (offset %v)", before)
	}

	// Walk Up to the menu, exactly like the dispatcher's bodyTopPath.
	md.FocusMenu()
	if md.lastBodyFocus != tview.Primitive(lb) {
		t.Fatalf("FocusMenu did not remember the focused body primitive (got %T)", md.lastBodyFocus)
	}

	// Down from the menu restores the SAME body primitive but DROPS the key:
	// Python's MenuColumns.keypress sets frame.focus_position = "body" and
	// urwid's Frame dispatches by the entry-time focus part, so the key never
	// re-dispatches into the body (verified live on nomadnet 1.2.8 — the first
	// Down renders nothing, the second Down scrolls).
	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if ret := md.handleInput(down); ret != nil {
		t.Fatalf("Down from the menu was forwarded (%v); Python drops it (MenuColumns.keypress Main.py:171-176)", ret)
	}
	if got := app.GetFocus(); got != tview.Primitive(lb) {
		t.Fatalf("focus after Down from menu = %T, want the message list (restored body focus)", got)
	}
	if got := lb.Offset(); got != before {
		t.Errorf("offset after the first Down = %v, want %v (the dropped key must not scroll)", got, before)
	}

	// The SECOND Down reaches the restored list and scrolls one row.
	ret := md.handleInput(down)
	if ret == nil {
		t.Fatal("second Down was consumed in the body; want it forwarded to the message list")
	}
	lb.InputHandler()(ret, func(p tview.Primitive) {})
	if got := lb.Offset(); got != before+1 {
		t.Errorf("offset after the second Down = %v, want %v (one row per keypress from the body)", got, before+1)
	}
}

// TestPageSwitchInvalidatesBodyFocusRestore pins the guard against restoring a
// HIDDEN page's widget: switching to a different menu page clears the
// remembered body focus, so a later FocusBody falls back to the content area's
// own focus chain instead of stealing focus to a widget that is no longer
// displayed (bodyPages exists precisely because hidden pages must never take
// input).
func TestPageSwitchInvalidatesBodyFocusRestore(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Main = NewMainDisplay(app, ThemeDark, GlyphUnicode)
	md := app.Main

	// Any non-menu primitive stands in for a focused body widget.
	cw := NewConversationWidget(app, "aabb1122")
	app.SetFocus(cw.messageList)

	md.FocusMenu()
	if md.lastBodyFocus == nil {
		t.Fatal("precondition: FocusMenu must remember the body focus")
	}

	// Switch to another menu page: the remembered primitive belongs to the
	// previous page and must be forgotten.
	other := -1
	for i, item := range md.menuItems {
		if item.Key != md.activePage {
			other = i
			break
		}
	}
	if other < 0 {
		t.Fatal("need at least two menu items to exercise a page switch")
	}
	md.selectMenu(other)
	if md.lastBodyFocus != nil {
		t.Errorf("after switching pages, remembered body focus = %T, want nil", md.lastBodyFocus)
	}

	// FocusBody now falls back to the content area, not the stale widget.
	md.FocusBody()
	if got := app.GetFocus(); got == tview.Primitive(cw.messageList) {
		t.Error("FocusBody restored a widget from the hidden page")
	}
}
