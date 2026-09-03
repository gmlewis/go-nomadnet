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
)

// TestIndicativeMessagesBarCentering pins the indicator bars' centering to
// Python's IndicativeListBox (additional_urwid_widgets — urwid centers with
// the ceil formula (maxcol-len+1)/2 on the RUNE length): drawn at the
// fleet's 96-wide chat inner, "▲" starts at col 48 and "───" at col 47.
// Measured on the 2026-09-03 12:32 full-fleet capture (mac rows 3/33): the
// Go nodes drew "───" at col 44 and "▲" at col 47 — the byte length of the
// 3-byte runes instead of the rune count.
func TestIndicativeMessagesBarCentering(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	rw := NewRoomWidget(app, "hub", "test")

	// Both ends visible: ───/─── (short content).
	rw.SetMessages([]ChannelMessage{{Nick: "a", SrcHash: "01", Text: "hello"}})
	m := NewIndicativeMessages(rw.messages)
	rows := drawMessages(t, m, 96, 9)
	if i := strings.Index(rows[0], "───"); i != 47 {
		t.Errorf("top bar starts at col %v, want 47 ((96-3+1)/2 on the rune count)", i)
	}
	if i := strings.Index(rows[8], "───"); i != 47 {
		t.Errorf("bottom bar starts at col %v, want 47", i)
	}

	// Content hidden above: ▲ at the top (len-1 bar centers at col 48); the
	// sticky tail keeps ─── at the bottom.
	var msgs []ChannelMessage
	for i := range 60 {
		msgs = append(msgs, ChannelMessage{Nick: "a", SrcHash: "01", Text: "line", TsMs: int64(i)})
	}
	rw.SetMessages(msgs)
	rows = drawMessages(t, m, 96, 12)
	if i := strings.Index(rows[0], "▲"); i != 48 {
		t.Errorf("top bar starts at col %v, want 48 ((96-1+1)/2 on the rune count)", i)
	}
	if i := strings.Index(rows[11], "───"); i != 47 {
		t.Errorf("bottom bar starts at col %v, want 47", i)
	}

	// Scrolled up: content hidden above AND below → ▲/▼ (both len-1).
	rw.messages.ScrollTo(2, 0)
	rows = drawMessages(t, m, 96, 12)
	if i := strings.Index(rows[0], "▲"); i != 48 {
		t.Errorf("mid-scroll top bar at col %v, want 48", i)
	}
	if i := strings.Index(rows[11], "▼"); i != 48 {
		t.Errorf("mid-scroll bottom bar at col %v, want 48", i)
	}
}
