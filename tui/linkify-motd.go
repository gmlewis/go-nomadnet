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
	"unicode"
	"unicode/utf8"
)

// motdRoomRe matches a `#room` reference in a hub MOTD. It mirrors the room-name
// portion of Python's _MOTD_ROOM_RE (Channels.py:1229); the two lookbehinds
// (`(?<!\[)` and `(?<!\w)`) are applied manually by LinkifyMOTD, since Go's
// regexp engine does not support lookarounds.
var motdRoomRe = regexp.MustCompile(`#([A-Za-z0-9][A-Za-z0-9_\-]{0,62})`)

// motdSpan is one #room reference found in a MOTD line.
type motdSpan struct {
	Start, End int // byte offsets of the "#room" text
	Room       string
}

// motdRoomSpans finds every #room reference Python's _MOTD_ROOM_RE matches in
// one MOTD line (Channels.py:1229). A `#` preceded by `[` (an already-linked
// room) or by a word character (e.g. `word#room`) is rejected, replicating the
// `(?<!\[)(?<!\w)` lookbehinds.
func motdRoomSpans(text string) []motdSpan {
	var spans []motdSpan
	pos := 0
	for pos < len(text) {
		loc := motdRoomRe.FindStringIndex(text[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		end := pos + loc[1]
		if start > 0 {
			prev, _ := utf8.DecodeLastRuneInString(text[:start])
			if prev == '[' || isWordRune(prev) {
				pos = start + 1
				continue
			}
		}
		spans = append(spans, motdSpan{Start: start, End: end, Room: text[start+1 : end]})
		pos = end
	}
	return spans
}

// LinkifyMOTD replaces #room references in a hub MOTD with Micron link markup
// of the form `[`#room`room://room]`, matching Python's _linkify_motd
// (Channels.py:1231).
func LinkifyMOTD(text string) string {
	if text == "" {
		return ""
	}
	var sb strings.Builder
	prev := 0
	for _, span := range motdRoomSpans(text) {
		sb.WriteString(text[prev:span.Start])
		fmt.Fprintf(&sb, "`[%v`room://%v]", text[span.Start:span.End], span.Room)
		prev = span.End
	}
	sb.WriteString(text[prev:])
	return sb.String()
}

// isWordRune reports whether r matches Python's \w under re.UNICODE (the
// default for str patterns): letters, digits, and the underscore connector.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
