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

// TestFormatAnnounceStreamRowPythonParity is a LIVE cross-implementation
// check: it runs the stubbed-urwid golden capture script tooling/tui-parity/
// announce_entry_golden.py fresh on every run. That script stubs gi/urwid
// (GLib is broken on this host), instantiates the real Python nomadnet
// AnnounceStreamEntry (Network.py:259-390) with a fake directory at a fixed
// now (unix 1700000000), and prints the row text + style for each named case.
// The Go test reproduces each case through FormatAnnounceStreamRow and
// compares the row text to the freshly captured Python value, matched by case
// name.
//
// Both sides render the timestamp in the machine's LOCAL zone — Go via
// time.Unix(ts,0) (matching Python's datetime.fromtimestamp) — so the
// same-day %H:%M:%S vs other-day %Y-%m-%d choice and the rendered value are
// zone-consistent across implementations. now=2023-11-14 (local); same-day
// announce = 1h prior; other-day announce = 7 days prior (-> "2023-11-07",
// whose date is zone-stable). Source hash is bytes(0..31) so the hex form is
// "000102…1f". The test SKIPs, not fails, when the Python reference is not
// importable or the script file is not accessible.
func TestFormatAnnounceStreamRowPythonParity(t *testing.T) {
	t.Parallel()

	// Local zone, matching Python's datetime.fromtimestamp — no forced UTC.
	now := time.Unix(1700000000, 0)
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
	}{
		{"node_trusted_today", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Alice"}, false, false},
		{"peer_untrusted_today", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "peer", DisplayName: "Bob"}, false, false},
		{"pn_unknown_today", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "pn", DisplayName: "PN-1"}, false, false},
		{"node_warning_otherday", AnnounceEntry{Timestamp: otherDay, SourceHash: srcHex, Type: "node", DisplayName: "Carol"}, false, false},
		{"long_name_truncate", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: longName}, false, false},
		{"show_destination_hexrep", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Alice"}, true, false},
		{"sanitize_on", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Hello > world"}, false, true},
		{"sanitize_off_strip_mods", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: "Hello > world"}, false, false},
		{"empty_name_prettyhex", AnnounceEntry{Timestamp: today, SourceHash: srcHex, Type: "node", DisplayName: ""}, false, false},
	}

	type goldenEntry struct {
		Case string `json:"case"`
		Row  string `json:"row"`
	}
	var entries []goldenEntry
	runPythonNomadnetScript(t, "../tooling/tui-parity/announce_entry_golden.py", &entries)
	want := make(map[string]string, len(entries))
	for _, e := range entries {
		want[e.Case] = e.Row
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, ok := want[tc.name]
			if !ok {
				t.Fatalf("no golden entry named %q in script output", tc.name)
			}
			got := FormatAnnounceStreamRow(tc.ann, now, tc.showDestination, tc.sanitize, g)
			if got != w {
				t.Errorf("row = %q\nwant  %q (Python)", got, w)
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
