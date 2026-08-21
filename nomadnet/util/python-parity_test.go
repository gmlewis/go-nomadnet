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
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// utilInputs is the input battery for the live cross-implementation parity
// check. The Go test owns these inputs; the expected outputs are derived FRESH
// on every run by executing the real Python nomadnet.util reference (see
// utilPythonOnce). The battery covers default, unicode-category, combining-mark,
// variation-selector, ZWJ/ZWNJ, control-character, micron-formatting, and
// edge cases that exercise the category-based filtering Go must replicate.
var utilInputs = map[string][]string{
	"strip_modifiers":           {"", "hello world", "café", "123", "Ⅰ", "①", "☃snowman", "hello\x00world", "hello\x07world", "áe", "äb", "test​‌", "abc‍def", "test️", "test󠄀", "test🏻", "🕵️", "😀emoji😁", "hello\r\nworld", "hello\rworld", "line1\r\nline2\r\nline3", "  spaces  ", "hello  world  test", "🕵️☝", "\t", "\n", "price $5", "2+2=4", "a_b-c.d", "(bracket)", "'quote'", "你好", "ʰmodifier", "café́", "‮mirror‬", "\uFEFFBOM", "⁠wordjoiner", " nbsp", "mix😀😎text", "`F000colored`B123text", "before`F000after", "`FT0000FF`BT123456", "`*`!`_`=", "`f`b", "`<heading`>", "`{image}"},
	"sanitize_name":             {"", "hello world", "café", "123", "Ⅰ", "①", "☃snowman", "hello\x00world", "hello\x07world", "áe", "äb", "test​‌", "abc‍def", "test️", "test󠄀", "test🏻", "🕵️", "😀emoji😁", "hello\r\nworld", "hello\rworld", "line1\r\nline2\r\nline3", "  spaces  ", "hello  world  test", "🕵️☝", "\t", "\n", "price $5", "2+2=4", "a_b-c.d", "(bracket)", "'quote'", "你好", "ʰmodifier", "café́", "‮mirror‬", "\uFEFFBOM", "⁠wordjoiner", " nbsp", "mix😀😎text", "`F000colored`B123text", "before`F000after", "`FT0000FF`BT123456", "`*`!`_`=", "`f`b", "`<heading`>", "`{image}"},
	"strip_micron":              {"hello", "`F000`B123", "`FT0000FF`BT123456", "`*`!`_`=", "`f`b", "`<heading`>", "`{image_data}", "before`F000after", "normal text", "`FAB`BCD", "`r`c`l align", "color`Faaared"},
	"strip_escaped_micron":      {"hello", "¦F000", "¦FT0000FF", "¦*¦!¦_", "¦f¦b", "¦<heading¦>", "¦{image_data}", "¦r¦c¦l", "text¦Fabc more"},
	"unescape_micron":           {"hello", "¦F000", "¦FT0000FF", "¦*¦!¦_", "¦f¦b", "¦<heading¦>", "¦{image_data}", "¦r¦c¦l", "text¦Fabc more"},
	"strip_non_formatting_tags": {"hello", "`<`>`{`r`c`l", "`<heading`>", "all`<`>`{`r`c`lstripped", "`r`c`l align me", "`F000keep`r`c`l"},
}

// utilParityScript imports the real nomadnet.util reference and applies each
// function to the input battery supplied as JSON on stdin, emitting the fresh
// outputs as JSON on stdout. None results become JSON null.
const utilParityScript = `
import sys, json
import nomadnet.util as U
req = json.loads(sys.stdin.read() or "{}")
fns = {
    "strip_modifiers": U.strip_modifiers,
    "sanitize_name": U.sanitize_name,
    "strip_micron": U.strip_micron,
    "strip_escaped_micron": U.strip_escaped_micron,
    "unescape_micron": U.unescape_micron,
    "strip_non_formatting_tags": U.strip_non_formatting_tags,
}
out = {}
for fn, inputs in req.items():
    f = fns[fn]
    out[fn] = [f(s) for s in inputs]
print(json.dumps(out, ensure_ascii=False))
`

// utilPythonOnce caches the single live Python run that derives fresh expected
// outputs for every util parity function, so the six tests below share one
// python3 exec instead of one each.
var (
	utilPythonOnce sync.Once
	utilPythonOut  map[string][]any
)

func utilPython(t *testing.T) map[string][]any {
	t.Helper()
	utilPythonOnce.Do(func() {
		testutils.RunPythonNomadnet(t, utilInputs, utilParityScript, &utilPythonOut)
	})
	return utilPythonOut
}

// pyStr converts a Python output element (string or nil) to (value, ok).
func pyStr(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func TestStripModifiersPythonParity(t *testing.T) {
	t.Parallel()
	want := utilPython(t)["strip_modifiers"]
	for i, in := range utilInputs["strip_modifiers"] {
		got := StripModifiers(&in)
		wv, wok := pyStr(want[i])
		if !wok {
			if got != nil {
				t.Errorf("case %v (%q): got %q, want nil", i, in, *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("case %v (%q): got nil, want %q", i, in, wv)
			continue
		}
		if *got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, *got, wv)
		}
	}
}

func TestSanitizeNamePythonParity(t *testing.T) {
	t.Parallel()
	want := utilPython(t)["sanitize_name"]
	for i, in := range utilInputs["sanitize_name"] {
		got := SanitizeName(&in)
		wv, wok := pyStr(want[i])
		if !wok {
			if got != nil {
				t.Errorf("case %v (%q): got %q, want nil", i, in, *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("case %v (%q): got nil, want %q", i, in, wv)
			continue
		}
		if *got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, *got, wv)
		}
	}
}

func TestStripMicronPythonParity(t *testing.T) {
	t.Parallel()
	want := utilPython(t)["strip_micron"]
	for i, in := range utilInputs["strip_micron"] {
		wv, _ := pyStr(want[i])
		if got := StripMicron(in); got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, got, wv)
		}
	}
}

func TestStripEscapedMicronPythonParity(t *testing.T) {
	t.Parallel()
	want := utilPython(t)["strip_escaped_micron"]
	for i, in := range utilInputs["strip_escaped_micron"] {
		wv, _ := pyStr(want[i])
		if got := StripEscapedMicron(in); got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, got, wv)
		}
	}
}

func TestUnescapeMicronPythonParity(t *testing.T) {
	t.Parallel()
	want := utilPython(t)["unescape_micron"]
	for i, in := range utilInputs["unescape_micron"] {
		wv, _ := pyStr(want[i])
		if got := UnescapeMicron(in); got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, got, wv)
		}
	}
}

func TestStripNonFormattingTagsPythonParity(t *testing.T) {
	t.Parallel()
	want := utilPython(t)["strip_non_formatting_tags"]
	for i, in := range utilInputs["strip_non_formatting_tags"] {
		wv, _ := pyStr(want[i])
		if got := StripNonFormattingTags(in); got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, got, wv)
		}
	}
}
