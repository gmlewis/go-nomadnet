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

package util

import (
	"embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/util_parity.json
var parityFS embed.FS

// parityCase is a single [input, expected-output] pair captured from the
// Python reference implementation in markqvist/NomadNet (nomadnet/util.py).
// expected is nil when the Python function returned None.
type parityCase [2]any

func loadParity(t *testing.T, fn string) []parityCase {
	t.Helper()
	data, err := parityFS.ReadFile("testdata/util_parity.json")
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	var raw map[string][]parityCase
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return raw[fn]
}

// strOrZero returns the string value of v, or "" for nil/missing.
func strOrZero(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func TestStripModifiersPythonParity(t *testing.T) {
	t.Parallel()
	cases := loadParity(t, "strip_modifiers")
	for i, c := range cases {
		in, inOk := strOrZero(c[0])
		want, wantOk := strOrZero(c[1])
		got := StripModifiers(func() *string {
			if !inOk {
				return nil
			}
			return &in
		}())
		_ = i
		if !wantOk {
			if got != nil {
				t.Errorf("case %d (%q): got %q, want nil", i, in, *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("case %d (%q): got nil, want %q", i, in, want)
			continue
		}
		if *got != want {
			t.Errorf("case %d (%q): got %q, want %q", i, in, *got, want)
		}
	}
}

func TestSanitizeNamePythonParity(t *testing.T) {
	t.Parallel()
	cases := loadParity(t, "sanitize_name")
	for i, c := range cases {
		in, inOk := strOrZero(c[0])
		want, wantOk := strOrZero(c[1])
		got := SanitizeName(func() *string {
			if !inOk {
				return nil
			}
			return &in
		}())
		if !wantOk {
			if got != nil {
				t.Errorf("case %d (%q): got %q, want nil", i, in, *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("case %d (%q): got nil, want %q", i, in, want)
			continue
		}
		if *got != want {
			t.Errorf("case %d (%q): got %q, want %q", i, in, *got, want)
		}
	}
}

func TestStripMicronPythonParity(t *testing.T) {
	t.Parallel()
	cases := loadParity(t, "strip_micron")
	for i, c := range cases {
		in, _ := strOrZero(c[0])
		want, _ := strOrZero(c[1])
		if got := StripMicron(in); got != want {
			t.Errorf("case %d (%q): got %q, want %q", i, in, got, want)
		}
	}
}

func TestStripEscapedMicronPythonParity(t *testing.T) {
	t.Parallel()
	cases := loadParity(t, "strip_escaped_micron")
	for i, c := range cases {
		in, _ := strOrZero(c[0])
		want, _ := strOrZero(c[1])
		if got := StripEscapedMicron(in); got != want {
			t.Errorf("case %d (%q): got %q, want %q", i, in, got, want)
		}
	}
}

func TestUnescapeMicronPythonParity(t *testing.T) {
	t.Parallel()
	cases := loadParity(t, "unescape_micron")
	for i, c := range cases {
		in, _ := strOrZero(c[0])
		want, _ := strOrZero(c[1])
		if got := UnescapeMicron(in); got != want {
			t.Errorf("case %d (%q): got %q, want %q", i, in, got, want)
		}
	}
}

func TestStripNonFormattingTagsPythonParity(t *testing.T) {
	t.Parallel()
	cases := loadParity(t, "strip_non_formatting_tags")
	for i, c := range cases {
		in, _ := strOrZero(c[0])
		want, _ := strOrZero(c[1])
		if got := StripNonFormattingTags(in); got != want {
			t.Errorf("case %d (%q): got %q, want %q", i, in, got, want)
		}
	}
}
