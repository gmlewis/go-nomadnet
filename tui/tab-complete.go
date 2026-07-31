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
	"sort"
	"strings"
	"unicode"
)

// TabState tracks tab-completion cycling state, mirroring Python
// RoomMessageEdit._tab_state at Channels.py:458. All offsets are code-point
// (rune) indices, matching urwid's character-based edit_pos.
type TabState struct {
	Prefix      string // lowercased token being completed
	TokenStart  int    // rune offset where the replacement begins
	HasAt       bool   // token is preceded by '@'
	CursorAfter int    // cursor position after the last completion
	Idx         int    // current match index
}

// filterTabCandidates returns the member display names whose lowercase form
// starts with prefixLower, sorted by lowercase form, mirroring Python
// _candidates at Channels.py:439. The caller is responsible for deduplication
// and own-nick exclusion (Python builds a set in _candidates first); the sort
// is stable so input order is preserved for ties.
func filterTabCandidates(candidates []string, prefixLower string) []string {
	var matches []string
	for _, n := range candidates {
		if strings.HasPrefix(strings.ToLower(n), prefixLower) {
			matches = append(matches, n)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
	})
	return matches
}

// TabComplete performs one tab-completion step, mirroring Python's
// RoomMessageEdit._try_tab_complete at Channels.py:458. candidates must be the
// deduplicated, own-nick-excluded member display names. pos is a code-point
// cursor position. On success it returns the new text, the new cursor
// position, the updated cycling state, and true; it returns false when there
// is no alphanumeric token under the cursor or no candidate matches.
//
// A leading nick (at the start of the line) is completed as "nick: ", a
// mention ("@nick") keeps the leading "@", and a nick elsewhere is inserted
// bare — matching Python's replacement selection.
func TabComplete(text string, pos int, state *TabState, candidates []string) (string, int, *TabState, bool) {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	var (
		matches     []string
		prefixLower string
		tokenStart  int
		hasAt       bool
		idx         int
	)

	if state != nil && state.CursorAfter == pos {
		// Continue cycling the previous completion.
		prefixLower = state.Prefix
		tokenStart = state.TokenStart
		hasAt = state.HasAt
		matches = filterTabCandidates(candidates, prefixLower)
		if len(matches) == 0 {
			return "", 0, nil, false
		}
		idx = (state.Idx + 1) % len(matches)
	} else {
		// Fresh completion: scan the token left of the cursor.
		start := pos
		for start > 0 && isTokenChar(runes[start-1]) {
			start--
		}
		hasAt = start > 0 && runes[start-1] == '@'
		if hasAt {
			tokenStart = start - 1
		} else {
			tokenStart = start
		}
		token := string(runes[start:pos])
		if token == "" {
			return "", 0, nil, false
		}
		prefixLower = strings.ToLower(token)
		matches = filterTabCandidates(candidates, prefixLower)
		if len(matches) == 0 {
			return "", 0, nil, false
		}
		idx = 0
	}

	selected := matches[idx]
	var replacement string
	switch {
	case hasAt:
		replacement = "@" + selected
	case tokenStart == 0:
		replacement = selected + ": "
	default:
		replacement = selected
	}

	newRunes := make([]rune, 0, tokenStart+len(replacement)+len(runes)-pos)
	newRunes = append(newRunes, runes[:tokenStart]...)
	newRunes = append(newRunes, []rune(replacement)...)
	newRunes = append(newRunes, runes[pos:]...)
	newText := string(newRunes)
	newCursor := tokenStart + len([]rune(replacement))

	return newText, newCursor, &TabState{
		Prefix:      prefixLower,
		TokenStart:  tokenStart,
		HasAt:       hasAt,
		CursorAfter: newCursor,
		Idx:         idx,
	}, true
}

// isTokenChar reports whether r is part of a nick token, matching Python's
// `c.isalnum() or c in "_-"` at Channels.py:474.
func isTokenChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}
