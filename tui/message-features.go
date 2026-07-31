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
	Start  int    // byte offset of the match start in the source text
	End    int    // byte offset just past the match end
}

// MentionSpan represents a detected @nick mention in a message.
type MentionSpan struct {
	Nick   string
	IsSelf bool
	Start  int // byte offset of the "@nick" match start
	End    int // byte offset just past the match end
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
// LinkSpan values. Matches Python's _scan_links (Channels.py:75), including
// the _LINK_RE lookbehind/lookahead boundaries (Channels.py:60):
//
//   - lxmf@hash and #room are rejected when preceded by a word char
//     ((?<!\w) in Python).
//   - a bare 32-hex page address is rejected when preceded by a word char
//     or by '@' ((?<![@\w]) in Python).
//   - lxmf@hash and page addresses are rejected when followed by a word char
//     ((?!\w) in Python); #room has no trailing boundary.
//
// Because Go's regexp cannot express lookarounds, the scanner drives the
// regex manually and advances one byte past a rejected match start (rather
// than past the whole match) so a link hidden inside a rejected token — for
// example a 32-hex page inside a rejected "#hash" room — is still found,
// exactly as Python's lookbehind lets it.
func ScanLinks(text string) []LinkSpan {
	if text == "" {
		return nil
	}
	var result []LinkSpan
	pos := 0
	for pos < len(text) {
		loc := linkRe.FindStringIndex(text[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]
		match := text[start:end]

		accept := true
		// Leading boundary, mirroring Python's lookbehinds.
		if start > 0 {
			prev := text[start-1]
			switch {
			case strings.HasPrefix(match, "lxmf@"):
				if wordChar(prev) {
					accept = false
				}
			case match[0] == '#':
				if wordChar(prev) {
					accept = false
				}
			default: // page
				if wordChar(prev) || prev == '@' {
					accept = false
				}
			}
		}
		// Trailing boundary, mirroring Python's (?!\w) on lxmf and page.
		// #room has no trailing boundary in _LINK_RE.
		if accept && end < len(text) {
			next := text[end]
			if match[0] != '#' && wordChar(next) {
				accept = false
			}
		}

		if accept {
			switch {
			case strings.HasPrefix(match, "lxmf@"):
				result = append(result, LinkSpan{Kind: "lxmf", Target: match[5:], Start: start, End: end})
			case match[0] == '#':
				result = append(result, LinkSpan{Kind: "room", Target: match[1:], Start: start, End: end})
			default:
				result = append(result, LinkSpan{Kind: "page", Target: match, Start: start, End: end})
			}
			pos = end
		} else {
			pos = start + 1
		}
	}
	return result
}

// mentionRe matches @nick. Go lacks lookbehind, so we match broadly
// and validate the preceding character in the scanner.
var mentionRe = regexp.MustCompile(`@([A-Za-z0-9_]+)`)

// ScanMentions finds all self-mentions of ownNick in the given text.
// Matches Python's _scan_mentions (Channels.py:124), which yields only
// matches of the own nick (case-insensitive, word-bounded). The matched
// span is recorded as byte offsets in Start/End.
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
		if start > 0 && wordChar(text[start-1]) {
			continue
		}

		// Check word boundary after: must not be followed by alphanumeric or underscore
		if nickEnd < len(text) && wordChar(text[nickEnd]) {
			continue
		}

		nick := text[nickStart:nickEnd]
		if strings.ToLower(nick) == ownLower {
			result = append(result, MentionSpan{Nick: nick, IsSelf: true, Start: start, End: nickEnd})
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
