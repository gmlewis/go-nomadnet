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
	"reflect"
	"strings"
	"testing"
)

// TestSplitDialogPythonParity is a LIVE cross-implementation check: it
// AST-extracts the pure prologue of Python's _open_split_dialog
// (nomadnet.ui.textui.Channels) — the body-byte count, _split_message parts,
// part count K, and the part-1 preview (truncated to 70 code points with a
// U+2026 ellipsis, then newlines/tabs replaced by spaces) — execs it with a
// mock self whose _local_message captures the "too small to split" error, and
// derives the expected fields freshly on every run. The noun is "message" +
// ("s" if K != 1 else ""), matching Python's dialog assembly. Go owns the
// input battery; Python owns the reference behavior. The test SKIPs, not
// fails, when the Python reference is not importable.
func TestSplitDialogPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		limit int
	}{
		{"three parts ascii", "hello world this is a long message", 20},
		{"too small to split", "hello world", 5},
		{"preview truncation 100 x", strings.Repeat("x", 100), 200},
		{"multiline preview", "line1\nline2\ttab here", 200},
		{"exactly 70 chars truncates", strings.Repeat("y", 70), 200},
		{"71 chars truncates same", strings.Repeat("y", 71), 200},
		{"three parts multibyte", "café résumé nomadnet test message", 18},
		{"empty text", "", 200},
		{"limit equal to prefix bytes", "abc", 6},
	}

	type splitInput struct {
		Text  string `json:"text"`
		Limit int    `json:"limit"`
	}
	inputs := make([]splitInput, len(tests))
	for i, tt := range tests {
		inputs[i] = splitInput{Text: tt.text, Limit: tt.limit}
	}

	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Channels as C
src = textwrap.dedent(inspect.getsource(C.RoomWidget._open_split_dialog))
fn = ast.parse(src).body[0]
prologue = []
for s in fn.body:
    if isinstance(s, ast.Assign) and len(s.targets) == 1 and isinstance(s.targets[0], ast.Name) and s.targets[0].id == "error_text":
        break
    prologue.append(s)
# Expose the prologue locals onto self so we can read them after the call.
expose = ast.parse("self._parts = parts\nself._K = K\nself._preview = preview\n").body
tmpl = ast.parse("def _prologue(self, text, limit):\n    pass").body[0]
tmpl.body = prologue + expose
mod = ast.Module(body=[tmpl], type_ignores=[]); ast.fix_missing_locations(mod)
g = {"__builtins__": __builtins__, "_split_message": C._split_message}
exec(compile(mod, "<prologue>", "exec"), g)
_prologue = g["_prologue"]

cases = json.load(sys.stdin)
out = []
for c in cases:
    class S:
        def _local_message(self, kind, msg): self.err = msg
    s = S(); s.err = None
    _prologue(s, c["text"], c["limit"])
    b = len(c["text"].encode("utf-8"))
    if s.err is not None:
        out.append({"bytes": b, "error": s.err, "k": 0, "noun": "", "preview": "", "nparts": 0})
        continue
    K = s._K
    noun = "message" + ("" if K == 1 else "s")
    out.append({"bytes": b, "error": "", "k": K, "noun": noun, "preview": s._preview, "nparts": len(s._parts)})
json.dump(out, sys.stdout)
`

	var want []splitLiveWant
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info := ComputeSplitDialog(tt.text, tt.limit)
			w := want[i]
			if info.BodyBytes != w.Bytes {
				t.Errorf("BodyBytes = %v, want %v (Python)", info.BodyBytes, w.Bytes)
			}
			if info.Error != w.Error {
				t.Errorf("Error = %q, want %q (Python)", info.Error, w.Error)
			}
			if w.Error != "" {
				return
			}
			if info.K != w.K {
				t.Errorf("K = %v, want %v (Python)", info.K, w.K)
			}
			if info.Noun != w.Noun {
				t.Errorf("Noun = %q, want %q (Python)", info.Noun, w.Noun)
			}
			if info.Preview != w.Preview {
				t.Errorf("Preview = %q, want %q (Python)", info.Preview, w.Preview)
			}
			if len(info.Parts) != w.NParts {
				t.Errorf("len(Parts) = %v, want %v (Python)", len(info.Parts), w.NParts)
			}
		})
	}
}

type splitLiveWant struct {
	Bytes   int    `json:"bytes"`
	Error   string `json:"error"`
	K       int    `json:"k"`
	Noun    string `json:"noun"`
	Preview string `json:"preview"`
	NParts  int    `json:"nparts"`
}

// TestSplitDialogLines verifies the literal dialog body lines built from a
// SplitDialogInfo, matching the urwid.Text strings Python assembles in
// _open_split_dialog (Channels.py:920-925).
func TestSplitDialogLines(t *testing.T) {
	t.Parallel()

	info := ComputeSplitDialog("hello world this is a long message", 20)
	lines := SplitDialogLines(info)
	want := []string{
		"  Message is 34 bytes.",
		"  Hub limit  : 20 bytes per message.",
		"  Split into 3 messages.",
		"  Preview of part 1:",
		"    (1/3) hello world",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("SplitDialogLines = %v, want %v", lines, want)
	}

	infoSingular := ComputeSplitDialog("short message fits", 200)
	wantSingular := []string{
		"  Message is 18 bytes.",
		"  Hub limit  : 200 bytes per message.",
		"  Split into 1 message.",
		"  Preview of part 1:",
		"    (1/1) short message fits",
	}
	if got := SplitDialogLines(infoSingular); !reflect.DeepEqual(got, wantSingular) {
		t.Errorf("SplitDialogLines singular = %v, want %v", got, wantSingular)
	}
}
