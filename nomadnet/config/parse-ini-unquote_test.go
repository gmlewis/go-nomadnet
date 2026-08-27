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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseINIUnquote pins the quote-stripping behavior of parseINI against
// golden values captured from Python's RNS.vendor.configobj.ConfigObj, which
// nomadnet uses to read its config file. ConfigObj strips one matching pair of
// surrounding single or double quotes from every scalar value (and from each
// element of a comma-separated list), so `node_name = "My Node"` in the config
// file must announce as `My Node`, not `"My Node"`. Golden capture:
//
//	node.node_name = 'Go port of NomadNet'          (from `="Go port of NomadNet"`)
//	node.static_peers = ['00112233', '445566']      (from `="00112233", "445566"`)
//	client.display_name = 'Single Quoted Name'      (from `='Single Quoted Name'`)
//	client.empty_val = ''                           (from `=""`)
func TestParseINIUnquote(t *testing.T) {
	t.Parallel()

	content := `[node]
node_name = "Go port of NomadNet"
static_peers = "00112233", "445566"
[client]
display_name = 'Single Quoted Name'
empty_val = ""
[logging]
loglevel = 6
plain = keep me
inner = say "hello" twice
mixed = don't stop
`

	dir := tempDir(t)
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	raw, err := parseINI(f)
	if err != nil {
		t.Fatalf("parseINI: %v", err)
	}

	tests := []struct {
		section, key, want string
	}{
		{"node", "node_name", "Go port of NomadNet"},
		{"node", "static_peers", `"00112233", "445566"`},
		{"client", "display_name", "Single Quoted Name"},
		{"client", "empty_val", ""},
		{"logging", "loglevel", "6"},
		{"logging", "plain", "keep me"},
		// Quotes that do not wrap the entire value are kept, matching
		// ConfigObj (it only strips one matching surrounding pair).
		{"logging", "inner", `say "hello" twice`},
		{"logging", "mixed", "don't stop"},
	}
	for _, tc := range tests {
		if got := raw[tc.section][tc.key]; got != tc.want {
			t.Errorf("%v.%v = %q, want %q", tc.section, tc.key, got, tc.want)
		}
	}
}

// TestUnquoteValues exercises the value-level unquoting helper directly with
// the same golden table as the ConfigObj capture above.
func TestUnquoteValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{`"Go port of NomadNet"`, "Go port of NomadNet"},
		{`'Single Quoted Name'`, "Single Quoted Name"},
		{`""`, ""},
		{`''`, ""},
		{`"don't stop"`, `don't stop`},
		{`'say "hi"'`, `say "hi"`},
		{`plain`, "plain"},
		{``, ""},
		{`"unclosed`, `"unclosed`},
		{`unclosed"`, `unclosed"`},
		{`"mismatched'`, `"mismatched'`},
		// Surrounding whitespace is stripped before quote handling; spaces
		// INSIDE the quotes are preserved, matching ConfigObj.
		{`  " spaced "  `, " spaced "},
		// A fully-quoted value containing a comma stays quoted here: it is a
		// list value whose single element keeps the comma (see asList).
		{`"abcd,ef12"`, `"abcd,ef12"`},
	}
	for _, tc := range tests {
		if got := unquoteValue(tc.in); got != tc.want {
			t.Errorf("unquoteValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestUnquoteListElements pins element-wise unquoting inside comma-separated
// list values, matching ConfigObj: `static_peers = "00112233", "445566"`
// yields the two unquoted elements ['00112233', '445566'], while a fully
// quoted value with an embedded comma (`"abcd,ef12"`) is the single element
// ['abcd,ef12'].
func TestUnquoteListElements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want []string
	}{
		{`"00112233", "445566"`, []string{"00112233", "445566"}},
		{`abcd,ef12`, []string{"abcd", "ef12"}},
		{`'aa',bb,'cc'`, []string{"aa", "bb", "cc"}},
		{`single`, []string{"single"}},
		{`"abcd,ef12"`, []string{"abcd,ef12"}},
		{`'a, b, c'`, []string{"a, b, c"}},
	}
	for _, tc := range tests {
		got := asList(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("asList(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("asList(%q)[%v] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
