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
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/parseurl_parity.json
var parseURLParityJSON string

// TestParseURLPythonParity verifies that ParseURL matches Python's
// Browser.parse_url for a battery of URL forms captured from the Python
// reference implementation. The golden master (testdata/parseurl_parity.json)
// was produced by running Python's real parse_url logic with RNS mocked so
// that RNS.Reticulum.TRUNCATED_HASHLENGTH == 128 (the real constant).
//
// Python's parse_url returns the destination hash as bytes; here we compare
// hex strings. parse_url does NOT handle the backtick query part — that lives
// in retrieve_url — so the golden contains no backtick cases; the query
// parsing is exercised by TestParseURLWithQuery.
func TestParseURLPythonParity(t *testing.T) {
	t.Parallel()

	var golden struct {
		DefaultPath         string `json:"default_path"`
		TruncatedHashHexLen int    `json:"truncated_hash_hex_len"`
		Cases               []struct {
			URL         string  `json:"url"`
			CurrentHash *string `json:"current_hash"`
			Label       string  `json:"label"`
			DestHash    *string `json:"dest_hash"`
			Path        *string `json:"path"`
			Error       *string `json:"error"`
		} `json:"cases"`
	}
	if err := json.Unmarshal([]byte(parseURLParityJSON), &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	if golden.TruncatedHashHexLen != hashLen {
		t.Fatalf("golden truncated_hash_hex_len = %v, but Go hashLen = %v", golden.TruncatedHashHexLen, hashLen)
	}

	for _, tc := range golden.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			t.Parallel()

			currentHash := ""
			if tc.CurrentHash != nil {
				currentHash = *tc.CurrentHash
			}
			hash, path, err := ParseURL(tc.URL, currentHash)

			wantErr := tc.Error != nil
			if wantErr {
				if err == nil {
					t.Fatalf("ParseURL(%q) want error, got nil (hash=%q path=%q)", tc.URL, hash, path)
				}
				if tc.DestHash != nil || tc.Path != nil {
					t.Fatalf("golden for %q has error %q but also a hash/path; inconsistent golden", tc.URL, *tc.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseURL(%q) want success, got error: %v", tc.URL, err)
			}

			if tc.DestHash == nil || tc.Path == nil {
				t.Fatalf("golden for %q is a success case but missing dest_hash/path", tc.URL)
			}
			if hash != *tc.DestHash {
				t.Errorf("hash = %q, want %q", hash, *tc.DestHash)
			}
			if path != *tc.Path {
				t.Errorf("path = %q, want %q", path, *tc.Path)
			}
		})
	}
}
