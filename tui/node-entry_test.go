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
	"testing"
)

// TestFormatNodeEntryRowPythonParity checks the KnownNodes NodeEntry row text
// against golden values captured from the live Python nomadnet NodeEntry
// (Network.py:984-1015) via tooling/tui-parity/node_entry_golden.py, which
// binds the real Directory.simplest_display_str/trust_level to a fake
// directory instance. The source hash is bytes(0..15) (16 bytes =
// RNS.Identity.TRUNCATED_HASHLENGTH//8), so its hex form is
// "000102030405060708090a0b0c0d0e0f". The node glyph "Ⓝ " has a trailing
// space, so every row contains two spaces between glyph and display text.
func TestFormatNodeEntryRowPythonParity(t *testing.T) {
	t.Parallel()

	srcHex := hex.EncodeToString(bytesRange(16)) // "000102030405060708090a0b0c0d0e0f"
	hexStr := "<" + srcHex + ">"
	g := GetGlyphSet(GlyphUnicode)

	cases := []struct {
		name string
		node NodeEntry
		want string
	}{
		{
			"trusted_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "trusted"},
			"Ⓝ  Alice",
		},
		{
			"untrusted_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "untrusted"},
			"Ⓝ  Alice " + hexStr,
		},
		{
			"warning_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "warning"},
			"Ⓝ  Alice " + hexStr,
		},
		{
			"unknown_in_dir_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "unknown"},
			"Ⓝ  Alice",
		},
		{
			"trusted_empty_name", NodeEntry{SourceHash: srcHex, DisplayName: "", TrustLevel: "trusted"},
			"Ⓝ  " + hexStr,
		},
		{
			"unknown_not_in_dir", NodeEntry{SourceHash: srcHex, DisplayName: "", TrustLevel: "unknown"},
			"Ⓝ  " + hexStr,
		},
		{
			"trusted_not_in_dir", NodeEntry{SourceHash: srcHex, DisplayName: "", TrustLevel: "trusted"},
			"Ⓝ  " + hexStr,
		},
		{
			// NodeEntry always calls simplest_display_str with san=False, so the
			// app sanitize_names config is ignored — modifiers are stripped, not
			// sanitized. "Hello > world" survives strip_modifiers unchanged.
			"strip_modifiers_not_sanitize", NodeEntry{SourceHash: srcHex, DisplayName: "Hello > world", TrustLevel: "trusted"},
			"Ⓝ  Hello > world",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatNodeEntryRow(tc.node, g)
			if got != tc.want {
				t.Errorf("row = %q\nwant  %q", got, tc.want)
			}
		})
	}
}

// TestNodeTrustStyleParity checks the NodeEntry trust-level → style mapping
// against Python (Network.py:993-1013). It is identical to AnnounceTrustStyle.
func TestNodeTrustStyleParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		trust             string
		style, focusStyle string
	}{
		{"trusted", "list_trusted", "list_focus_trusted"},
		{"untrusted", "list_untrusted", "list_focus_untrusted"},
		{"unknown", "list_unknown", "list_focus"},
		{"warning", "list_warning", "list_focus"},
		{"", "list_untrusted", "list_focus_untrusted"}, // else/default branch
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.trust, func(t *testing.T) {
			t.Parallel()
			style, focus := NodeTrustStyle(tc.trust)
			if style != tc.style || focus != tc.focusStyle {
				t.Errorf("(%q) = (%q,%q), want (%q,%q)", tc.trust, style, focus, tc.style, tc.focusStyle)
			}
		})
	}
}
