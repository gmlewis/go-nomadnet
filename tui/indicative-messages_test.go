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
)

// drawMessages renders the wrapper into a simulation screen and returns the
// painted rows as strings.
func drawMessages(t *testing.T, m *IndicativeMessages, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	m.SetRect(0, 0, w, h)
	m.Draw(screen)
	rows := make([]string, h)
	for y := range h {
		var sb strings.Builder
		for x := range w {
			c, _, _ := screen.Get(x, y)
			sb.WriteString(c)
		}
		rows[y] = sb.String()
	}
	return rows
}

// TestIndicativeMessagesBars pins the IndicativeListBox indicator semantics
// for the room's message area (Python _StickyMessageListBox): "───" on both
// ends when everything fits, "▲"/"▼" when content is hidden beyond an end,
// and one reserved viewport row per bar.
func TestIndicativeMessagesIndicators(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	rw := NewRoomWidget(app, "hub", "test")

	// Short content: both ends exposed → "───" top and bottom.
	rw.SetMessages([]ChannelMessage{{Nick: "a", SrcHash: "01", Text: "hello"}})
	m := NewIndicativeMessages(rw.messages)
	rows := drawMessages(t, m, 60, 9)
	if !strings.Contains(rows[0], "───") || !strings.Contains(rows[8], "───") {
		t.Errorf("short-content bars: top=%q bottom=%q, want ───/───", rows[0], rows[8])
	}

	// Long content scrolled to the tail: ▲ at top (content above), ─── at
	// the bottom (the sticky tail).
	var msgs []ChannelMessage
	for i := range 60 {
		msgs = append(msgs, ChannelMessage{Nick: "a", SrcHash: "01", Text: "line", TsMs: int64(i)})
	}
	rw.SetMessages(msgs)
	rows = drawMessages(t, m, 60, 12)
	if !strings.Contains(rows[0], "▲") {
		t.Errorf("tail top bar = %q, want ▲ (content hidden above)", rows[0])
	}
	if !strings.Contains(rows[11], "───") {
		t.Errorf("tail bottom bar = %q, want ─── (last row visible)", rows[11])
	}

	// Scrolled up: both ends covered → ▲ and ▼.
	rw.messages.ScrollTo(2, 0)
	rows = drawMessages(t, m, 60, 12)
	if !strings.Contains(rows[0], "▲") || !strings.Contains(rows[11], "▼") {
		t.Errorf("mid-scroll bars: top=%q bottom=%q, want ▲/▼", rows[0], rows[11])
	}
}

// TestRoomWidgetStickyBottom pins the sticky-tail behavior: fresh renders
// open at the tail (Python's initial bottom focus), and a user who scrolled
// up stays put across refreshes (Channels.py:799-805 append_message's
// was_at_bottom latch).
func TestRoomWidgetStickyBottom(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	rw := NewRoomWidget(app, "hub", "test")

	var msgs []ChannelMessage
	for i := range 60 {
		msgs = append(msgs, ChannelMessage{Nick: "a", SrcHash: "01", Text: "line", TsMs: int64(i)})
	}
	rw.SetMessages(msgs)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 14)
	rw.messages.SetRect(0, 0, 60, 14)
	rw.messages.Draw(screen)

	// A fresh render with more content keeps the tail visible (bottom row
	// shows the last message, not the top of the buffer).
	rw.SetMessages(append(msgs, ChannelMessage{Nick: "a", SrcHash: "01", Text: "tail row", TsMs: 61}))
	rw.messages.Draw(screen)
	visible := ""
	for y := range 14 {
		var sb strings.Builder
		for x := range 60 {
			c, _, _ := screen.Get(x, y)
			sb.WriteString(c)
		}
		visible += sb.String() + "\n"
	}
	if !strings.Contains(visible, "tail row") {
		t.Errorf("refresh lost the tail: the last row is not visible\n%v", visible)
	}

	// Scrolling up latches: a further refresh must NOT yank the user back
	// down (Python's sticky_bottom = false once the user leaves the tail).
	rw.messages.ScrollTo(0, 0)
	rw.SetMessages(append(msgs, ChannelMessage{Nick: "a", SrcHash: "01", Text: "another", TsMs: 62}))
	if row, _ := rw.messages.GetScrollOffset(); row != 0 {
		t.Errorf("scrolled-up user yanked to offset %v, want 0 (sticky)", row)
	}
}
