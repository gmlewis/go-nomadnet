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
	"unicode"
	"unicode/utf8"
)

// ChunkByBytes splits text into chunks that each fit within budget UTF-8 bytes.
// It matches Python's _chunk_by_bytes (Channels.py:85) exactly: it takes a
// byte-budget prefix, strips trailing UTF-8 continuation bytes, decodes with
// errors="ignore", splits on the last whitespace boundary in the second half
// of the chunk (falling back to the whole chunk when there is none), rstrips
// the chunk, and lstrips the remainder. When the byte budget is smaller than a
// single code point, it keeps one code point.
func ChunkByBytes(text string, budget int) []string {
	if budget <= 0 || text == "" {
		return nil
	}

	var chunks []string
	remaining := text

	for remaining != "" {
		encoded := []byte(remaining)
		if len(encoded) <= budget {
			chunks = append(chunks, remaining)
			break
		}

		// Take a byte-budget prefix and strip trailing UTF-8 continuation
		// bytes so we do not cut in the middle of a multibyte code point.
		cut := encoded[:budget]
		for len(cut) > 0 && cut[len(cut)-1]&0xC0 == 0x80 {
			cut = cut[:len(cut)-1]
		}

		// Decode ignoring any incomplete trailing lead byte, then operate on
		// code points (Python str indexing is by code point).
		chunk := strings.ToValidUTF8(string(cut), "")
		chunkRunes := []rune(chunk)

		// Find the last whitespace boundary (space/newline/tab) and split there
		// when it lies in the second half of the chunk.
		lastSpace := -1
		for i, r := range chunkRunes {
			if r == ' ' || r == '\n' || r == '\t' {
				lastSpace = i
			}
		}
		if lastSpace > 0 && lastSpace >= len(chunkRunes)/2 {
			chunkRunes = chunkRunes[:lastSpace]
			chunk = string(chunkRunes)
		}

		// If the budget was too small to decode even one code point, take the
		// first code point of the remaining text (Python remaining[:1]).
		if len(chunkRunes) == 0 {
			_, size := utf8.DecodeRuneInString(remaining)
			chunk = remaining[:size]
			chunkRunes = []rune(chunk)
		}

		// Append the right-stripped chunk. Python advances remaining by the
		// pre-rstrip code-point length of chunk.
		consumed := len(chunkRunes)
		chunks = append(chunks, strings.TrimRightFunc(chunk, unicode.IsSpace))
		remaining = strings.TrimLeftFunc(sliceFromRunes(remaining, consumed), unicode.IsSpace)
	}

	return chunks
}

// sliceFromRunes returns s with its first n code points removed.
func sliceFromRunes(s string, n int) string {
	i := 0
	for n > 0 && i < len(s) {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n--
	}
	return s[i:]
}

// SplitMessage splits text into multiple messages that each fit within
// maxBytes, accounting for the "(i/N) " prefix. Returns nil if the prefix
// overhead alone exceeds the limit. Matches Python's _split_message
// (Channels.py:107), including the convergence loop that re-estimates the
// prefix size once the part count is known.
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
