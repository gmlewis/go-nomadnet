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
	"testing"
)

// TestParseURLPythonParity is a LIVE cross-implementation check: it execs
// Python's real Browser.parse_url (nomadnet.ui.textui.Browser) as a standalone
// function with a mock self (providing only destination_hash) and derives the
// expected (destination_hash, path, error) freshly on every run. Go owns the
// input battery; Python owns the reference behavior. The test SKIPs, not
// fails, when the Python reference is not importable.
//
// Python's parse_url returns the destination hash as bytes; here we compare
// hex strings. parse_url does NOT handle the backtick query part — that lives
// in retrieve_url — so the battery contains no backtick cases; query parsing
// is exercised by TestParseURLWithQuery.
func TestParseURLPythonParity(t *testing.T) {
	t.Parallel()

	const h32 = "a1b2c3d4e5f6a1b2c3d4a1b2c3d4e5f6"
	const alt = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		label       string
		url         string
		currentHash string // empty == None
	}{
		{"bare_32hex_hash", h32, ""},
		{"hash_with_path", h32 + ":/page/about.mu", ""},
		{"hash_empty_path", h32 + ":", ""},
		{"hash_file_path", h32 + ":/file/foo.bin", ""},
		{"relative_with_current", ":/page/foo.mu", h32},
		{"relative_empty_path", ":", h32},
		{"empty_url", "", ""},
		{"short_hash", "tooShort", ""},
		{"hash_too_long_33", h32 + "0", ""},
		{"hash_too_short_30", "a1b2c3d4e5f6a1b2c3d4a1b2c3d4e5", ""},
		{"invalid_hex_32", "zzb2c3d4e5f6a1b2c3d4a1b2c3d4e5f6", ""},
		{"too_many_colons", h32 + ":a:b", ""},
		{"relative_no_current", ":/page/x.mu", ""},
		{"nonhex_len32", "g1b2c3d4e5f6a1b2c3d4a1b2c3d4e5f6", ""},
		{"bare_alt_hash", alt, ""},
		{"alt_hash_default_path", alt + ":/page/index.mu", ""},
		{"path_contains_colon_is_too_many", h32 + ":/p:age", ""},
	}

	type urlInput struct {
		URL         string `json:"url"`
		CurrentHash string `json:"current_hash"` // "" -> None
	}
	inputs := make([]urlInput, len(cases))
	for i, c := range cases {
		inputs[i] = urlInput{URL: c.url, CurrentHash: c.currentHash}
	}

	// Exec the real Browser.parse_url as a standalone function with a mock
	// self whose destination_hash is the current hash (bytes or None). The
	// method body only reads self.destination_hash, RNS.Reticulum, and
	// Browser.DEFAULT_PATH, so a minimal mock suffices and the live source is
	// exercised directly (no transcription).
	const script = `
import sys, json, inspect, textwrap
import nomadnet.ui.textui.Browser as B
import RNS
cls = B.Browser
src = textwrap.dedent(inspect.getsource(cls.parse_url))
g = {"RNS": RNS, "Browser": cls, "__builtins__": __builtins__}
exec(src, g)
fn = g["parse_url"]
class Mock: pass
inputs = json.load(sys.stdin)
out = []
for inp in inputs:
    m = Mock()
    m.destination_hash = bytes.fromhex(inp["current_hash"]) if inp["current_hash"] else None
    err = None; dh = None; path = None
    try:
        dh, path = fn(m, inp["url"])
    except ValueError as e:
        err = str(e)
    out.append({"dest_hash": dh.hex() if isinstance(dh, (bytes, bytearray)) else None,
                "path": path, "error": err})
json.dump(out, sys.stdout)
`

	var want []parseURLLiveWant
	runPythonNomadnet(t, inputs, script, &want)

	for i, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			t.Parallel()
			w := want[i]
			hash, path, err := ParseURL(c.url, c.currentHash)

			wantErr := w.Error != nil
			if wantErr {
				if err == nil {
					t.Fatalf("ParseURL(%q, %q) want error %q, got nil (hash=%q path=%q)",
						c.url, c.currentHash, *w.Error, hash, path)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseURL(%q, %q) want success, got error: %v", c.url, c.currentHash, err)
			}
			if w.DestHash == nil {
				t.Fatalf("Python returned success but no dest_hash for %q", c.url)
			}
			if hash != *w.DestHash {
				t.Errorf("hash = %q, want %q (Python)", hash, *w.DestHash)
			}
			if path != w.Path {
				t.Errorf("path = %q, want %q (Python)", path, w.Path)
			}
		})
	}
}

type parseURLLiveWant struct {
	DestHash *string `json:"dest_hash"`
	Path     string  `json:"path"`
	Error    *string `json:"error"`
}
