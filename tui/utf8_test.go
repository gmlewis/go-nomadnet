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

// TestNonAsciiNamesRoundTrip verifies that announce/source/channel names
// containing multibyte UTF-8 characters round-trip through the rendering
// path unchanged — no rune is split, no U+FFFD replacement character is
// introduced. This is the Go port's Task 0.7 spec: "a fixture with non-ASCII
// names round-trips through the rendering path unchanged."
func TestNonAsciiNamesRoundTrip(t *testing.T) {
	t.Parallel()

	names := []string{
		"Müller",
		"日本太郎",
		"café",
		"€ rate",
		"😀 peer",
	}

	for _, name := range names {
		t.Run("announce/"+name, func(t *testing.T) {
			t.Parallel()
			ann := AnnounceEntry{Type: "node", DisplayName: name, SourceHash: "abcd1234"}
			got := FormatAnnounceFull(ann, false)
			if !utf8.ValidString(got) {
				t.Errorf("FormatAnnounceFull produced invalid UTF-8: %q", got)
			}
			if strings.ContainsRune(got, '\uFFFD') {
				t.Errorf("FormatAnnounceFull produced U+FFFD: %q", got)
			}
			if !strings.Contains(got, name) {
				t.Errorf("FormatAnnounceFull lost the name %q in %q", name, got)
			}
		})

		t.Run("conversation/"+name, func(t *testing.T) {
			t.Parallel()
			conv := ConversationInfo{DisplayName: name, TrustLevel: "trusted"}
			text, _ := FormatConversationItem(conv, ThemeDark)
			if !utf8.ValidString(text) {
				t.Errorf("FormatConversationItem text invalid UTF-8: %q", text)
			}
			if strings.ContainsRune(text, '\uFFFD') {
				t.Errorf("FormatConversationItem text contains U+FFFD: %q", text)
			}
			if !strings.Contains(text, name) {
				t.Errorf("FormatConversationItem lost the name %q in %q", name, text)
			}
		})

		t.Run("hub/"+name, func(t *testing.T) {
			t.Parallel()
			hub := HubEntry{Name: name, Status: "connected"}
			got := FormatHubEntry(hub)
			if !utf8.ValidString(got) {
				t.Errorf("FormatHubEntry invalid UTF-8: %q", got)
			}
			if !strings.Contains(got, name) {
				t.Errorf("FormatHubEntry lost the name %q in %q", name, got)
			}
		})

		t.Run("room/"+name, func(t *testing.T) {
			t.Parallel()
			room := HubRoom{Name: name, Joined: true}
			got := FormatHubRoom(room)
			if !utf8.ValidString(got) {
				t.Errorf("FormatHubRoom invalid UTF-8: %q", got)
			}
			if !strings.Contains(got, name) {
				t.Errorf("FormatHubRoom lost the name %q in %q", name, got)
			}
		})
	}
}

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
