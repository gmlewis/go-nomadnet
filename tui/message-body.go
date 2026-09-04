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
	"github.com/rivo/tview"

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

// rrcRenderColors returns the RRC render's theme colors. The message-row
// colors (text/ts/mention/link/self/peer) are the Channels.py theme dicts'
// 3-hex specs NIBBLE-DOUBLED like the Python micron render path; the
// system/notice/error rows instead carry the STATIC palette attrs
// (irc_system/irc_notice/irc_error, ui/TextUI.py:66-68) whose 3-hex specs
// urwid routes through the 256-color cube first — so they (and the irc_ts
// prefix of system/notice/error rows, ui/TextUI.py:63) are CUBE-quantized
// here. Measured on the 2026-09-03 12:32 full-fleet capture (mac row 24):
// the notice ts run (135,135,135) and the 󰙎 Welcome run (255,215,95).
func rrcRenderColors(theme int) map[string]tcell.Color {
	if theme == ThemeLight {
		return map[string]tcell.Color{
			"text":    nibbleHex3("#111"),
			"ts":      nibbleHex3("#888"),
			"notice":  cubeHex3("#a70"),
			"error":   cubeHex3("#a22"),
			"system":  cubeHex3("#888"),
			"ircTs":   cubeHex3("#888"),
			"link":    nibbleHex3("#79d"),
			"mention": nibbleHex3("#c50"),
			"self":    nibbleHex3("#3a0"),
			"peer":    nibbleHex3("#077"),
		}
	}
	return map[string]tcell.Color{
		"text":    nibbleHex3("#ddd"),
		"ts":      nibbleHex3("#888"),
		"notice":  cubeHex3("#fd3"),
		"error":   cubeHex3("#f55"),
		"system":  cubeHex3("#888"),
		"ircTs":   cubeHex3("#888"),
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
	JustifyMsgs            bool
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

// rrcTsText renders the timestamp's bracket text: "[HH:MM:SS]" for real
// timestamps, eight spaces inside the brackets for zero timestamps like
// Python _ts_prefix (Channels.py:1129-1135). Both are 10 columns.
func rrcTsText(tsMs int64) string {
	if tsMs == 0 {
		return "        "
	}
	return time.UnixMilli(tsMs).Format("15:04:05")
}

// rrcTsPrefix renders the "[HH:MM:SS] " timestamp prefix in the ts color;
// zero timestamps render eight spaces inside the brackets like Python
// _ts_prefix (Channels.py:1129-1135).
func rrcTsPrefix(tsMs int64, tsColor tcell.Color) string {
	return colorTag(tsColor, "") + "[" + rrcTsText(tsMs) + "] " + colorReset
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

// ircTsRun renders the system/notice/error ts prefix (" [HH:MM:SS] "): the
// LEADING space lives INSIDE the irc_ts-styled run (Python _ts_prefix,
// Channels.py:1129-1131 — ("irc_ts", " ["+t+"] ")), 12 chars at the static
// palette's cube color; zero timestamps render eight spaces inside the
// brackets like Python.
func ircTsRun(tsMs int64, tsColor tcell.Color) string {
	return colorTag(tsColor, "") + " [" + rrcTsText(tsMs) + "] " + colorReset
}

// rrcEventIconBody resolves a system/notice/error row's render shape: the
// event icon (empty when the text carries its own) and the body color.
// Join/part events arrive as Kind "system" rows whose text is "<nick>
// joined" / "<nick> left" (Python _record_system, no arrow in the text):
// the suffix-derived arrow below reproduces capture 47's `→ Nick joined` /
// `← Nick left` rows; older arrow-prefixed rows (the arrow already in the
// text) render the text AS-IS with no extra icon. Regular notices carry the
// info glyph in the notice color and errors the warning glyph in the error
// color.
func rrcEventIconBody(msg ChannelMessage, opts RRCRenderOpts) (icon string, color tcell.Color) {
	colors := rrcRenderColors(opts.Theme)
	switch {
	case msg.IsError:
		return opts.Glyphs["warning"], colors["error"]
	case isArrowPrefixedText(msg.Text, opts):
		return "", colors["system"]
	case msg.IsSystem:
		icon := opts.Glyphs["arrow_r"]
		if strings.HasSuffix(msg.Text, " left") {
			icon = opts.Glyphs["arrow_l"]
		}
		return icon, colors["system"]
	default:
		return opts.Glyphs["info"], colors["notice"]
	}
}

// isArrowPrefixedText reports whether text already carries a join/part arrow
// followed by a space — one of the port's configured arrow glyphs or the
// plain Unicode arrows — so the render must not add its own event icon.
func isArrowPrefixedText(text string, opts RRCRenderOpts) bool {
	candidates := []string{"→", "←", opts.Glyphs["arrow_r"], opts.Glyphs["arrow_l"]}
	for _, arrow := range candidates {
		if arrow != "" && strings.HasPrefix(text, arrow+" ") {
			return true
		}
	}
	return false
}

// formatRRCEventLines renders a system/notice/error row the way Python's
// _wrap_text flow does (Channels.py:1281-1311): the " [HH:MM:SS] " ts run,
// the event icon and the body form ONE text flow wrapped at the message
// pane's inner width (urwid.Text "space" wrap), and every CONTINUATION line
// starts at the LEFT EDGE of the pane with no timestamp, no indent and no
// leader — capture 47's `members in general:` notice whose `(9ebb…)`
// continuations start at column 0. Chat rows keep their justified layout.
// A width too small to hold the prefix falls back to the single-line render.
func formatRRCEventLines(msg ChannelMessage, opts RRCRenderOpts, width int) []string {
	colors := rrcRenderColors(opts.Theme)
	icon, color := rrcEventIconBody(msg, opts)
	iconPart := ""
	if icon != "" {
		iconPart = icon + " "
	}
	tsRun := ircTsRun(msg.TsMs, colors["ircTs"])
	flow := " [" + rrcTsText(msg.TsMs) + "] " + iconPart + msg.Text
	oneLine := func() []string {
		return []string{tsRun + colorTag(color, "") + iconPart + msg.Text + colorReset}
	}
	if width <= 0 {
		return oneLine()
	}
	segments := urwidSpaceWrap(flow, width)
	// The ts run is always 12 columns (" [HH:MM:SS] " / " [        ] "); the
	// icon part adds its runes when present.
	prefixLen := 12 + utf8.RuneCountInString(iconPart)
	first := []rune(segments[0])
	if len(first) < prefixLen {
		// The prefix alone overflowed the width; keep the single-line form.
		return oneLine()
	}
	lines := make([]string, 0, len(segments))
	// The first line carries the ts run, then the icon + the wrapped body
	// part (the icon's own color run covers it, like the capture's
	// `[ts][notice]󰙎 members…`); the body part is segment 0 minus the
	// prefix runes.
	lines = append(lines, tsRun+colorTag(color, "")+iconPart+string(first[prefixLen:])+colorReset)
	for _, seg := range segments[1:] {
		lines = append(lines, colorTag(color, "")+seg+colorReset)
	}
	return lines
}

// formatRRCMessage renders one chat message to a tview color-tagged string,
// mirroring Python _message_widget. system/notice/error rows render as a
// single full-width urwid.Text (Channels.py:1293-1311, _wrap_text — no
// columns and no left pad): the " [HH:MM:SS] " run in the irc_ts palette
// color, then the icon + body in the kind's palette color. Message rows
// carry the one-column left indent of Python's urwid.Padding(left=1); the
// justified layout (formatRRCMessageLines) renders their two-column form.
func formatRRCMessage(msg ChannelMessage, opts RRCRenderOpts) string {
	if msg.IsSystem || msg.IsNotice || msg.IsError {
		return strings.Join(formatRRCEventLines(msg, opts, 0), "\n") + "\n"
	}
	colors := rrcRenderColors(opts.Theme)
	var sb strings.Builder
	sb.WriteString(" " + rrcTsPrefix(msg.TsMs, colors["ts"]) +
		colorTag(rrcNickColor(msg, opts), "") + "<" + rrcSenderName(msg) + ">" + colorReset + " " +
		formatRRCBody(msg, opts) + colorReset + "\n")
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

// Justified-layout render (rrc_ui_justify_msgs=True default, Channels.py
// 1408-1413): Python renders each message as
// urwid.Columns([(PACK, ts-prefix), (body)], dividechars=1) padded left=1, so
// chat col 0 is a DEFAULT-styled pad space, cols 1-11 the "[HH:MM:SS] " run
// (11 chars, fg #888888), col 12 a DEFAULT-styled gap space (the column
// divider, carrying no fg), and the body column starts at col 13 with
// "<nick>". The body column wraps in the remaining width and EVERY wrapped
// continuation line starts at col 13 — the same column as the "<" — with a
// default-styled pad (measured on the 2026-09-03 12:32 full-fleet capture,
// mac rows 22/27/28: pad len 1 default, ts len 11 (136,136,136), gap len 1
// default, nick palette at col 13, body #dddddd from col 13 to the edge).

const (
	// rrcJustifyIndent is the chat column the body column starts at: the
	// one-column pad, the 11-char "[HH:MM:SS] " prefix and the one-column
	// divider gap.
	rrcJustifyIndent = 13

	// defaultPadTag pins the pad/gap spaces to the default foreground so
	// they do not inherit the body color (the urwid Columns divider and the
	// Padding filler carry no attr), clearing any underline/reverse latch
	// carried across wrapped lines. [-:-:UR] restores the TextView's base
	// (body) color with the toggles cleared.
	defaultPadTag = "[default:-:UR]"
)

// formatRRCMessageLines renders one chat message into the newline-free tagged
// lines of the justified two-column layout at the given chat-inner width
// (Python wraps the body column at the inner width minus the prefix columns).
// For system/notice/error rows Python renders a single full-width urwid.Text
// (_wrap_text, no columns and no left pad — the leading space lives inside
// the irc_ts-styled run) which WORD-WRAPS at the pane width with continuation
// lines at column 0; formatRRCEventLines renders that shape.
func formatRRCMessageLines(msg ChannelMessage, opts RRCRenderOpts, width int) []string {
	if msg.IsSystem || msg.IsNotice || msg.IsError {
		return formatRRCEventLines(msg, opts, width)
	}
	if width <= rrcJustifyIndent {
		return []string{strings.TrimSuffix(formatRRCMessage(msg, opts), "\n")}
	}

	bodyW := width - rrcJustifyIndent
	sender := rrcSenderName(msg)
	colors := rrcRenderColors(opts.Theme)
	// The body COLUMN holds "<sender> " plus the body (Channels.py:1411:
	// body_rendered renders nick_micron + message_body as one flow). A nick
	// too wide for the column (a pathological narrow chat pane) falls back to
	// the one-line render.
	nickLen := len("<" + sender + "> ")
	if nickLen > bodyW {
		return []string{strings.TrimSuffix(formatRRCMessage(msg, opts), "\n")}
	}
	column := "<" + sender + "> " + msg.Text
	segments := urwidSpaceWrap(column, bodyW)

	lines := make([]string, 0, len(segments))
	for i, seg := range segments {
		var line string
		if i == 0 {
			line = formatJustifiedHead(msg, opts, seg, nickLen)
		} else {
			line = defaultPadTag + strings.Repeat(" ", rrcJustifyIndent) +
				spanReset + formatRRCBody(ChannelMessage{Room: msg.Room, Text: seg}, opts)
		}
		// Python's urwid AttrMap rows paint the body attr across the FULL
		// body column (the 2026-09-03 12:32 capture's rows 22/27/28: the
		// body run + trailing spaces carry #dddddd to the chat box edge,
		// cols 13..96) — pad the tagged line to the inner width; the pad
		// is styled with the body color.
		if pad := width - tview.TaggedStringWidth(line); pad > 0 {
			line += colorTag(colors["text"], "") + strings.Repeat(" ", pad) + colorReset
		}
		lines = append(lines, line)
	}
	return lines
}

// formatJustifiedHead renders the first line of a justified message row: the
// default pad, the grey ts run, the default gap, the palette-colored
// "<sender>", and the body up to the first wrap. The styled nick run covers
// "<sender>" only — the space after it carries the body (base) color like the
// live capture's row 27 col 49.
func formatJustifiedHead(msg ChannelMessage, opts RRCRenderOpts, seg string, nickLen int) string {
	colors := rrcRenderColors(opts.Theme)
	// The wrapped first line may END inside the "<sender> " prefix: urwid's
	// space wrap breaks AT the nick's trailing space and drops it, leaving a
	// segment as short as "<sender>" (a narrow chat pane, or a wrap point
	// that walks back into the prefix). Python renders the nick as a styled
	// run inside one wrapping flow (Channels.py:1405-1413), so a partial
	// nick run on line one is normal — slice defensively instead of
	// panicking (the [17:16] crash of the JOINED-FANOUT live run,
	// logs/crash-20260904-143649.log).
	nick := seg
	body := ""
	if len(seg) >= nickLen {
		nick = seg[:nickLen-1]
		body = seg[nickLen:]
	}
	var sb strings.Builder
	sb.WriteString(defaultPadTag + " ")
	sb.WriteString(rrcTsPrefix(msg.TsMs, colors["ts"]))
	sb.WriteString("[default] ")
	sb.WriteString(colorTag(rrcNickColor(msg, opts), "") + nick + colorReset)
	sb.WriteString(" " + formatRRCBody(ChannelMessage{Room: msg.Room, Text: body}, opts))
	return sb.String()
}
