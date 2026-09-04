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
)

func renderTestOpts(theme int) RRCRenderOpts {
	return RRCRenderOpts{
		Theme:      theme,
		Palette:    DefaultNickPalette(theme),
		NickColors: true,
		Glyphs: map[string]string{
			"arrow_r": "→", "arrow_l": "←", "info": "ℹ", "warning": "⚠", "peer": "Ⓟ",
		},
	}
}

// TestFormatJustifiedHeadPrefixWrap pins the wrapped first line when the wrap
// point lands INSIDE the "<sender> " prefix: urwid's space wrap breaks at the
// nick's trailing space and drops it, leaving the segment exactly "<sender>"
// (len == nickLen-1). Python renders the nick as a styled run inside one
// wrapping flow (Channels.py:1405-1413), so the whole nick run stays on line
// one and the body continues on line two — the slice must not panic (the
// [17:16] crash of the JOINED-FANOUT live run, crash-20260904-143649.log:
// "<Anonymous Peer> JOINED-FANOUT:…" at a 47-column body).
func TestFormatJustifiedHeadPrefixWrap(t *testing.T) {
	t.Parallel()

	srcHex := "0102030405060708090a0b0c0d0e0f10"
	msg := ChannelMessage{
		Room: "general", Nick: "Anonymous Peer", SrcHash: srcHex,
		Text: "JOINED-FANOUT:0102030405060708090a0b0c0d0e0f10",
	}
	// bodyW 47 mirrors the live 120-column run: the first body word does not
	// fit after the 17-column prefix, so the walk-back break lands on the
	// nick's trailing space.
	lines := formatRRCMessageLines(msg, renderTestOpts(ThemeDark), 13+47)
	if len(lines) < 2 {
		t.Fatalf("lines = %v, want at least the prefix line plus the wrapped body", lines)
	}
	if !strings.Contains(lines[0], "<Anonymous Peer>") {
		t.Errorf("line 0 = %q, want the full nick run (Python's wrapped styled run)", lines[0])
	}
	if strings.Contains(lines[0], "JOINED-FANOUT") {
		t.Errorf("line 0 = %q, want no body on the prefix-only first line", lines[0])
	}
	if !strings.Contains(lines[1], "JOINED-FANOUT") {
		t.Errorf("line 1 = %q, want the wrapped body", lines[1])
	}

	// A narrower body (20 cols) hits the same break with a different wrap
	// position; the render must stay panic-free with the same shape.
	lines = formatRRCMessageLines(msg, renderTestOpts(ThemeDark), 13+20)
	if len(lines) < 2 {
		t.Fatalf("narrow lines = %v, want at least two rows", lines)
	}
	if !strings.Contains(lines[0], "<Anonymous Peer>") || strings.Contains(lines[0], "JOINED-FANOUT") {
		t.Errorf("narrow line 0 = %q, want the nick run only", lines[0])
	}

	// An even narrower body (the guard's edge): nickLen == bodyW is allowed
	// past the >bodyW fallback, and the whole prefix is still line one.
	lines = formatRRCMessageLines(msg, renderTestOpts(ThemeDark), 13+17)
	if len(lines) < 2 {
		t.Fatalf("edge lines = %v, want at least two rows", lines)
	}
}

// TestFormatRRCMessageGolden pins the full msg-row render against the model
// Python _message_widget produces (Channels.py:1281-1427), cross-checked with
// the live #test SGR capture: one-space indent, grey [#888888][HH:MM:SS]
// prefix (136;136;136), palette-colored <sender> by the SENDER HASH, plain
// body inheriting the #dddddd default.
func TestFormatRRCMessageGolden(t *testing.T) {
	t.Parallel()

	// 16-byte src 01..10 → (int.from_bytes(src,"big")+15) % 24 = 7
	// → DarkThemeNickColors[7] = 81b385 (Horner: 256 ≡ 16 mod 24).
	srcHex := "0102030405060708090a0b0c0d0e0f10"
	tsMs := time.Date(2026, 9, 3, 7, 57, 16, 0, time.Local).UnixMilli()
	got := formatRRCMessage(ChannelMessage{
		Room: "test", Nick: "Bob", SrcHash: srcHex, Text: "hello world", TsMs: tsMs,
	}, renderTestOpts(ThemeDark))
	want := " [#888888][" + time.UnixMilli(tsMs).Format("15:04:05") + "] [-]" +
		"[#81b385]<Bob>[-] hello world[-]\n"
	if got != want {
		t.Errorf("formatRRCMessage =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRRCMessageTimestampFormat pins the [HH:MM:SS] prefix shape and
// the zero-timestamp placeholder (Python _ts_prefix, Channels.py:1129-1135).
func TestFormatRRCMessageTimestampFormat(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 9, 3, 7, 56, 25, 0, time.Local).UnixMilli()
	if got := rrcTsPrefix(ts, tcell.NewHexColor(0x888888)); !strings.Contains(got, "[07:56:25] ") {
		t.Errorf("ts prefix = %q, want [07:56:25] inside", got)
	}
	if got := rrcTsPrefix(0, tcell.ColorDefault); !strings.Contains(got, "[        ] ") {
		t.Errorf("zero-ts prefix = %q, want the eight-space placeholder inside", got)
	}
	if got := rrcTsPrefix(0, tcell.NewHexColor(0x888888)); !strings.Contains(got, "#888888") {
		t.Errorf("ts prefix color = %q, want the ts color tag", got)
	}
}

// TestRRCRenderColorsBodyText pins item 11: the body default fg is the RRC
// render's nibble-doubled text color (#dddddd dark / #111111 light), the
// value the live Python capture shows (221;221;221).
func TestRRCRenderColorsBodyText(t *testing.T) {
	t.Parallel()

	dark := rrcRenderColors(ThemeDark)["text"]
	if got := uint32(dark.Hex()) & 0xffffff; got != 0xdddddd {
		t.Errorf("dark body text = #%06x, want #dddddd (live capture 221;221;221)", got)
	}
	light := rrcRenderColors(ThemeLight)["text"]
	if got := uint32(light.Hex()) & 0xffffff; got != 0x111111 {
		t.Errorf("light body text = #%06x, want #111111", got)
	}
	ts := rrcRenderColors(ThemeDark)["ts"]
	if got := uint32(ts.Hex()) & 0xffffff; got != 0x888888 {
		t.Errorf("ts color = #%06x, want #888888 (live capture 136;136;136)", got)
	}
	link := rrcRenderColors(ThemeDark)["link"]
	if got := uint32(link.Hex()) & 0xffffff; got != 0x7799dd {
		t.Errorf("link color = #%06x, want #7799dd (live capture 119;153;221)", got)
	}
}

// TestNickColorByHashHexColor pins item 10's color MODEL: the palette index
// comes from the SENDER HASH (Python get_nick_color), not the nick string.
func TestRRCNickColorBySrcHash(t *testing.T) {
	t.Parallel()

	// The 16-byte identity 01..10 → palette[7] = 81b385.
	if got, want := NickColorByHashHexColor("0102030405060708090a0b0c0d0e0f10", DarkThemeNickColors), tcell.NewHexColor(0x81b385); got != want {
		t.Errorf("nick color = %v, want %v", got.Hex(), want.Hex())
	}
	// The same nick STRING hashes differently than the same bytes — the
	// render must use the wire hash.
	if got, want := NickColorByHashHex("", DarkThemeNickColors), "#bbab00"; got != want {
		t.Errorf("empty hash color = %q, want %q ((0+15)%%24 = 15)", got, want)
	}
}

// TestFormatRRCMessageSelfUsesPalette pins that self messages color with the
// same palette-by-src model as peers (Python rrc_nick_colors=True default).
func TestFormatRRCMessageSelfUsesPalette(t *testing.T) {
	t.Parallel()

	srcHex := "0102030405060708090a0b0c0d0e0f10"
	got := formatRRCMessage(ChannelMessage{
		Nick: "Me", SrcHash: srcHex, Text: "hi", IsSelf: true,
	}, renderTestOpts(ThemeDark))
	if !strings.Contains(got, "[#81b385]<Me>") {
		t.Errorf("self message render = %q, want the palette color by src", got)
	}
}

// TestRRCNickColorFallback pins the non-palette mode: irc_nick_self for self
// messages, irc_nick_peer for others (Channels.py:1399).
func TestRRCNickColorFallback(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	opts.NickColors = false
	self := ChannelMessage{Nick: "Me", SrcHash: "01", IsSelf: true}
	peer := ChannelMessage{Nick: "Other", SrcHash: "02"}
	if got := rrcNickColor(self, opts); got != rrcRenderColors(ThemeDark)["self"] {
		t.Errorf("self color = %v, want irc_nick_self", got.Hex())
	}
	if got := rrcNickColor(peer, opts); got != rrcRenderColors(ThemeDark)["peer"] {
		t.Errorf("peer color = %v, want irc_nick_peer", got.Hex())
	}
}

// TestRRCSenderNameFallback pins the sender display model (Channels.py:1323):
// nick when present, else the 12-hex source prefix, else "?".
func TestRRCSenderNameFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		nick, srcHash, want string
	}{
		{"Bob", "0102030405060708090a0b0c0d0e0f10", "Bob"},
		{"", "aabbccddeeff112233445566", "aabbccddeeff"},
		{"", "", "?"},
	}
	for _, tc := range cases {
		got := rrcSenderName(ChannelMessage{Nick: tc.nick, SrcHash: tc.srcHash})
		if got != tc.want {
			t.Errorf("rrcSenderName(nick=%q, src=%q) = %q, want %q", tc.nick, tc.srcHash, got, tc.want)
		}
	}
}

// TestFormatRRCMessageLinkify pins item 13: 32-hex page runs, lxmf@ hashes
// and #room names render underlined in the link color (#7799dd), while code
// spans are left plain (Channels.py _body_markup + the link_ branch).
func TestFormatRRCMessageLinkify(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	hash := "c388d720f56483a8dc8668ee5bea3577"

	got := formatRRCMessage(ChannelMessage{
		Nick: "Bob", SrcHash: strings.Repeat("ab", 16), Text: "see " + hash + " now",
	}, opts)
	// src ab×16 → palette[18] = ad98fe (Horner: 256 ≡ 16 mod 24, 0xab ≡ 3).
	want := " [#888888][        ] [-][#ad98fe]<Bob>[-] see [#7799dd::u]" + hash + "[-:-:UR] now[-]\n"
	if got != want {
		t.Errorf("page-link render =\n%q\nwant\n%q", got, want)
	}

	got = formatRRCMessage(ChannelMessage{
		Nick: "Bob", SrcHash: strings.Repeat("ab", 16),
		Text: "lxmf@" + hash + " and #test",
	}, opts)
	if !strings.Contains(got, "[#7799dd::u]lxmf@"+hash+"[-:-:UR]") {
		t.Errorf("lxmf link render = %q", got)
	}
	if !strings.Contains(got, "[#7799dd::u]#test[-:-:UR]") {
		t.Errorf("room link render = %q", got)
	}

	// 31 hex chars: not a link.
	got = formatRRCMessage(ChannelMessage{
		Nick: "Bob", SrcHash: strings.Repeat("ab", 16), Text: strings.Repeat("a", 31),
	}, opts)
	if strings.Contains(got, "[#7799dd::u]") {
		t.Errorf("31-hex run must not linkify: %q", got)
	}

	// Inline code: hashes inside code regions are not linkified.
	got = formatRRCMessage(ChannelMessage{
		Nick: "Bob", SrcHash: strings.Repeat("ab", 16), Text: "`" + hash + "`",
	}, opts)
	if strings.Contains(got, "[#7799dd::u]") {
		t.Errorf("inline code must not linkify: %q", got)
	}
}

// TestScanChatSpansBoundaries pins the regex boundary semantics ported from
// Python _LINK_RE: 33-hex runs linkify their LAST 32 hex, a hash preceded by
// a word char does not match, and lxmf@ beats the bare-hash alternative.
func TestScanChatSpansBoundaries(t *testing.T) {
	t.Parallel()

	hash := "c388d720f56483a8dc8668ee5bea3577"

	// A 33-hex run: every candidate 32-hex window in Python fails a boundary
	// (the trailing word-char check at position 0, the lookbehind at 1), so
	// nothing linkifies.
	spans := scanChatLinks("a" + hash)
	if len(spans) != 0 {
		t.Errorf("33-hex run spans = %+v, want none (all windows fail a boundary)", spans)
	}

	// A non-word char before the hash lets the window at offset 1 through.
	spans = scanChatLinks("-" + hash)
	if len(spans) != 1 || spans[0].start != 1 || spans[0].end != 33 {
		t.Errorf("'-'+hash spans = %+v, want one page span at [1,33)", spans)
	}

	// Hash preceded by a word char: no match.
	if spans := scanChatLinks("z" + hash); len(spans) != 0 {
		t.Errorf("hash after word char must not linkify: %+v", spans)
	}

	// Hash preceded by '@': not a page link.
	if spans := scanChatLinks("@" + hash); len(spans) != 0 {
		t.Errorf("hash after @ must not linkify as page: %+v", spans)
	}

	// lxmf@ hash: the lxmf alternative wins and the trailing boundary holds.
	spans = scanChatLinks("ping lxmf@" + hash + ".")
	if len(spans) != 1 || spans[0].kind != "lxmf" {
		t.Errorf("lxmf spans = %+v, want one lxmf span", spans)
	}

	// Room names: leading word char blocks, trailing word char does not.
	spans = scanChatLinks("join #test and x#general")
	if len(spans) != 1 || spans[0].target != "test" {
		t.Errorf("room spans = %+v, want only #test", spans)
	}

	// Mentions with boundaries: @bob matches, x@bob and @bob1 do not.
	spans = scanChatSpans("hey @bob and x@bob and @bob1", "bob")
	if len(spans) != 1 || spans[0].start != 4 {
		t.Errorf("mention spans = %+v, want one at offset 4", spans)
	}
}

// TestFormatRRCMessageSystemNoticeError pins the system/notice/error render:
// the leading space INSIDE the irc_ts-styled run (Python _ts_prefix,
// Channels.py:1129-1131 — " [HH:MM:SS] ", 12 chars), the event icon, and the
// STATIC palette's cube-quantized kind colors (ui/TextUI.py:63-68, measured
// on the 2026-09-03 12:32 full-fleet capture, mac row 24: the " [12:21:08] "
// run is (135,135,135) and the 󰙎 Welcome run (255,215,95)).
func TestFormatRRCMessageSystemNoticeError(t *testing.T) {
	t.Parallel()

	opts := renderTestOpts(ThemeDark)
	text := "room test: unregistered; mode=(none); topic=(none)"

	got := formatRRCMessage(ChannelMessage{Text: text, IsNotice: true, TsMs: 0}, opts)
	want := colorTag(cubeHex3("#888"), "") + " [" + "        " + "] " + colorReset +
		colorTag(cubeHex3("#fd3"), "") + opts.Glyphs["info"] + " " + text + colorReset + "\n"
	if got != want {
		t.Errorf("notice render =\n%q\nwant\n%q", got, want)
	}

	got = formatRRCMessage(ChannelMessage{Text: "alice left", IsSystem: true, TsMs: 0}, opts)
	want = colorTag(cubeHex3("#888"), "") + " [" + "        " + "] " + colorReset +
		colorTag(cubeHex3("#888"), "") + opts.Glyphs["arrow_l"] + " alice left" + colorReset + "\n"
	if got != want {
		t.Errorf("system render =\n%q\nwant\n%q", got, want)
	}

	got = formatRRCMessage(ChannelMessage{Text: "boom", IsError: true, TsMs: 0}, opts)
	want = colorTag(cubeHex3("#888"), "") + " [" + "        " + "] " + colorReset +
		colorTag(cubeHex3("#f55"), "") + opts.Glyphs["warning"] + " boom" + colorReset + "\n"
	if got != want {
		t.Errorf("error render =\n%q\nwant\n%q", got, want)
	}
}
