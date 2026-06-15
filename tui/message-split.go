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

// ChunkByBytes splits text into chunks that each fit within budget
// UTF-8 bytes. Splits prefer word boundaries (spaces/newlines/tabs)
// when possible. Matches Python's _chunk_by_bytes exactly.
func ChunkByBytes(text string, budget int) []string {
	if budget <= 0 || text == "" {
		return nil
	}

	var parts []string
	remaining := text

	for remaining != "" {
		if len([]byte(remaining)) <= budget {
			parts = append(parts, remaining)
			break
		}

		// Take first budget characters (Python: remaining[:budget])
		runes := []rune(remaining)
		n := budget
		if n > len(runes) {
			n = len(runes)
		}

		// Encode to UTF-8, strip trailing continuation bytes
		encoded := []byte(string(runes[:n]))
		safe := len(encoded)
		for safe > 0 && (encoded[safe-1]&0xC0) == 0x80 {
			safe--
		}
		if safe == 0 {
			safe = len(encoded)
		}

		// Decode back to get safe character count
		chunkRunes := []rune(string(encoded[:safe]))
		chunkLen := len(chunkRunes)

		// Look for word boundary in second half of chunk
		splitRuneIdx := -1
		for i := chunkLen - 1; i >= 0; i-- {
			if chunkRunes[i] == ' ' || chunkRunes[i] == '\n' || chunkRunes[i] == '\t' {
				if i > chunkLen/2 {
					splitRuneIdx = i
					break
				}
			}
		}

		if splitRuneIdx >= 0 {
			// Apply split to remaining using rune indexing
			before := strings.TrimRight(string(runes[:splitRuneIdx]), " \n\t")
			after := strings.TrimLeft(string(runes[splitRuneIdx:]), " \n\t")
			parts = append(parts, before)
			remaining = after
		} else {
			// Fall back to single character
			if len(runes) == 0 {
				break
			}
			parts = append(parts, string(runes[0]))
			remaining = string(runes[1:])
		}
	}

	return parts
}

// SplitMessage splits text into multiple messages that each fit within
// maxBytes (accounting for the "(i/N) " prefix). Returns nil if the
// prefix overhead alone exceeds the limit. Matches Python's
// _split_message exactly.
func SplitMessage(text string, maxBytes int) []string {
	if text == "" {
		return []string{""}
	}

	var parts []string
	for i := 0; i < 10; i++ {
		guess := len(parts)
		if guess < 1 {
			guess = 1
		}
		prefix := fmt.Sprintf("(%d/%d) ", guess, guess)
		budget := maxBytes - len(prefix)
		if budget <= 0 {
			return nil
		}
		parts = ChunkByBytes(text, budget)
		if len(parts) == guess {
			break
		}
	}

	K := len(parts)
	result := make([]string, K)
	for i, p := range parts {
		result[i] = fmt.Sprintf("(%d/%d) %s", i+1, K, p)
	}
	return result
}

// NeedsSplit reports whether text exceeds the given byte limit.
func NeedsSplit(text string, maxBytes int) bool {
	return len([]byte(text)) > maxBytes
}
