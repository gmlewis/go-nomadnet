// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateStringPreservesRuneBoundaries verifies the rune-aware truncation
// helper used by the rendering path never splits a multibyte rune and appends
// the ellipsis only when the input exceeds the limit. TruncateString keeps at
// most maxVisible runes of the input and appends "..." when truncation
// occurred — the rune-safe replacement for the byte-wise `name[:8]+"..."`
// pattern that split multibyte runes and produced U+FFFD.
func TestTruncateStringPreservesRuneBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		input      string
		maxVisible int
		expect     string
	}{
		{name: "short_passthrough", input: "Müller", maxVisible: 20, expect: "Müller"},
		{name: "ascii_truncate", input: "abcdefghijklmnop", maxVisible: 8, expect: "abcdefgh..."},
		{name: "multibyte_truncate", input: "日本太郎こんにちは", maxVisible: 8, expect: "日本太郎こんにち..."},
		{name: "emoji_truncate", input: "😀😁😂😃😄😅😆😇", maxVisible: 1, expect: "😀..."},
		{name: "exact_fit", input: "日本太郎", maxVisible: 4, expect: "日本太郎"},
		{name: "zero_visible", input: "abc", maxVisible: 0, expect: "..."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := TruncateString(tc.input, tc.maxVisible)
			if got != tc.expect {
				t.Errorf("TruncateString(%q, %v) = %q, want %q", tc.input, tc.maxVisible, got, tc.expect)
			}
			if !utf8.ValidString(got) {
				t.Errorf("TruncateString produced invalid UTF-8: %q", got)
			}
			if strings.ContainsRune(got, '\uFFFD') {
				t.Errorf("TruncateString produced U+FFFD: %q", got)
			}
		})
	}
}

// TestHumanizeConfigKeyFirstRune verifies humanizeConfigKey capitalizes the
// first character by rune, not by byte, so a multibyte first character is not
// corrupted. (Config keys are ASCII in practice, but the helper must be
// UTF-8-safe.)
func TestHumanizeConfigKeyFirstRune(t *testing.T) {
	t.Parallel()
	// ASCII keys still work as before.
	if got := humanizeConfigKey("listen_ip"); got != "Listen Ip" {
		t.Errorf("humanizeConfigKey(listen_ip) = %q, want %q", got, "Listen Ip")
	}
	// A multibyte first character must round-trip unchanged (upper-cased by
	// rune, not split into an incomplete byte sequence).
	got := humanizeConfigKey("ñame")
	if !utf8.ValidString(got) {
		t.Errorf("humanizeConfigKey produced invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("humanizeConfigKey produced U+FFFD: %q", got)
	}
	if !strings.HasPrefix(got, "Ñ") && !strings.HasPrefix(got, "ñ") {
		t.Errorf("humanizeConfigKey(ñame) = %q, want first rune Ñ/ñ preserved", got)
	}
}
