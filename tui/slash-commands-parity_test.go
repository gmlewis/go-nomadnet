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

// TestParseSlashCommandPythonParity is a LIVE cross-implementation check of
// Go's ParseSlashCommand against the PARSE prologue of Python's
// RoomWidget._handle_slash_command (Channels.py), executed fresh on every run.
//
// The Python parse prologue is extracted from the CURRENT installed source via
// AST (the `parts`, `cmd`, `arg` assignments and the `not parts or not
// parts[0]` empty-test), so the expected (raw_cmd, arg, is_empty) values are
// derived from live Python code, not a stale transcription. The test SKIPs,
// not fails, when the Python reference is not importable.
//
// Go's ParseSlashCommand intentionally DIVERGES from Python's parse in one
// respect: it canonicalizes command aliases in the parser (returning canonical
// command names via CommandAlias, e.g. /j -> "join", /leave -> "part") and
// adds a /q -> "quit" convenience that Python does not recognize. Python keeps
// the raw lowered command word in the parse and treats aliases only in its
// dispatch branches. This test therefore compares the PARSE-equivalent fields
// — the argument, the isCmd flag, and the raw (pre-alias) command word —
// against Python, and separately verifies that Go's returned command is the
// CommandAlias canonicalization of Python's raw word (documenting, not
// asserting as Python parity, Go's alias layer). The Go behavioral test
// TestCommandAlias in slash-commands_test.go pins the alias set (incl. /q).
func TestParseSlashCommandPythonParity(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"/JOIN #general",
		"/Join #General",
		"/HELP",
		"/Me dances",
		"/join\t#general",
		"/nick   Alice  ",
		"/JOIN",
		"/J #room",
		"/Leave #room",
		"/",
		"/   ",
	}

	// The script discovers the class owning _handle_slash_command at runtime
	// (currently RoomWidget) so a class rename does not silently break it, then
	// extracts the parse prologue expressions from the live source via AST and
	// re-executes them per input with `text` bound.
	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Channels as C
cls = None
for name in dir(C):
    obj = getattr(C, name, None)
    if isinstance(obj, type) and hasattr(obj, "_handle_slash_command"):
        cls = obj
        break
if cls is None:
    raise RuntimeError("no class with _handle_slash_command in nomadnet Channels")
src = textwrap.dedent(inspect.getsource(cls._handle_slash_command))
fn = ast.parse(src).body[0]
exprs = {}
empty_test_src = None
for stmt in ast.walk(fn):
    if isinstance(stmt, ast.Assign) and len(stmt.targets) == 1 and isinstance(stmt.targets[0], ast.Name) and stmt.targets[0].id in ("parts", "cmd", "arg"):
        exprs[stmt.targets[0].id] = ast.get_source_segment(src, stmt.value)
    if isinstance(stmt, ast.If) and empty_test_src is None:
        seg = ast.get_source_segment(src, stmt.test)
        if seg and "not parts" in seg:
            empty_test_src = seg
if not all(k in exprs for k in ("parts", "cmd", "arg")) or not empty_test_src:
    raise RuntimeError("could not extract parse prologue: %r / %r" % (exprs, empty_test_src))
inputs = json.load(sys.stdin)
out = []
for inp in inputs:
    env = {"text": inp}
    env["parts"] = eval(exprs["parts"], env)
    is_empty = eval(empty_test_src, env)
    raw_cmd = ""
    arg = ""
    if not is_empty:
        env["cmd"] = eval(exprs["cmd"], env)
        env["arg"] = eval(exprs["arg"], env)
        raw_cmd = env["cmd"]
        arg = env["arg"]
    out.append({"raw_cmd": raw_cmd, "arg": arg, "is_empty": is_empty})
json.dump(out, sys.stdout)
`

	var want []slashParseLiveWant
	runPythonNomadnet(t, inputs, script, &want)

	for i, inp := range inputs {
		t.Run(inp, func(t *testing.T) {
			t.Parallel()
			cmd, arg, isCmd := ParseSlashCommand(inp)
			w := want[i]

			// Parse parity: argument and isCmd.
			if arg != w.Arg {
				t.Errorf("arg = %q, want %q (Python parse)", arg, w.Arg)
			}
			if isCmd == w.IsEmpty {
				t.Errorf("isCmd = %v, want %v (Python parse: not is_empty=%v)",
					isCmd, !w.IsEmpty, !w.IsEmpty)
			}

			// Command word: Go returns the CommandAlias canonicalization of
			// Python's raw lowered word. For a non-alias word they must be
			// equal; for an alias word Go must return CommandAlias[raw].
			if w.RawCmd == "" {
				if cmd != "" {
					t.Errorf("cmd = %q, want %q (Python parse: empty)", cmd, "")
				}
				return
			}
			if alias, ok := CommandAlias[w.RawCmd]; ok {
				if cmd != alias {
					t.Errorf("cmd = %q, want %q (CommandAlias[%q])", cmd, alias, w.RawCmd)
				}
			} else if cmd != w.RawCmd {
				t.Errorf("cmd = %q, want %q (Python raw command word, no alias)", cmd, w.RawCmd)
			}
		})
	}
}

type slashParseLiveWant struct {
	RawCmd  string `json:"raw_cmd"`
	Arg     string `json:"arg"`
	IsEmpty bool   `json:"is_empty"`
}
