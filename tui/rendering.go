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
	"strings"
)

// FormatChannelMessage returns a tview-compatible formatted string
// for a single channel message. Matches Python's ShowMessages() logic
// but as a pure formatter (no widget dependency).
func FormatChannelMessage(msg ChannelMessage, theme int) string {
	nickStyle := nickColor(msg.Nick)

	switch {
	case msg.IsSystem:
		return fmt.Sprintf("[gray]%v[-]", msg.Text)
	case msg.IsNotice:
		return fmt.Sprintf("[yellow]%v[-]", msg.Text)
	case msg.IsError:
		return fmt.Sprintf("[red]%v[-]", msg.Text)
	case msg.IsSelf:
		return fmt.Sprintf("[green]%v[-] %v", msg.Nick, msg.Text)
	default:
		return fmt.Sprintf("[%v]%v[-] %v", nickStyle, msg.Nick, msg.Text)
	}
}

// FormatConversationItem formats a conversation list entry into
// display text and secondary text. Matches Python's populateList() logic.
func FormatConversationItem(conv ConversationInfo, theme int) (text, secondary string) {
	prefix := "  "
	switch {
	case conv.Unread:
		prefix = "[!] "
	case conv.Failed:
		prefix = "[x] "
	}

	trustIcon := "○"
	switch conv.TrustLevel {
	case "trusted":
		trustIcon = "●"
	case "untrusted":
		trustIcon = "×"
	}

	text = fmt.Sprintf("%v%v %v", prefix, trustIcon, conv.DisplayName)
	secondary = fmt.Sprintf("%v — %v", RelativeTime(conv.LastTime), conv.LastMessage)
	return text, secondary
}

// StyledSpan represents a styled text segment for rendering.
type StyledSpan struct {
	Kind   string // "text", "link", "mention", "nick_mention", "code"
	Text   string
	Style  string // tview color attribute name
	Target string // link target (for link spans)
}

// spanInfo is an internal type for tracking spans during body markup parsing.
type spanInfo struct {
	start, end int
	kind       string // "mention", "nick_mention", or "link"
	linkKind   string // for links: "room", "lxmf", "page"
	text       string
	target     string
}

// BodyMarkup parses message body text into a list of styled spans.
// Links, mentions, and nick mentions are extracted; code blocks exclude
// mentions. Matches Python's _body_markup() logic.
// ownNick is the current user's nick for self-mention detection.
func BodyMarkup(body string, theme int, ownNick ...string) ([]StyledSpan, bool) {
	if body == "" {
		return nil, false
	}

	var own string
	if len(ownNick) > 0 {
		own = ownNick[0]
	}

	codeBlocks := ScanCodeBlocks(body)
	links := ScanLinks(body)

	// Build mention spans
	var mentions []MentionSpan
	if own != "" {
		mentions = ScanMentions(body, own)
	}

	// Also scan for nick mentions (other people's @nicks)
	var nickMentions []struct {
		start, end int
		nick       string
	}
	if own != "" {
		nickMentions = scanNickMentions(body, own)
	}

	var allSpans []spanInfo

	// Add link spans, using the byte offsets captured by ScanLinks so a
	// link that appears more than once (or whose target text also occurs
	// elsewhere as plain text) is placed at the right position, matching
	// Python's _body_markup which slices body[m.start():m.end()].
	for _, link := range links {
		if link.Start < 0 || link.End > len(body) || link.Start > link.End {
			continue
		}
		allSpans = append(allSpans, spanInfo{
			start:    link.Start,
			end:      link.End,
			kind:     "link",
			linkKind: link.Kind,
			text:     body[link.Start:link.End],
			target:   link.Target,
		})
	}

	// Add self-mention spans using the offsets captured by ScanMentions.
	for _, m := range mentions {
		if m.Start < 0 || m.End > len(body) || m.Start > m.End {
			continue
		}
		allSpans = append(allSpans, spanInfo{
			start: m.Start,
			end:   m.End,
			kind:  "mention",
			text:  body[m.Start:m.End],
		})
	}

	// Add nick mention spans
	for _, nm := range nickMentions {
		allSpans = append(allSpans, spanInfo{
			start: nm.start,
			end:   nm.end,
			kind:  "nick_mention",
			text:  body[nm.start:nm.end],
		})
	}

	// Filter out spans overlapping code blocks
	var filtered []spanInfo
	for _, s := range allSpans {
		if s.start >= 0 && !overlapsCodeBlock(s.start, s.end, codeBlocks) {
			filtered = append(filtered, s)
		}
	}

	// Sort by start position
	sortSpans(filtered)

	// Merge overlapping spans (keep earlier one)
	var merged []spanInfo
	lastEnd := 0
	for _, s := range filtered {
		if s.start >= lastEnd {
			merged = append(merged, s)
			lastEnd = s.end
		}
	}

	// Build output spans from text segments
	if len(merged) == 0 {
		return []StyledSpan{{Kind: "text", Text: body, Style: "body_text"}}, false
	}

	var out []StyledSpan
	pos := 0
	hasLinks := false

	for _, s := range merged {
		if s.start > pos {
			out = append(out, StyledSpan{
				Kind:  "text",
				Text:  body[pos:s.start],
				Style: "body_text",
			})
		}

		switch s.kind {
		case "mention":
			out = append(out, StyledSpan{
				Kind:  "mention",
				Text:  s.text,
				Style: "irc_mention",
			})
		case "nick_mention":
			out = append(out, StyledSpan{
				Kind:  "nick_mention",
				Text:  s.text,
				Style: "nick_mention",
			})
		case "link":
			out = append(out, StyledSpan{
				Kind:   "link",
				Text:   s.text,
				Style:  "link_" + s.linkKind,
				Target: s.target,
			})
			hasLinks = true
		}
		pos = s.end
	}

	if pos < len(body) {
		out = append(out, StyledSpan{
			Kind:  "text",
			Text:  body[pos:],
			Style: "body_text",
		})
	}

	return out, hasLinks
}

// scanNickMentions finds all @nick mentions that are NOT the own nick.
func scanNickMentions(text string, ownNick string) []struct {
	start, end int
	nick       string
} {
	ownLower := strings.ToLower(ownNick)
	var result []struct {
		start, end int
		nick       string
	}

	for _, m := range mentionRe.FindAllStringSubmatchIndex(text, -1) {
		start := m[0]
		nickStart := m[2]
		nickEnd := m[3]

		// Word boundary before
		if start > 0 {
			prev := text[start-1]
			if isWordByte(prev) {
				continue
			}
		}

		// Word boundary after
		if nickEnd < len(text) {
			next := text[nickEnd]
			if isWordByte(next) {
				continue
			}
		}

		nick := text[nickStart:nickEnd]
		if strings.ToLower(nick) != ownLower {
			result = append(result, struct {
				start, end int
				nick       string
			}{start: start, end: nickEnd, nick: nick})
		}
	}
	return result
}

// isWordByte checks if a byte is a word character (letter, digit, underscore).
func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// overlapsCodeBlock checks if a span overlaps any code block region.
func overlapsCodeBlock(start, end int, blocks []CodeBlockRegion) bool {
	for _, b := range blocks {
		if start < b.End && end > b.Start {
			return true
		}
	}
	return false
}

// sortSpans sorts spans by start position.
func sortSpans(spans []spanInfo) {
	for i := 1; i < len(spans); i++ {
		key := spans[i]
		j := i - 1
		for j >= 0 && spans[j].start > key.start {
			spans[j+1] = spans[j]
			j--
		}
		spans[j+1] = key
	}
}

// FormatHubEntry formats a hub entry for display in the channel list.
func FormatHubEntry(hub HubEntry) string {
	return fmt.Sprintf("%v %v", StatusIcon(hub.Status), hub.Name)
}

// FormatHubRoom formats a room entry for display under a hub.
func FormatHubRoom(room HubRoom) string {
	prefix := "  "
	switch {
	case room.Unread:
		prefix = "  [!] "
	case room.Joined:
		prefix = "  [*] "
	}
	return fmt.Sprintf("%v#%v", prefix, room.Name)
}

// FormatMemberStatus formats a member entry for the member list.
func FormatMemberStatus(member ChannelMember) (text, secondary string) {
	status := "○"
	if member.Online {
		status = "●"
	}
	text = fmt.Sprintf("%v %v", status, member.Nick)
	secondary = member.Hash
	return text, secondary
}
