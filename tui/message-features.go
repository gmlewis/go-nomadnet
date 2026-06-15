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
	"sort"
	"strings"
	"unicode"
)

// LinkSpan represents a detected link in a message.
type LinkSpan struct {
	Kind   string // "lxmf", "page", or "room"
	Target string // the link target (hash, page addr, or room name)
}

// MentionSpan represents a detected @nick mention in a message.
type MentionSpan struct {
	Nick   string
	IsSelf bool
}

// linkRe matches lxmf@hash, page addresses, and #room links.
// Go's regexp lacks lookaheads/lookbehinds, so we match broadly
// and validate word boundaries in the scanner.
var linkRe = regexp.MustCompile(
	`lxmf@[0-9a-fA-F]{32}` +
		`|[0-9a-fA-F]{32}(:\S+)?` +
		`|#[A-Za-z0-9][A-Za-z0-9_\-]{0,62}`,
)

// wordChar returns true for characters that form part of a word.
func wordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_'
}

// ScanLinks finds all links in the given text and returns them as
// LinkSpan values. Matches Python's _scan_links function.
func ScanLinks(text string) []LinkSpan {
	if text == "" {
		return nil
	}
	var result []LinkSpan
	locs := linkRe.FindAllStringIndex(text, -1)
	for _, loc := range locs {
		match := text[loc[0]:loc[1]]
		start, end := loc[0], loc[1]

		if start > 0 && wordChar(text[start-1]) {
			if strings.HasPrefix(match, "lxmf@") {
				continue
			}
			if len(match) >= 32 && match[0] != '#' {
				continue
			}
			// Room links (#) must not be preceded by word char
			if match[0] == '#' {
				continue
			}
		}

		if end < len(text) && wordChar(text[end]) {
			if strings.HasPrefix(match, "lxmf@") || match[0] == '#' {
				continue
			}
		}

		switch {
		case strings.HasPrefix(match, "lxmf@"):
			result = append(result, LinkSpan{Kind: "lxmf", Target: match[5:]})
		case match[0] == '#':
			result = append(result, LinkSpan{Kind: "room", Target: match[1:]})
		default:
			result = append(result, LinkSpan{Kind: "page", Target: match})
		}
	}
	return result
}

// mentionRe matches @nick. Go lacks lookbehind, so we match broadly
// and validate the preceding character in the scanner.
var mentionRe = regexp.MustCompile(`@([A-Za-z0-9_]+)`)

// ScanMentions finds all @nick mentions in the given text.
// Self-mentions (matching ownNick, case-insensitive) are flagged.
func ScanMentions(text string, ownNick string) []MentionSpan {
	if text == "" || ownNick == "" {
		return nil
	}
	ownLower := strings.ToLower(ownNick)
	var result []MentionSpan
	for _, m := range mentionRe.FindAllStringSubmatchIndex(text, -1) {
		start := m[0]
		nickStart := m[2]
		nickEnd := m[3]

		// Check word boundary before: must not be preceded by alphanumeric or underscore
		if start > 0 {
			prev := text[start-1]
			if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') ||
				(prev >= '0' && prev <= '9') || prev == '_' {
				continue
			}
		}

		// Check word boundary after: must not be followed by alphanumeric or underscore
		if nickEnd < len(text) {
			next := text[nickEnd]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') ||
				(next >= '0' && next <= '9') || next == '_' {
				continue
			}
		}

		nick := text[nickStart:nickEnd]
		if strings.ToLower(nick) == ownLower {
			result = append(result, MentionSpan{Nick: nick, IsSelf: true})
		} else {
			result = append(result, MentionSpan{Nick: nick, IsSelf: false})
		}
	}
	return result
}

// CompleteNick returns sorted matching nick completions for the
// token at cursor position. Matches Python's _try_tab_complete logic.
func CompleteNick(text string, cursor int, members []ChannelMember, self string) []string {
	if cursor > len(text) {
		cursor = len(text)
	}

	start := cursor
	for start > 0 {
		ch := rune(text[start-1])
		if !isCompletionChar(ch) {
			break
		}
		start--
	}

	prefix := strings.ToLower(text[start:cursor])

	var candidates []string
	seen := make(map[string]bool)
	for _, m := range members {
		lower := strings.ToLower(m.Nick)
		if strings.HasPrefix(lower, prefix) && !seen[lower] {
			seen[lower] = true
			candidates = append(candidates, m.Nick)
		}
	}

	sort.Strings(candidates)
	return candidates
}

// isCompletionChar returns true for characters in a nick token.
func isCompletionChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-'
}
