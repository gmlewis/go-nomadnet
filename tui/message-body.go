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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"

	"github.com/gmlewis/go-nomadnet/nomadnet/util"
)

// The RRC message render mirrors Python RoomWidget._message_widget
// (Channels.py:1281-1427) and its theme dicts (Channels.py:22-45).
//
// Color-model note: the RRC render's 3-hex specs ("ddd", "888", …) flow
// through MicronParser high_color, which NIBBLE-DOUBLES them into 6-digit
// "#rrggbb" specs ("#dddddd"), so urwid's truecolor path renders them exact
// (221;221;221 for the body text — verified against the live #test captures).
// That differs from the static UI palette entries ("#bbb" etc.), which urwid
// routes through the 256-color cube first (175) — hence nibbleHex3 here
// versus cubeHex3 for the chrome colors in palette.go.

// nibbleHex3 expands a 3-digit "#rgb" spec by doubling each nibble
// ("#ddd" → #dddddd), matching MicronParser high_color + urwid's
// _parse_color_true 7-digit path.
func nibbleHex3(spec string) tcell.Color {
	if len(spec) != 4 || spec[0] != '#' {
		return tcell.ColorDefault
	}
	r, ok1 := hexNibble(spec[1])
	g, ok2 := hexNibble(spec[2])
	b, ok3 := hexNibble(spec[3])
	if !ok1 || !ok2 || !ok3 {
		return tcell.ColorDefault
	}
	return tcell.NewHexColor(int32(r)*0x11<<16 | int32(g)*0x11<<8 | int32(b)*0x11)
}

// rrcRenderColors returns the RRC render's theme colors (Channels.py theme
// dicts): all 3-hex specs nibble-doubled like the Python render path.
func rrcRenderColors(theme int) map[string]tcell.Color {
	if theme == ThemeLight {
		return map[string]tcell.Color{
			"text":    nibbleHex3("#111"),
			"ts":      nibbleHex3("#888"),
			"notice":  nibbleHex3("#a70"),
			"error":   nibbleHex3("#a22"),
			"system":  nibbleHex3("#888"),
			"link":    nibbleHex3("#79d"),
			"mention": nibbleHex3("#c50"),
			"self":    nibbleHex3("#3a0"),
			"peer":    nibbleHex3("#077"),
		}
	}
	return map[string]tcell.Color{
		"text":    nibbleHex3("#ddd"),
		"ts":      nibbleHex3("#888"),
		"notice":  nibbleHex3("#fd3"),
		"error":   nibbleHex3("#f55"),
		"system":  nibbleHex3("#888"),
		"link":    nibbleHex3("#79d"),
		"mention": nibbleHex3("#fb4"),
		"self":    nibbleHex3("#6c5"),
		"peer":    nibbleHex3("#3cd"),
	}
}

// RRCRenderOpts carries what the message render needs beyond the message
// itself: the app's rrc_* toggles, the effective palette and the local user's
// nick (Python reads app config and hub state inside _message_widget).
type RRCRenderOpts struct {
	Theme                  int
	Palette                []string
	NickColors             bool
	MentionColor           string
	ColorMentionTimestamps bool
	OwnNick                string
	Glyphs                 map[string]string
}

// chatSpan is one styled region of a message body (Python _body_markup's
// span tuples: start, end, kind, target).
type chatSpan struct {
	start, end int
	kind       string // "page", "lxmf", "room", "mention", "nick_mention"
	target     string
}

var (
	// chatLinkCoreRE mirrors the alternation core of Python _LINK_RE
	// (Channels.py:60-64); the (?<!…) / (?!\w) boundary conditions have no
	// RE2 equivalent and are checked manually in scanChatLinks.
	chatLinkCoreRE = regexp.MustCompile(
		`lxmf@[0-9a-fA-F]{32}` +
			`|[0-9a-fA-F]{32}(?::\S+)?` +
			`|#[A-Za-z0-9][A-Za-z0-9_\-]{0,62}`)

	chatCodeFenceRE = regexp.MustCompile("(?s)```[^\\n]*\\n.*?```")
	chatInlineRE    = regexp.MustCompile("`[^`\\n]+`")
)

// linkBoundaryOK checks the lookbehind/lookahead boundaries for a candidate
// match: prev/next are the runes around the match (present only when inside
// the body). lxmf/page carry a trailing (?!\w); room does not.
// isWordRune (linkify-motd.go) mirrors Python's \w.
func linkBoundaryOK(kind, body string, start, end int) bool {
	if start > 0 {
		prev, _ := utf8.DecodeLastRuneInString(body[:start])
		switch kind {
		case "lxmf", "room":
			if isWordRune(prev) {
				return false
			}
		case "page":
			if prev == '@' || isWordRune(prev) {
				return false
			}
		}
	}
	if kind != "room" && end < len(body) {
		next, _ := utf8.DecodeRuneInString(body[end:])
		if isWordRune(next) {
			return false
		}
	}
	return true
}

// scanChatLinks extracts link spans from the body, mirroring Python
// _scan_links: leftmost-match scanning, alternatives in lxmf/page/room
// order, with the regex's boundary lookarounds applied manually.
func scanChatLinks(body string) []chatSpan {
	var spans []chatSpan
	for pos := 0; pos < len(body); {
		loc := chatLinkCoreRE.FindStringIndex(body[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		m := body[start:end]
		var kind, target string
		switch {
		case strings.HasPrefix(m, "lxmf@"):
			kind, target = "lxmf", strings.TrimPrefix(m, "lxmf@")
		case strings.HasPrefix(m, "#"):
			kind, target = "room", strings.TrimPrefix(m, "#")
		default:
			kind, target = "page", m
		}
		if linkBoundaryOK(kind, body, start, end) {
			spans = append(spans, chatSpan{start, end, kind, target})
			pos = end
		} else {
			// Retry one rune later, like the regex engine's next attempt.
			_, size := utf8.DecodeRuneInString(body[start:])
			if size == 0 {
				size = 1
			}
			pos = start + size
		}
	}
	return spans
}

// scanChatSpans produces the body's styled spans: links and @own mentions,
// excluding spans that overlap code blocks and deduplicating to
// non-overlapping left-to-right order (Channels.py:152-198).
func scanChatSpans(body, ownNick string) []chatSpan {
	spans := scanChatLinks(body)
	if ownNick != "" && body != "" {
		pat := regexp.MustCompile(`@` + regexp.QuoteMeta(ownNick))
		for _, loc := range pat.FindAllStringIndex(body, -1) {
			if mentionBoundaryOK(body, loc[0], loc[1]) {
				spans = append(spans, chatSpan{loc[0], loc[1], "mention", ""})
			}
		}
	}
	spans = filterOverlappingChatSpans(spans, scanChatCodeBlocks(body))
	return nonOverlappingChatSpans(stableSortChatSpans(spans))
}

// mentionBoundaryOK checks the Python mention pattern's (?<![A-Za-z0-9_]) …
// (?![A-Za-z0-9_]) boundaries around an @<own_nick> match.
func mentionBoundaryOK(body string, start, end int) bool {
	if start > 0 {
		prev, _ := utf8.DecodeLastRuneInString(body[:start])
		if isWordRune(prev) {
			return false
		}
	}
	if end < len(body) {
		next, _ := utf8.DecodeRuneInString(body[end:])
		if isWordRune(next) {
			return false
		}
	}
	return true
}

// scanChatCodeBlocks returns ``` fence and `inline` code regions.
func scanChatCodeBlocks(body string) [][2]int {
	var regions [][2]int
	for _, loc := range chatCodeFenceRE.FindAllStringIndex(body, -1) {
		regions = append(regions, [2]int{loc[0], loc[1]})
	}
	for _, loc := range chatInlineRE.FindAllStringIndex(body, -1) {
		overlaps := false
		for _, r := range regions {
			if loc[0] < r[1] && loc[1] > r[0] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			regions = append(regions, [2]int{loc[0], loc[1]})
		}
	}
	return regions
}

// filterOverlappingChatSpans drops spans overlapping any code region.
func filterOverlappingChatSpans(spans []chatSpan, regions [][2]int) []chatSpan {
	out := make([]chatSpan, 0, len(spans))
	for _, s := range spans {
		blocked := false
		for _, r := range regions {
			if s.start < r[1] && s.end > r[0] {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, s)
		}
	}
	return out
}

// stableSortChatSpans orders spans by start position (stable for ties).
func stableSortChatSpans(spans []chatSpan) []chatSpan {
	out := make([]chatSpan, len(spans))
	copy(out, spans)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].start < out[j-1].start; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// nonOverlappingChatSpans keeps spans that start at or after the previous
// span's end (Channels.py:165-172).
func nonOverlappingChatSpans(spans []chatSpan) []chatSpan {
	filtered := make([]chatSpan, 0, len(spans))
	lastEnd := 0
	for _, s := range spans {
		if s.start >= lastEnd {
			filtered = append(filtered, s)
			lastEnd = s.end
		}
	}
	return filtered
}

// rrcTsPrefix renders the "[HH:MM:SS] " timestamp prefix in the ts color;
// zero timestamps render eight spaces inside the brackets like Python
// _ts_prefix (Channels.py:1129-1135).
func rrcTsPrefix(tsMs int64, tsColor tcell.Color) string {
	t := time.UnixMilli(tsMs).Format("15:04:05")
	if tsMs == 0 {
		t = "        "
	}
	return colorTag(tsColor, "") + "[" + t + "] " + colorReset
}

// rrcNickColor picks the palette color for a message's sender hash; without
// nick_colors it falls back to the theme's self/peer color (Channels.py:1398).
func rrcNickColor(msg ChannelMessage, opts RRCRenderOpts) tcell.Color {
	if opts.NickColors && msg.SrcHash != "" {
		return NickColorByHashHexColor(msg.SrcHash, opts.Palette)
	}
	colors := rrcRenderColors(opts.Theme)
	if opts.NickColors {
		// Palette mode with an unknown hash: Python's get_nick_color returns
		// theme["nick_peer"] for non-bytes hashes (Channels.py:1260).
		return colors["peer"]
	}
	if msg.IsSelf {
		return colors["self"]
	}
	return colors["peer"]
}

// rrcSenderName resolves the displayed sender: the nick when present, else
// the first 12 hex of the source hash, else "?" (Channels.py:1323-1325).
func rrcSenderName(msg ChannelMessage) string {
	if msg.Nick != "" {
		n := msg.Nick
		if s := util.SanitizeName(&n); s != nil {
			return *s
		}
	}
	if len(msg.SrcHash) >= 12 {
		return msg.SrcHash[:12]
	}
	if msg.SrcHash != "" {
		return msg.SrcHash
	}
	return "?"
}

// formatRRCMessage renders one chat message to a tview color-tagged string,
// mirroring Python _message_widget: a grey [HH:MM:SS] prefix, the
// palette-colored <sender> (get_nick_color of the sender hash), and the body
// with linkified hash runs (TODO items 10-13). Message rows carry the one-
// column left indent of Python's urwid.Padding(left=1); system, notice and
// error rows instead start with the leading space inside _ts_prefix.
func formatRRCMessage(msg ChannelMessage, opts RRCRenderOpts) string {
	colors := rrcRenderColors(opts.Theme)
	var sb strings.Builder
	switch {
	case msg.IsSystem:
		icon := opts.Glyphs["arrow_r"]
		if strings.HasSuffix(msg.Text, " left") {
			icon = opts.Glyphs["arrow_l"]
		}
		sb.WriteString(" " + rrcTsPrefix(msg.TsMs, colors["ts"]) +
			colorTag(colors["system"], "") + icon + " " + msg.Text + colorReset + "\n")
	case msg.IsNotice:
		sb.WriteString(" " + rrcTsPrefix(msg.TsMs, colors["ts"]) +
			colorTag(colors["notice"], "") + opts.Glyphs["info"] + " " + msg.Text + colorReset + "\n")
	case msg.IsError:
		sb.WriteString(" " + rrcTsPrefix(msg.TsMs, colors["ts"]) +
			colorTag(colors["error"], "") + opts.Glyphs["warning"] + " " + msg.Text + colorReset + "\n")
	default:
		sb.WriteString(" " + rrcTsPrefix(msg.TsMs, colors["ts"]) +
			colorTag(rrcNickColor(msg, opts), "") + "<" + rrcSenderName(msg) + ">" + colorReset + " " +
			formatRRCBody(msg, opts) + colorReset + "\n")
	}
	return sb.String()
}

// formatRRCBody renders the message body with linkified hash runs, room
// links and code-block protection (Python _body_markup + the link_ branch
// of _message_widget: underline + the link palette color).
func formatRRCBody(msg ChannelMessage, opts RRCRenderOpts) string {
	body := msg.Text
	spans := scanChatSpans(body, opts.OwnNick)
	if len(spans) == 0 {
		return body
	}
	colors := rrcRenderColors(opts.Theme)
	var sb strings.Builder
	pos := 0
	for _, s := range spans {
		if s.start > pos {
			sb.WriteString(body[pos:s.start])
		}
		seg := body[s.start:s.end]
		switch s.kind {
		case "mention":
			color := colors["mention"]
			if opts.MentionColor != "" {
				color = hexColorOr(opts.MentionColor, color)
			}
			sb.WriteString(colorTag(color, "r") + seg + spanReset)
		default:
			// Links render underlined in the link palette color
			// (Channels.py:1373: underline + `F{t['link']}`). The span reset
			// must explicitly clear the underline/reverse toggles: the fork's
			// bare [-] resets colors only, so underline would leak into the
			// wrapped continuation line (the styled-tview renderer uses the
			// same [-:-:U] latch).
			sb.WriteString(colorTag(colors["link"], "u") + seg + spanReset)
		}
		pos = s.end
	}
	if pos < len(body) {
		sb.WriteString(body[pos:])
	}
	return sb.String()
}

func colorTag(c tcell.Color, flags string) string {
	if flags == "" {
		return "[" + colorName(c) + "]"
	}
	return "[" + colorName(c) + "::" + flags + "]"
}

// colorName formats a tcell color as a tview color tag value: "default" for
// the default color, "#rrggbb" for hex colors.
func colorName(c tcell.Color) string {
	if c == tcell.ColorDefault {
		return "default"
	}
	return fmt.Sprintf("#%06x", c.Hex()&0xffffff)
}

const colorReset = "[-]"

// spanReset closes a styled body span: it resets the colors AND explicitly
// clears the underline/reverse toggles (uppercase U/R in the fork's tag
// grammar), because a bare [-] would leave those attributes latched across
// line wraps.
const spanReset = "[-:-:UR]"

// hexColorOr parses a 3- or 6-digit hex color spec (the rrc_mention_color
// config value), nibble-doubling 3-digit specs like the micron render does;
// an invalid spec falls back to fallback.
func hexColorOr(spec string, fallback tcell.Color) tcell.Color {
	switch len(spec) {
	case 6:
		if v, ok := hexParse6(spec); ok {
			return tcell.NewHexColor(v)
		}
	case 3:
		return nibbleHex3("#" + spec)
	}
	return fallback
}
