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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// notice47Body is the `members in general:` /who notice body reconstructed
// from capture 47 (tooling/parity-reference/captures/
// 47-mac-mini-nomadnet-channels-general.txt, message-pane rows 11-28): each
// rendered row's visible text joined with the single wrap space urwid's
// "space" wrap drops at every break. It contains emoji and wide runes that
// must never be split mid-glyph.
const notice47Body = `members in general: qbit (b253938bf730), Nomad1n0 (d94777f6b45c), rmnd (9ebb7a043a6d), 0a8b370a62de4c5464b7ef7f56ff33c8, PN42 (0c9b54f545a1), 464360ee59ed9938ba59f6677cf4ac4d, Marvin (4f5edd89b1ca), Ivan (9b246eead574), deka0 (f85b0a6349f1), hojan (f26e49bd7302), typhoon (3d2006d44aad), iamGBOX (2d00312e4279), Matt (cf50aac62b32), fengKlump (d4e363fbe237), adept 🐲 (f6f8de525419), weechatter (44b0edc89bd4), JWD (d30839368721), afri-orion (27716218762c), msjl.nl (1875cc3a1aa3), kilo40 (77c36219188a), xl8r🚀 (4319c0f4c458), Mees electronics (e5b543d3f588), wiretapped (383571c943dd), CBOWork (56244cbdc291), generalist (a9e16f04b125), tyrd (455abe2f53d7), nickie (fb511cbd0fc9), Kraven Meshy Chat (7b9469ee74dc), jrrz (a45e6508a37d), Kraven The Hunter (425b7b6f8e6f), guvy (ee5f14932a4a), VK2FAT (1747e565d5d1), 0xmp (433133b7eca4), kMan_PM (03830fc0e2c1), vclv (9b0c460de2bb), zer0bitz | nomadnet (8be242191a5f), zlotokrad (19f748afcc6c), jmeshy (d0f1dbd3b8cc), London_Whitechapel (65d3e5684ad1), drkhsh (0421d110aced), Derpy - Cloud MC🦄🌐 (885b5767c45f), MK (0a331693069b), B (bddc55ed47b4), lazygravy (d0c56134b5f1), CarL_PetErson@MeshChatX (9d40d76f7bb2), metrafonic (1a87eb336f8b), 379-Dev (d58ad819e402), 379-RadioStation (12534efda978), jlamothe (8c981edbc6f5), Ф-butthead (32b6ef81e8e2), noderunner (85b0a24d2efd), CBOWork (56244cbdc291), kujeger (89e9244599fe), stephen (58a8de59f91e), rohs (7756d1d4c628), RandomDude (628fd60f546c), motte (1c539190202f), govardhana (292283af689f), Janus-MCX (a5cc2509d4e4)`

// notice47Lines holds capture 47's rendered notice rows: line 0 is the FIRST
// line's body (after the " [08:45:54] 󰙎 " prefix), the rest are the
// continuation lines — every one starting at the LEFT EDGE of the message
// pane with no timestamp, no indent and no leader.
var notice47Lines = []string{
	`members in general: qbit (b253938bf730), Nomad1n0 (d94777f6b45c), rmnd`,
	`(9ebb7a043a6d), 0a8b370a62de4c5464b7ef7f56ff33c8, PN42 (0c9b54f545a1),`,
	`464360ee59ed9938ba59f6677cf4ac4d, Marvin (4f5edd89b1ca), Ivan (9b246eead574), deka0`,
	`(f85b0a6349f1), hojan (f26e49bd7302), typhoon (3d2006d44aad), iamGBOX (2d00312e4279), Matt`,
	`(cf50aac62b32), fengKlump (d4e363fbe237), adept 🐲 (f6f8de525419), weechatter (44b0edc89bd4),`,
	`JWD (d30839368721), afri-orion (27716218762c), msjl.nl (1875cc3a1aa3), kilo40 (77c36219188a),`,
	`xl8r🚀 (4319c0f4c458), Mees electronics (e5b543d3f588), wiretapped (383571c943dd), CBOWork`,
	`(56244cbdc291), generalist (a9e16f04b125), tyrd (455abe2f53d7), nickie (fb511cbd0fc9), Kraven`,
	`Meshy Chat (7b9469ee74dc), jrrz (a45e6508a37d), Kraven The Hunter (425b7b6f8e6f), guvy`,
	`(ee5f14932a4a), VK2FAT (1747e565d5d1), 0xmp (433133b7eca4), kMan_PM (03830fc0e2c1), vclv`,
	`(9b0c460de2bb), zer0bitz | nomadnet (8be242191a5f), zlotokrad (19f748afcc6c), jmeshy`,
	`(d0f1dbd3b8cc), London_Whitechapel (65d3e5684ad1), drkhsh (0421d110aced), Derpy - Cloud MC🦄🌐`,
	`(885b5767c45f), MK (0a331693069b), B (bddc55ed47b4), lazygravy (d0c56134b5f1),`,
	`CarL_PetErson@MeshChatX (9d40d76f7bb2), metrafonic (1a87eb336f8b), 379-Dev (d58ad819e402),`,
	`379-RadioStation (12534efda978), jlamothe (8c981edbc6f5), Ф-butthead (32b6ef81e8e2), noderunner`,
	`(85b0a24d2efd), CBOWork (56244cbdc291), kujeger (89e9244599fe), stephen (58a8de59f91e), rohs`,
	`(7756d1d4c628), RandomDude (628fd60f546c), motte (1c539190202f), govardhana (292283af689f),`,
	`Janus-MCX (a5cc2509d4e4)`,
}

// noticeTagStrip removes the tview style tags the RRC render emits
// (`[#rrggbb]`, `[default]`, `[-]`, `[-:-:UR]`, …) so a tagged render line can
// be compared with the capture's plain text. The ts run's literal
// `[08:45:54]` brackets do NOT match (they carry no color spec), but the
// bodies under test carry no literal brackets either way.
var noticeTagStrip = regexp.MustCompile(`\[(?:#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|default|-)(?::-[a-zA-Z-]*)*\]`)

// TestFormatRRCNoticeWrapParityCapture47 pins the long-notice word-wrap
// geometry to capture 47: at the capture's 96-column message-pane inner
// width, Python's urwid.Text flow (Channels.py _message_widget notice branch
// → _wrap_text) breaks the `members in general:` body at word boundaries —
// never mid-word, never inside an emoji — and every CONTINUATION line starts
// at the LEFT EDGE of the pane with no timestamp, no indent and no leader.
func TestFormatRRCNoticeWrapParityCapture47(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	opts.Glyphs = map[string]string{"info": "󰙎"}
	tsMs := time.Date(2026, 9, 3, 8, 45, 54, 0, time.Local).UnixMilli()

	got := formatRRCMessageLines(ChannelMessage{
		Room: "general", Text: notice47Body, IsNotice: true, TsMs: tsMs,
	}, opts, 96)

	if len(got) != len(notice47Lines) {
		t.Fatalf("wrapped line count = %v, want %v (the capture's notice rows)\nlines:\n%v",
			len(got), len(notice47Lines), strings.Join(got, "\n"))
	}
	// The exact tagged lines: line 0 carries the " [HH:MM:SS] " ts run and
	// the info glyph inside the notice color run; every continuation line
	// opens with the notice color tag — the LEFT-EDGE body, no ts run.
	tsRun := colorTag(cubeHex3("#888"), "") + " [08:45:54] " + colorReset
	noticeTag := colorTag(noticeColor(ThemeDark), "")
	for i, line := range got {
		var want string
		if i == 0 {
			want = tsRun + noticeTag + "󰙎 " + notice47Lines[0] + colorReset
		} else {
			want = noticeTag + notice47Lines[i] + colorReset
		}
		if line != want {
			t.Errorf("notice line %v =\n%q\nwant\n%q", i, line, want)
		}
		// Every wrapped line fits the 96-column pane (tags excluded).
		if w := tview.TaggedStringWidth(line); w > 96 {
			t.Errorf("notice line %v visible width = %v, want <= 96", i, w)
		}
	}
}

// TestFormatRRCEventWrapShapes pins the system/notice/error wrap shapes:
// short rows stay on one line, join/part notices (the rrc layer records
// "→ <nick> joined" / "← <nick> left" as Kind "notice" with the arrow in the
// text) render the text AS-IS in the system color with no info glyph and no
// quoting, and wrapped continuations start at column 0 with no ts run.
func TestFormatRRCEventWrapShapes(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	opts.Glyphs = map[string]string{
		"arrow_r": "→", "arrow_l": "←", "info": "󰙎", "warning": "⚠",
	}
	sys := rrcRenderColors(ThemeDark)["system"]
	notice := rrcRenderColors(ThemeDark)["notice"]
	tsMs := time.Date(2026, 9, 3, 8, 42, 44, 0, time.Local).UnixMilli()

	t.Run("short notice stays on one line", func(t *testing.T) {
		got := formatRRCMessageLines(ChannelMessage{
			Text: "members in general: qbit", IsNotice: true, TsMs: tsMs,
		}, opts, 96)
		want := colorTag(cubeHex3("#888"), "") + " [08:42:44] " + colorReset +
			colorTag(notice, "") + "󰙎 members in general: qbit" + colorReset
		if len(got) != 1 || got[0] != want {
			t.Errorf("short notice lines = %q, want [%q]", got, want)
		}
	})

	t.Run("join/part notice renders the arrow text as-is", func(t *testing.T) {
		got := formatRRCMessageLines(ChannelMessage{
			Text: "→ CarL_PetErson@MeshChatX joined", IsNotice: true, TsMs: tsMs,
		}, opts, 96)
		want := colorTag(cubeHex3("#888"), "") + " [08:42:44] " + colorReset +
			colorTag(sys, "") + "→ CarL_PetErson@MeshChatX joined" + colorReset
		if len(got) != 1 || got[0] != want {
			t.Errorf("join/part notice lines = %q, want [%q]", got, want)
		}
		if strings.Contains(got[0], "󰙎") {
			t.Errorf("join/part notice must carry no info glyph: %q", got[0])
		}
	})

	t.Run("system row keeps the derived arrow icon", func(t *testing.T) {
		got := formatRRCMessageLines(ChannelMessage{
			Text: "alice left", IsSystem: true, TsMs: tsMs,
		}, opts, 96)
		want := colorTag(cubeHex3("#888"), "") + " [08:42:44] " + colorReset +
			colorTag(sys, "") + "← alice left" + colorReset
		if len(got) != 1 || got[0] != want {
			t.Errorf("system lines = %q, want [%q]", got, want)
		}
	})

	t.Run("long continuation lines start at column 0 with no ts", func(t *testing.T) {
		text := strings.Repeat("word ", 30) // 150 chars > 40
		got := formatRRCMessageLines(ChannelMessage{
			Text: text, IsNotice: true, TsMs: tsMs,
		}, opts, 40)
		if len(got) < 2 {
			t.Fatalf("long notice wrapped into %v lines, want several", len(got))
		}
		for i, line := range got {
			if w := tview.TaggedStringWidth(line); w > 40 {
				t.Errorf("line %v visible width = %v, want <= 40", i, w)
			}
			plain := noticeTagStrip.ReplaceAllString(line, "")
			if i == 0 {
				if !strings.HasPrefix(plain, " [08:42:44] ") {
					t.Errorf("first line = %q, want the ts prefix", plain)
				}
				continue
			}
			// Continuations carry NO timestamp run and no leading space.
			if strings.Contains(plain, "[08:42:44]") {
				t.Errorf("continuation line %v repeats the timestamp: %q", i, plain)
			}
			if strings.HasPrefix(plain, " ") {
				t.Errorf("continuation line %v starts with an indent: %q", i, plain)
			}
		}
	})
}

// TestFormatRRCEventWrapWideRunes pins the UTF-8 wrap: an emoji is never
// split across lines and wraps land on word boundaries around it (the
// capture's `adept 🐲 (f6f8de525419)` row).
func TestFormatRRCEventWrapWideRunes(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	opts.Glyphs = map[string]string{"info": "󰙎"}
	tsMs := time.Date(2026, 9, 3, 8, 45, 54, 0, time.Local).UnixMilli()

	got := formatRRCMessageLines(ChannelMessage{
		Text: "adept 🐲 (f6f8de525419), hojan", IsNotice: true, TsMs: tsMs,
	}, opts, 20)
	want := []string{
		colorTag(cubeHex3("#888"), "") + " [08:45:54] " + colorReset +
			colorTag(noticeColor(ThemeDark), "") + "󰙎 adept" + colorReset,
		colorTag(noticeColor(ThemeDark), "") + "🐲 (f6f8de525419)," + colorReset,
		colorTag(noticeColor(ThemeDark), "") + "hojan" + colorReset,
	}
	if len(got) != len(want) {
		t.Fatalf("wide-rune wrap lines = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("wide-rune wrap line %v =\n%q\nwant\n%q", i, got[i], want[i])
		}
	}
}

// noticeColor returns the notice theme color for assertions.
func noticeColor(theme int) tcell.Color { return rrcRenderColors(theme)["notice"] }

// renderNoticeRoom draws a RoomWidget with the given messages at the fleet's
// chat geometry (120x37, the 96-wide chat inner) with the NERD glyph set the
// capture used (info = 󰙎) and returns the screen for per-cell assertions.
func renderNoticeRoom(t *testing.T, msgs []ChannelMessage) tcell.Screen {
	t.Helper()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphNerd)
	app.RRCRender.JustifyMsgs = true
	app.RRCRender.NickColors = true
	rw := NewRoomWidget(app, "RNS Community", "general")
	rw.SetMessages(msgs)

	screen := tcell.NewSimulationScreen("UTF-8")
	if screen == nil {
		t.Fatal("nil simulation screen")
	}
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(120, 37)

	widget := rw.Widget().(*tview.Flex)
	widget.SetRect(0, 0, 120, 37)
	widget.Draw(screen)
	return screen
}

// TestRoomWidgetNoticeRenderLeftEdge pins the END-TO-END rendering through
// the room widget: the capture-47 notice's continuation lines draw starting
// at the LEFT EDGE of the message pane (chat column 0), with the first row's
// body after the " [08:45:54] 󰙎 " prefix, and wide runes never misalign the
// per-cell walk.
func TestRoomWidgetNoticeRenderLeftEdge(t *testing.T) {
	t.Parallel()

	tsMs := time.Date(2026, 9, 3, 8, 45, 54, 0, time.Local).UnixMilli()
	msgs := []ChannelMessage{
		{Room: "general", Text: notice47Body, IsNotice: true, TsMs: tsMs},
	}

	screen := renderNoticeRoom(t, msgs)

	// Locate the notice's first body row: the row whose chat column 14
	// starts the body ("members…"). The pane rows above hold the header.
	firstRow := -1
	for y := 1; y < 34; y++ {
		r, _, _ := chatCell(screen, y, 14)
		if r == 'm' {
			if r2, _, _ := chatCell(screen, y, 15); r2 == 'e' {
				firstRow = y
				break
			}
		}
	}
	if firstRow < 0 {
		t.Fatal("notice first body row not found in the chat pane")
	}

	// The ts run occupies the pane's first 12 columns...
	mustChatCol(t, screen, firstRow, 0, ' ', tcellColorFrom256orHex(135, 135, 135), "ts leading space")
	mustChatCol(t, screen, firstRow, 1, '[', tcellColorFrom256orHex(135, 135, 135), "ts open")
	mustChatCol(t, screen, firstRow, 10, ']', tcellColorFrom256orHex(135, 135, 135), "ts close")
	// ...the info glyph follows at col 12 (the capture's 󰙎 at (255,215,95))...
	mustChatCol(t, screen, firstRow, 12, '󰙎', tcellColorFrom256orHex(255, 215, 95), "info glyph")
	mustChatCol(t, screen, firstRow, 14, 'm', tcellColorFrom256orHex(255, 215, 95), "body start")

	// ...and every continuation line starts at chat col 0 with the notice
	// color and no ts run (the capture's `(9ebb7a043a6d),` rows). The per-cell
	// walk advances by each rune's CELL width, so wide emoji never misalign.
	wantNotice := tcellColorFrom256orHex(255, 215, 95)
	for i, wantLine := range notice47Lines[1:] {
		runes := []rune(wantLine)
		y := firstRow + 1 + i
		r, _, _ := chatCell(screen, y, 0)
		if r != runes[0] {
			t.Fatalf("continuation row %v starts with %q, want %q", i, string(r), string(runes[0]))
		}
		_, fg, _ := chatCell(screen, y, 0)
		if fg != wantNotice {
			t.Errorf("continuation row %v col 0 fg = %v, want the notice color", i, fg)
		}
		col := 0
		for _, wantR := range runes {
			r, _, _ := chatCell(screen, y, col)
			if r != wantR {
				t.Errorf("continuation row %v col %v = %q, want %q", i, col, string(r), string(wantR))
				break
			}
			col += cellWidth(wantR)
		}
	}
}
