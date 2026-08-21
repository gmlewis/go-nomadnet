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

// TestFormatNodeEntryRowPythonParity is a LIVE cross-implementation check: it
// runs the stubbed-urwid golden capture script tooling/tui-parity/
// node_entry_golden.py fresh on every run. That script stubs gi/urwid (GLib is
// broken on this host), binds the real Directory.simplest_display_str /
// trust_level to a fake directory instance, instantiates the real Python
// nomadnet NodeEntry (Network.py:984-1015), and prints the row text + style
// for each named case. The Go test reproduces each case through
// FormatNodeEntryRow and compares the row text to the freshly captured Python
// value, matched by case name.
//
// The source hash is bytes(0..15) (16 bytes = RNS.Identity.
// TRUNCATED_HASHLENGTH//8), so its hex form is
// "000102030405060708090a0b0c0d0e0f". The node glyph "Ⓝ " has a trailing
// space, so every row contains two spaces between glyph and display text. The
// sanitize_on/sanitize_off pair confirms NodeEntry always calls
// simplest_display_str with san=False (modifiers stripped, not sanitized), so
// Go — which always strips modifiers — matches Python for both. The test
// SKIPs, not fails, when the Python reference is not importable or the script
// file is not accessible.
func TestFormatNodeEntryRowPythonParity(t *testing.T) {
	t.Parallel()

	srcHex := hex.EncodeToString(bytesRange(16)) // "000102030405060708090a0b0c0d0e0f"
	g := GetGlyphSet(GlyphUnicode)

	cases := []struct {
		name string
		node NodeEntry
	}{
		{"trusted_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "trusted"}},
		{"untrusted_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "untrusted"}},
		{"warning_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "warning"}},
		{"unknown_in_dir_named", NodeEntry{SourceHash: srcHex, DisplayName: "Alice", TrustLevel: "unknown"}},
		{"trusted_empty_name", NodeEntry{SourceHash: srcHex, DisplayName: "", TrustLevel: "trusted"}},
		{"unknown_not_in_dir", NodeEntry{SourceHash: srcHex, DisplayName: "", TrustLevel: "unknown"}},
		{"trusted_not_in_dir", NodeEntry{SourceHash: srcHex, DisplayName: "", TrustLevel: "trusted"}},
		// NodeEntry always calls simplest_display_str with san=False, so the
		// app sanitize_names config is ignored — modifiers are stripped, not
		// sanitized. "Hello > world" survives strip_modifiers unchanged. Go
		// has no sanitize knob (it always strips modifiers), so both cases
		// reproduce the same row, matching Python for both.
		{"trusted_sanitize_on", NodeEntry{SourceHash: srcHex, DisplayName: "Hello > world", TrustLevel: "trusted"}},
		{"trusted_sanitize_off", NodeEntry{SourceHash: srcHex, DisplayName: "Hello > world", TrustLevel: "trusted"}},
	}

	type goldenEntry struct {
		Name string `json:"name"`
		Row  string `json:"row"`
	}
	var entries []goldenEntry
	runPythonNomadnetScript(t, "../tooling/tui-parity/node_entry_golden.py", &entries)
	want := make(map[string]string, len(entries))
	for _, e := range entries {
		want[e.Name] = e.Row
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, ok := want[tc.name]
			if !ok {
				t.Fatalf("no golden entry named %q in script output", tc.name)
			}
			got := FormatNodeEntryRow(tc.node, g)
			if got != w {
				t.Errorf("row = %q\nwant  %q (Python)", got, w)
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
		t.Run(tc.trust, func(t *testing.T) {
			t.Parallel()
			style, focus := NodeTrustStyle(tc.trust)
			if style != tc.style || focus != tc.focusStyle {
				t.Errorf("(%q) = (%q,%q), want (%q,%q)", tc.trust, style, focus, tc.style, tc.focusStyle)
			}
		})
	}
}
