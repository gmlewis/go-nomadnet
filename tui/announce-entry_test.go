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
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// TestFormatAnnounceStreamRowPythonParity checks the announce-stream row text
// against golden values captured from the live Python nomadnet
// AnnounceStreamEntry (Network.py:259-390) via tooling/tui-parity-style capture.
// Fixed now = 2023-11-14 22:13:20 UTC; same-day announce uses %H:%M:%S, an
// announce 7 days ago uses %Y-%m-%d. Source hash is bytes(0..31) so the hex
// form is "000102…1f".
func TestFormatAnnounceStreamRowPythonParity(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0).UTC()
	today := now.Add(-time.Hour)
	otherDay := now.Add(-7 * 24 * time.Hour)
	srcHex := hex.EncodeToString(bytesRange(32)) // "000102…1f"
	g := GetGlyphSet(GlyphUnicode)

	longName := "A" + strings.Repeat("x", 40)

	cases := []struct {
		name            string
		ann             AnnounceEntry
		showDestination bool
		sanitize        bool
		want            string
	}{
		{
			"node_trusted_today", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Alice"}, false, false,
			"21:13:20 Ⓝ  Alice",
		},
		{
			"peer_untrusted_today", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "peer", DisplayName: "Bob"}, false, false,
			"21:13:20 Ⓟ  Bob",
		},
		{
			"pn_unknown_today", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "pn", DisplayName: "PN-1"}, false, false,
			"21:13:20 ↑ PN-1",
		},
		{
			"node_warning_otherday", AnnounceEntry{Timestamp: otherDay, SourceHash: srcHex, Type: "node", DisplayName: "Carol"}, false, false,
			"2023-11-07 Ⓝ  Carol",
		},
		{
			"long_name_truncate", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: longName}, false, false,
			"21:13:20 Ⓝ  " + truncateRunes(longName, 34),
		},
		{
			"show_destination_hexrep", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Alice"}, true, false,
			"21:13:20 Ⓝ  " + truncateRunes(srcHex, 34),
		},
		{
			"sanitize_on", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Hello > world"}, false, true,
			"21:13:20 Ⓝ  Hello world",
		},
		{
			"sanitize_off_strip_mods", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Hello > world"}, false, false,
			"21:13:20 Ⓝ  Hello > world",
		},
		{
			"empty_name_prettyhex", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: ""}, false, false,
			"21:13:20 Ⓝ  " + truncateRunes("<"+srcHex+">", 34),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatAnnounceStreamRow(tc.ann, now, tc.showDestination, tc.sanitize, g)
			if got != tc.want {
				t.Errorf("row = %q\nwant  %q", got, tc.want)
			}
		})
	}
}

// TestAnnounceTrustStyleParity checks the trust-level → urwid style mapping
// against Python AnnounceStreamEntry (Network.py:347-369).
func TestAnnounceTrustStyleParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		trust             string
		style, focusStyle string
	}{
		{"trusted", "list_trusted", "list_focus_trusted"},
		{"untrusted", "list_untrusted", "list_focus_untrusted"},
		{"unknown", "list_unknown", "list_focus"},
		{"warning", "list_warning", "list_focus"},
		{"", "list_untrusted", "list_focus_untrusted"}, // default branch
	}
	for _, tc := range cases {
		t.Run(tc.trust, func(t *testing.T) {
			t.Parallel()
			style, focus := AnnounceTrustStyle(tc.trust)
			if style != tc.style || focus != tc.focusStyle {
				t.Errorf("(%q) = (%q,%q), want (%q,%q)", tc.trust, style, focus, tc.style, tc.focusStyle)
			}
		})
	}
}

// bytesRange returns a slice of bytes 0..n-1.
func bytesRange(n int) []byte {
	b := make([]byte, n)
	for i := range n {
		b[i] = byte(i)
	}
	return b
}
