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

package browser

import (
	"reflect"
	"testing"
)

// TestParseURL pins the Go port of Python nomadnet Browser.retrieve_url's
// address-parsing logic (Browser.py retrieve_url + parse_url, lines 631-657 +
// 884-945) against golden values captured from the INSTALLED Python 3.14
// nomadnet (TRUNCATED_HASHLENGTH=128 → destination hashes are 32 hex chars /
// 16 bytes). The capture script drove the real Browser.parse_url for the
// (dest_hash, path) part and replicated retrieve_url's backtick-field merge
// (var_<k>=<v>) verbatim.
//
// URL grammar accepted (after handle_link scheme stripping):
//
//	<32hex>                 → hash + /page/index.mu
//	<32hex>:                → hash + /page/index.mu  (empty path ⇒ default)
//	<32hex>:<path>          → hash + path
//	:<path>                 → reuse current dest hash + path (needs current)
//	:                       → reuse current dest hash + /page/index.mu
//	<...>`f1=v1|f2=v2       → merges var_f1=v1, var_f2=v2 into request_data
//	anything else           → ErrMalformedURL
//
// request_data semantics (matching Python `if not request_data: request_data = {}`):
// a nil/empty incoming map is replaced by a fresh map ONLY when a non-empty
// backtick-fields suffix is present; a non-empty incoming map is preserved and
// extended. A non-empty fields suffix with no valid entries yields an empty
// (non-nil) map; an empty fields suffix (or no backtick) yields nil.
func TestParseURL(t *testing.T) {
	const h32 = "abcdef0123456789abcdef0123456789" // 32-hex / 16-byte dest hash
	cur := bytesFromHex(t, h32)

	cases := []struct {
		name        string
		url         string
		currentDest []byte // current destination hash for relative ":<path>" URLs; nil allowed
		inRD        map[string]string
		wantDestHex string // "" when wantErr
		wantPath    string
		wantRD      map[string]string // exact: nil vs empty-vs-populated distinguishes matter
		wantErr     bool
	}{
		{"hash only", h32, cur, nil, h32, "/page/index.mu", nil, false},
		{"hash colon empty path", h32 + ":", cur, nil, h32, "/page/index.mu", nil, false},
		{"hash default path", h32 + ":/page/index.mu", cur, nil, h32, "/page/index.mu", nil, false},
		{"hash custom path", h32 + ":/page/foo.mu", cur, nil, h32, "/page/foo.mu", nil, false},
		{"hash file path", h32 + ":/file/x.tar", cur, nil, h32, "/file/x.tar", nil, false},
		{"relative custom path", ":/page/custom.mu", cur, nil, h32, "/page/custom.mu", nil, false},
		{"relative empty path", ":", cur, nil, h32, "/page/index.mu", nil, false},
		{"short", "short", cur, nil, "", "", nil, true},
		{"too long", h32 + "extra", cur, nil, "", "", nil, true},
		{"nonhex", "xyz", cur, nil, "", "", nil, true},
		{"three colons", h32 + ":/p:with:colons", cur, nil, "", "", nil, true},
		{"empty url", "", cur, nil, "", "", nil, true},
		{"relative no current", ":/page/custom.mu", nil, nil, "", "", nil, true},
		{"relative colon no current", ":", nil, nil, "", "", nil, true},

		{"fields two", h32 + "`a=1|b=2", cur, nil, h32, "/page/index.mu", map[string]string{"var_a": "1", "var_b": "2"}, false},
		{"fields drop noeq", h32 + "`a=1|noeq|b=2", cur, nil, h32, "/page/index.mu", map[string]string{"var_a": "1", "var_b": "2"}, false},
		{"empty fields suffix", h32 + "`", cur, nil, h32, "/page/index.mu", nil, false},
		{"path and fields", h32 + ":/page/x.mu`k=v", cur, nil, h32, "/page/x.mu", map[string]string{"var_k": "v"}, false},
		{"fields all invalid -> empty map", h32 + "`a=1=x", cur, nil, h32, "/page/index.mu", map[string]string{}, false},
		{"two backticks malformed", h32 + "``extra", cur, nil, "", "", nil, true},
		{"fields preserve existing", h32 + "`a=1", cur, map[string]string{"var_existing": "z"}, h32, "/page/index.mu", map[string]string{"var_existing": "z", "var_a": "1"}, false},
		{"fields empty input fresh", h32 + "`a=1", cur, map[string]string{}, h32, "/page/index.mu", map[string]string{"var_a": "1"}, false},
		{"relative with fields", ":/page/x.mu`k=v", cur, nil, h32, "/page/x.mu", map[string]string{"var_k": "v"}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest, path, rd, err := ParseURL(c.url, c.currentDest, c.inRD)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseURL(%q) err = nil, want non-nil", c.url)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseURL(%q) err = %v, want nil", c.url, err)
			}
			gotDestHex := ""
			if dest != nil {
				gotDestHex = hexEncode(dest)
			}
			if gotDestHex != c.wantDestHex {
				t.Errorf("ParseURL(%q) dest = %v, want %v", c.url, gotDestHex, c.wantDestHex)
			}
			if path != c.wantPath {
				t.Errorf("ParseURL(%q) path = %q, want %q", c.url, path, c.wantPath)
			}
			if !reflect.DeepEqual(rd, c.wantRD) {
				t.Errorf("ParseURL(%q) requestData = %v (nil=%v), want %v (nil=%v)",
					c.url, rd, rd == nil, c.wantRD, c.wantRD == nil)
			}
		})
	}
}

// TestCurrentURL pins the canonical URL reconstruction (Python Browser.current_url,
// Browser.py:146-163): "<32hex>:<path>" plus an optional "`k=v|k=v" suffix
// holding only var_* request-data entries.
func TestCurrentURL(t *testing.T) {
	const h32 = "abcdef0123456789abcdef0123456789"
	dh := bytesFromHex(t, h32)
	cases := []struct {
		name string
		path string
		rd   map[string]string
		want string
	}{
		{"no data default", "/page/index.mu", nil, h32 + ":/page/index.mu"},
		{"var and field keys", "/page/x.mu", map[string]string{"var_a": "1", "field_b": "2", "var_c": "3"}, h32 + ":/page/x.mu`a=1|c=3"},
		{"only field keys no suffix", "/page/x.mu", map[string]string{"field_b": "2"}, h32 + ":/page/x.mu"},
		{"empty map no suffix", "/page/x.mu", map[string]string{}, h32 + ":/page/x.mu"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CurrentURL(dh, c.path, c.rd); got != c.want {
				t.Errorf("CurrentURL(%q,%v) = %q, want %q", c.path, c.rd, got, c.want)
			}
		})
	}
}

func bytesFromHex(t *testing.T, s string) []byte {
	t.Helper()
	b := make([]byte, len(s)/2)
	for i := range b {
		var hi, lo byte
		switch {
		case s[2*i] >= '0' && s[2*i] <= '9':
			hi = s[2*i] - '0'
		case s[2*i] >= 'a' && s[2*i] <= 'f':
			hi = s[2*i] - 'a' + 10
		}
		switch {
		case s[2*i+1] >= '0' && s[2*i+1] <= '9':
			lo = s[2*i+1] - '0'
		case s[2*i+1] >= 'a' && s[2*i+1] <= 'f':
			lo = s[2*i+1] - 'a' + 10
		}
		b[i] = hi<<4 | lo
	}
	return b
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[2*i] = hex[v>>4]
		out[2*i+1] = hex[v&0x0f]
	}
	return string(out)
}
