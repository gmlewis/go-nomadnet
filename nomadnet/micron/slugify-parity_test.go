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

package micron

import (
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// slugifyInputs is the input battery for the live cross-implementation parity
// check of Slugify. The Go test owns these inputs; the expected outputs are
// derived FRESH on every run by executing the real Python
// nomadnet.ui.textui.MicronParser.slugify_micron reference (see
// slugifyPythonOnce). The battery covers empty input, plain text, mixed case,
// micron formatting directives (colors, links, headings, partials, alignment,
// emphasis), unicode, whitespace, newlines, tabs, emoji, and punctuation — the
// cases that exercise the format-stripping and category-based slugification Go
// must replicate.
var slugifyInputs = []string{
	"",
	"Hello World",
	"This is a Test",
	"Special!@#Chars",
	"`!bold` text",
	"Mixed CASE",
	"`F00acolor text",
	"`FT0000FF`BT123456",
	"`:link_target",
	"`<heading`>",
	"`{image}",
	"`}braceclose",
	"`r`c`l aligned",
	"café ünïcode",
	"  leading  spaces  ",
	"trailing---dashes---",
	"ALLCAPS",
	"snake_case_var",
	"multiple   spaces",
	"tab\there",
	"newline\nhere",
	"emoji😎text",
	"100% pure",
	"a/b\\c",
	"kebab-case-already",
	"`_underline`_",
	"`=equals`=",
	"MiXeD CaSe HeRe",
	"`f`b",
	"before`F000after",
	"`*`!`_`=",
	"`<heading`>",
	"`{image_data}",
	"`r`c`l",
	"price $5",
	"2+2=4",
	"(bracket)",
	"'quote'",
	"你好",
	"\u202emirror\u202c",
	"\uFEFFBOM",
	" nbsp",
	"mix😀😎text",
}

// slugifyParityScript imports the real nomadnet.ui.textui.MicronParser
// reference and applies slugify_micron to each input supplied as JSON on stdin,
// emitting the fresh outputs as JSON on stdout.
const slugifyParityScript = `
import sys, json
import nomadnet.ui.textui.MicronParser as M
inputs = json.loads(sys.stdin.read() or "[]")
out = [M.slugify_micron(s) for s in inputs]
print(json.dumps(out, ensure_ascii=False))
`

// slugifyPythonOnce caches the single live Python run that derives fresh
// expected slugify outputs, so the test shares one python3 exec.
var (
	slugifyPythonOnce sync.Once
	slugifyPythonOut  []any
)

func slugifyPython(t *testing.T) []any {
	t.Helper()
	slugifyPythonOnce.Do(func() {
		testutils.RunPythonNomadnet(t, slugifyInputs, slugifyParityScript, &slugifyPythonOut)
	})
	return slugifyPythonOut
}

func TestSlugifyPythonParity(t *testing.T) {
	t.Parallel()
	want := slugifyPython(t)
	for i, in := range slugifyInputs {
		wv, _ := want[i].(string)
		if got := Slugify(in); got != wv {
			t.Errorf("case %v (%q): got %q, want %q", i, in, got, wv)
		}
	}
}
