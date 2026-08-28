// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
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
)

// TestConversationWidgetPaletteColors pins the conversation widget's
// editor + peer-info-bar + message-list base colors to the Python palette.
// Python wraps the editor in `AttrMap(editor, "msg_editor")` and the peer-info
// bar in `AttrMap(Text, "msg_header_sent")` (Conversations.py:602,609); the
// messagelist is a bare `IndicativeListBox` with NO AttrMap (Conversations.py:
// 2287) so its base is the terminal default. The palette (ui/TextUI.py):
//
//	msg_header_sent  = #111 / #ddd   (both themes, lines 35 & 88)
//	msg_editor       = #111 / #0bb   (both themes, lines 32 & 85)
//
// Both are 3-hex, so urwid cube-quantizes them even in truecolor: #111→#000000,
// #ddd→#d7d7d7, #0bb→#00afaf. The Go port previously used nibble-doubled / wrong
// literals: the editor 0x222222/0xdddddd (Python never uses #222/#ddd for the
// editor — it is #0bb/#111) and the peer-info bar 0x111111/0xdddddd (exact, not
// the cube-quantized #000000/#d7d7d7 Python emits). The message-list base was
// 0xbbbbbb where Python uses default.
func TestConversationWidgetPaletteColors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		theme int
	}{
		{"dark", ThemeDark},
		{"light", ThemeLight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(tc.theme, GlyphUnicode, ColorModeTrue)
			cw := NewConversationWidget(app, "") // plain "No conversation selected"

			// Editor field colors must be the cube-quantized msg_editor palette:
			// fg #111→#000000, bg #0bb→#00afaf. (tview InputField.SetField*
			// update the same style GetFieldStyle returns.)
			for _, ed := range []*ReadlineEdit{cw.editor, cw.titleEditor} {
				fg, bg, _ := ed.GetFieldStyle().Decompose()
				if uint32(fg.Hex())&0xffffff != 0x000000 {
					t.Errorf("editor field fg = #%06x, want #000000 (msg_editor fg #111 cube-quantized)", uint32(fg.Hex())&0xffffff)
				}
				if uint32(bg.Hex())&0xffffff != 0x00afaf {
					t.Errorf("editor field bg = #%06x, want #00afaf (msg_editor bg #0bb cube-quantized)", uint32(bg.Hex())&0xffffff)
				}
			}

			// Peer-info bar base must be the cube-quantized msg_header_sent
			// palette: fg #111→#000000, bg #ddd→#d7d7d7. Draw the plain-text
			// header (source=="" → "No conversation selected", no color tags)
			// and probe a non-blank glyph cell.
			piScreen := tcell.NewSimulationScreen("UTF-8")
			if err := piScreen.Init(); err != nil {
				t.Fatalf("peerInfo screen.Init: %v", err)
			}
			defer piScreen.Fini()
			piScreen.SetSize(60, 1)
			cw.peerInfoBar.SetRect(0, 0, 60, 1)
			cw.peerInfoBar.Draw(piScreen)
			if c, _, style, _ := cellContent(piScreen, 1, 0); c == ' ' || c == 0 {
				t.Fatalf("peerInfoBar cell (1,0) is blank; cannot probe base color")
			} else {
				fg, bg, _ := style.Decompose()
				if uint32(fg.Hex())&0xffffff != 0x000000 {
					t.Errorf("peerInfoBar fg = #%06x, want #000000 (msg_header_sent fg #111 cube-quantized)", uint32(fg.Hex())&0xffffff)
				}
				if uint32(bg.Hex())&0xffffff != 0xd7d7d7 {
					t.Errorf("peerInfoBar bg = #%06x, want #d7d7d7 (msg_header_sent bg #ddd cube-quantized)", uint32(bg.Hex())&0xffffff)
				}
			}

			// Message-list base must be the terminal default (Python's
			// messagelist is a bare IndicativeListBox with no AttrMap). Probe
			// a rendered message entry's plain content row (below the styled
			// header): the entry TextView base is the list base. OwnHash is
			// set so the message renders a single-line outbound header.
			cw.OwnHash = []byte{1}
			cw.SetMessages([]ConversationMessage{{
				Content:   "X",
				Timestamp: time.Unix(1700000000, 0),
				State:     lxmfStateSent, SourceHash: []byte{1},
			}})
			mlScreen := tcell.NewSimulationScreen("UTF-8")
			if err := mlScreen.Init(); err != nil {
				t.Fatalf("messageList screen.Init: %v", err)
			}
			defer mlScreen.Fini()
			mlScreen.SetSize(80, 3)
			entry := cw.messageList.entries[0]
			entry.SetRect(0, 0, 80, 3)
			entry.Draw(mlScreen)
			// Row 1 is the content row ("  X") — an unstyled glyph.
			if c, _, style, _ := cellContent(mlScreen, 2, 1); c != 'X' {
				t.Fatalf("message entry content cell (2,1) = %q, want 'X'", string(c))
			} else {
				fg, _, _ := style.Decompose()
				if fg != tcell.ColorDefault {
					t.Errorf("message entry base fg = %v, want ColorDefault (Python messagelist has no AttrMap)", fg)
				}
			}
		})
	}
}

// TestConversationWidgetHeaderColors verifies each LXMessageWidget header row
// is rendered with the Python palette's foreground AND background colors, not
// a foreground-only color. Python wraps each message header in
// AttrMap(..., "msg_header_<style>") (Conversations.py:2596-2670 + TextUI.py
// palette lines 33-38), so e.g. a SENT header is dark text (#111 cube→#000000)
// on a light-gray background (#ddd cube→#d7d7d7), NOT colored text on the
// default background. The Go port previously emitted a foreground-only tview
// tag, producing completely different colors from nomadnet.
func TestConversationWidgetHeaderColors(t *testing.T) {
	t.Parallel()

	now := time.Now()
	own := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	peer := []byte{0x11, 0x22, 0x33, 0x44}

	cases := []struct {
		name  string
		style string // expected urwid header style name
		msg   ConversationMessage
	}{
		{
			name:  "sent",
			style: "msg_header_sent",
			msg: ConversationMessage{
				Content: "out", Timestamp: now, State: lxmfStateSent,
				SourceHash: own, TransportEncrypted: true,
			},
		},
		{
			name:  "propagated",
			style: "msg_header_propagated",
			msg: ConversationMessage{
				Content: "out", Timestamp: now, State: lxmfStateSent,
				Method: lxmfMethodPropagated, SourceHash: own, TransportEncrypted: true,
			},
		},
		{
			name:  "delivered",
			style: "msg_header_delivered",
			msg: ConversationMessage{
				Content: "out", Timestamp: now, State: lxmfStateDelivered,
				SourceHash: own, TransportEncrypted: true,
			},
		},
		{
			name:  "failed",
			style: "msg_header_failed",
			msg: ConversationMessage{
				Content: "out", Timestamp: now, State: lxmfStateFailed,
				SourceHash: own, TransportEncrypted: true,
			},
		},
		{
			name:  "inbound_ok",
			style: "msg_header_ok",
			msg: ConversationMessage{
				Content: "in", Timestamp: now, State: lxmfStateSent,
				SourceHash: peer, SignatureValidated: true, TransportEncrypted: true,
			},
		},
	}

	tc := GetThemeColors(ThemeDark)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
			cw := NewConversationWidget(app, "peerhex")
			cw.OwnHash = own
			cw.SetMessages([]ConversationMessage{c.msg})

			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(80, 6)
			cw.messageList.SetRect(0, 0, 80, 6)
			cw.messageList.Draw(screen)

			wantFG := uint32(tc[c.style+"_fg"].Hex()) & 0xffffff
			wantBG := uint32(tc[c.style+"_bg"].Hex()) & 0xffffff

			// The header is the first non-blank row (row 0). Scan the first
			// several cells for a styled header glyph cell.
			found := false
			for x := 0; x < 40 && !found; x++ {
				r, _, style, _ := cellContent(screen, x, 0)
				if r == ' ' || r == 0 {
					continue
				}
				fg, bg, _ := style.Decompose()
				gotFG := uint32(fg.Hex()) & 0xffffff
				gotBG := uint32(bg.Hex()) & 0xffffff
				if gotFG != wantFG {
					t.Errorf("header fg = #%06x, want #%06x (palette %v_fg)", gotFG, wantFG, c.style)
				}
				if gotBG != wantBG {
					t.Errorf("header bg = #%06x, want #%06x (palette %v_bg)", gotBG, wantBG, c.style)
				}
				found = true
			}
			if !found {
				t.Fatalf("no styled header cell found on row 0 for %v", c.name)
			}

			// The header background must fill the ENTIRE row width, matching
			// Python's urwid AttrMap which paints every cell of the row. Probe
			// a trailing cell near the right edge (well past the header text)
			// and assert it carries a bg within 1 unit of the header bg. The
			// Draw-fill nudges the blue component off a 256-color cube level
			// (a tcell flushing workaround), so the fill bg may differ by 1.
			r, _, style, _ := cellContent(screen, 78, 0)
			if r != ' ' && r != 0 {
				t.Errorf("trailing cell (78,0) rune = %q, want space", string(r))
			}
			_, bg, _ := style.Decompose()
			gotBG := uint32(bg.Hex()) & 0xffffff
			if gotBG < wantBG-1 || gotBG > wantBG+1 {
				t.Errorf("trailing header bg = #%06x, want #%06x (±1, header bg must fill full row width, not truncate at text end)", gotBG, wantBG)
			}
		})
	}
}
