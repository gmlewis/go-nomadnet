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
	"sort"
	"strings"
	"time"
)

// WhoMember holds a single member entry from a /who response.
type WhoMember struct {
	Nick string // empty for bare hash entries
	Hash string // 12-char hex prefix (named) or 32-char full hash (bare)
}

// namedMemberRe matches a named member: "Nick (12hex)"
var namedMemberRe = regexp.MustCompile(`([A-Za-z0-9_][A-Za-z0-9_ ]*?)\s*\(([0-9a-fA-F]{12})\)`)

// bareHashRe matches a bare 32-char hex hash.
var bareHashRe = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// ParseWhoNotice parses a /who server response into a room name and
// member list. Matches Python's _parse_who_notice() exactly.
// Returns (room, members, error).
func ParseWhoNotice(text string) (string, []WhoMember, error) {
	if text == "" {
		return "", nil, fmt.Errorf("empty text")
	}

	prefix := "members in "
	if !strings.HasPrefix(text, prefix) {
		return "", nil, fmt.Errorf("not a /who response")
	}

	rest := text[len(prefix):]
	sepIdx := strings.Index(rest, ": ")
	if sepIdx < 0 {
		return "", nil, fmt.Errorf("missing ': ' separator")
	}
	room := strings.TrimSpace(rest[:sepIdx])
	room = strings.TrimPrefix(room, "#")
	room = strings.ToLower(room)
	if room == "" {
		return "", nil, fmt.Errorf("empty room name")
	}

	body := strings.TrimSpace(rest[sepIdx+2:])
	var members []WhoMember

	if body != "" && body != "(none)" {
		// Split by comma and parse each entry
		entries := strings.Split(body, ",")
		for _, entry := range entries {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}

			// Try named member: "Nick (12hex)"
			if m := namedMemberRe.FindStringSubmatch(entry); m != nil {
				nick := strings.TrimSpace(m[1])
				np := strings.ToLower(m[2])
				members = append(members, WhoMember{Nick: nick, Hash: np})
				continue
			}

			// Try bare hash: 32 hex chars
			entry = strings.TrimSpace(entry)
			if bareHashRe.MatchString(entry) {
				members = append(members, WhoMember{Hash: entry})
			}
		}
	}

	return room, members, nil
}

// ParseRoomListNotice parses a /list server response into a map
// of room names to topics. Matches Python's _parse_room_list_notice()
// exactly. Returns (rooms, error).
func ParseRoomListNotice(text string) (map[string]string, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}

	stripped := strings.TrimSpace(text)
	if stripped == "No public rooms registered" {
		return map[string]string{}, nil
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty text")
	}

	header := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(header, "Registered public rooms") {
		return nil, fmt.Errorf("not a room list response")
	}

	rooms := make(map[string]string)
	for _, line := range lines[1:] {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.Contains(s, " - ") {
			parts := strings.SplitN(s, " - ", 2)
			name := strings.TrimSpace(parts[0])
			topic := strings.TrimSpace(parts[1])
			// Strip leading # and lowercase
			name = strings.TrimPrefix(name, "#")
			name = strings.ToLower(name)
			if topic == "" {
				rooms[name] = ""
			} else {
				rooms[name] = topic
			}
		} else {
			name := strings.TrimSpace(s)
			name = strings.TrimPrefix(name, "#")
			name = strings.ToLower(name)
			rooms[name] = ""
		}
	}

	return rooms, nil
}

// MakeMentionPattern returns a function that checks whether a text
// contains an @nick mention with word boundary assertions.
// Go's regexp lacks lookbehind, so we use a manual check.
// Matches Python's _mention_re() behavior.
func MakeMentionPattern(nick string) (*regexp.Regexp, error) {
	if nick == "" {
		return nil, fmt.Errorf("empty nick")
	}
	// Match @nick (without boundary checks in the regex itself).
	// The caller should use MatchMention() for proper boundary handling.
	pat := regexp.MustCompile(`@` + regexp.QuoteMeta(nick))
	return pat, nil
}

// MatchMention returns true if text contains @nick with proper
// word boundary assertions (not preceded or followed by word chars).
func MatchMention(text, nick string) bool {
	if nick == "" || text == "" {
		return false
	}

	pattern := "@" + nick
	idx := 0
	for idx < len(text) {
		pos := strings.Index(text[idx:], pattern)
		if pos < 0 {
			return false
		}
		start := idx + pos
		endPos := start + len(pattern)

		// Check character after @nick
		if endPos < len(text) {
			ch := text[endPos]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '_' || ch == '@' {
				idx = endPos
				continue
			}
		}

		// Check character before @
		if start > 0 {
			ch := text[start-1]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '_' {
				idx = endPos
				continue
			}
		}

		return true
	}
	return false
}

// ShortHash truncates a hex hash string to n characters.
// Returns the full string if shorter than n. Matches Python's
// _short_hash() behavior.
func ShortHash(hash string, n int) string {
	if len(hash) <= n {
		return hash
	}
	return hash[:n]
}

// FormatTimestamp formats a millisecond timestamp as "HH:MM:SS".
// Matches Python's _format_ts() exactly.
func FormatTimestamp(tsMs int64) string {
	if tsMs < 0 {
		return ""
	}
	t := time.UnixMilli(tsMs).UTC()
	return t.Format("15:04:05")
}

// CodeBlockRegion represents a span of text that is inside a code block.
type CodeBlockRegion struct {
	Start int
	End   int
}

// ScanCodeBlocks finds all fenced and inline code blocks in text.
// Fenced blocks (``` ... ```) take priority over inline backticks.
// Matches Python's _scan_code_blocks() exactly.
func ScanCodeBlocks(text string) []CodeBlockRegion {
	if text == "" {
		return nil
	}

	var regions []CodeBlockRegion

	// Fenced blocks: ``` ... ```
	fenceRe := regexp.MustCompile("(?s)```[^\\n]*\\n.*?```")
	for _, m := range fenceRe.FindAllStringIndex(text, -1) {
		regions = append(regions, CodeBlockRegion{Start: m[0], End: m[1]})
	}

	// Inline backticks: `...` (not preceded by another backtick)
	inlineRe := regexp.MustCompile("(?s)`[^`\\n]+`")
	for _, m := range inlineRe.FindAllStringIndex(text, -1) {
		// Skip if inside a fenced block
		if !isInAnyRegion(m[0], regions) {
			regions = append(regions, CodeBlockRegion{Start: m[0], End: m[1]})
		}
	}

	// Sort by start position
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Start < regions[j].Start
	})

	return regions
}

// isInAnyRegion checks if pos falls within any of the given regions.
func isInAnyRegion(pos int, regions []CodeBlockRegion) bool {
	for _, r := range regions {
		if pos >= r.Start && pos < r.End {
			return true
		}
	}
	return false
}

// IsInCodeBlock checks if a position falls within any code block region.
func IsInCodeBlock(pos int, blocks []CodeBlockRegion) bool {
	return isInAnyRegion(pos, blocks)
}
