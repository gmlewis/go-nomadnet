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

// TestParseRRCLinkPythonParity and TestValidateLXMFLinkPythonParity are LIVE
// cross-implementation checks: they exec the VALIDATION prologues of Python's
// Browser.handle_rrc_link and Browser.handle_lxmf_link (nomadnet.ui.textui
// .Browser), extracted fresh from the current source via AST, and derive the
// expected (hub, dest, room, error) / (error) freshly on every run. Only the
// pure parse/validate prologue (before the self.app dispatch) is exercised, so
// no app/urwid instance is required. Go owns the input battery; Python owns
// the reference behavior. Tests SKIP, not fail, when the Python reference is
// not importable.
//
// Python normalizes the room as room.strip().lstrip("#").strip().lower() and
// treats an empty room / dest as None (Go callers treat "" as absent). Python
// validates the hub hash is exactly TRUNCATED_HASHLENGTH//8 bytes and hex.
func TestParseRRCLinkPythonParity(t *testing.T) {
	t.Parallel()

	const hub = "aabb1122aabb1122aabb1122aabb1122"
	urls := []string{
		hub + "/general",
		hub + ":myhub/general",
		hub,
		"/" + hub + "/random",
		hub + "/#random",
		hub + "/##doubled",
		hub + "/ #spaced ",
		hub + "/Room",
		hub + "/",
		hub + ":dest/",
		hub + ": dest /room",
		hub + ":dest",
		"zzzz1122aabb1122aabb1122aabb1122/general",
		"aabb/general",
		hub + "aa/general",
		"",
		hub + "/room/extra",
	}

	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Browser as B
import RNS
cls = B.Browser
def try_prologue(method_name, stop_targets):
    src = textwrap.dedent(inspect.getsource(getattr(cls, method_name)))
    fn = ast.parse(src).body[0]
    tn = fn.body[0]
    assert isinstance(tn, ast.Try), method_name
    collected = []
    for s in tn.body:
        if isinstance(s, ast.Assign) and len(s.targets) == 1 and isinstance(s.targets[0], ast.Name) and s.targets[0].id in stop_targets:
            break
        collected.append(s)
    mod = ast.Module(body=collected, type_ignores=[]); ast.fix_missing_locations(mod)
    return compile(mod, "<" + method_name + ">", "exec")
rrc_code = try_prologue("handle_rrc_link", {"existing"})
urls = json.load(sys.stdin)
out = []
for u in urls:
    ns = {"RNS": RNS, "__builtins__": __builtins__, "link_target": u}
    try:
        exec(rrc_code, ns)
    except Exception as e:
        out.append({"hub": None, "dest": "", "room": "", "error": str(e)})
        continue
    hh = ns.get("hub_hash")
    out.append({"hub": hh.hex() if isinstance(hh, (bytes, bytearray)) else None,
                "dest": ns.get("dest") or "", "room": ns.get("room_norm") or "",
                "error": None})
json.dump(out, sys.stdout)
`

	var want []rrcLiveWant
	runPythonNomadnet(t, urls, script, &want)

	for i, url := range urls {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			w := want[i]
			hubGot, roomGot, destGot, err := ParseRRCLink(url)

			if w.Error != nil {
				if err == nil {
					t.Fatalf("ParseRRCLink(%q) want error %q, got nil (hub=%q room=%q dest=%q)",
						url, *w.Error, hubGot, roomGot, destGot)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRRCLink(%q) want success, got error: %v", url, err)
			}
			if w.Hub == nil {
				t.Fatalf("Python returned success but no hub for %q", url)
			}
			if hubGot != *w.Hub {
				t.Errorf("hub = %q, want %q (Python)", hubGot, *w.Hub)
			}
			if destGot != w.Dest {
				t.Errorf("dest = %q, want %q (Python)", destGot, w.Dest)
			}
			if roomGot != w.Room {
				t.Errorf("room = %q, want %q (Python)", roomGot, w.Room)
			}
		})
	}
}

// TestValidateLXMFLinkPythonParity verifies ValidateLXMFLink matches Python's
// handle_lxmf_link validation (type str, length == 32 hex chars, decodable
// hex), executed live via the AST-extracted validation prologue.
func TestValidateLXMFLinkPythonParity(t *testing.T) {
	t.Parallel()

	urls := []string{
		"aabb1122aabb1122aabb1122aabb1122",
		"aabb1122",
		"aabb1122aabb1122aabb1122aabb1122aa",
		"zzbb1122aabb1122aabb1122aabb1122",
		"",
		"aabb1122aabb1122aabb1122aabb1122a",
		"aabb1122aabb1122aabb1122aabb112",
	}

	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Browser as B
import RNS
cls = B.Browser
def try_prologue(method_name, stop_targets):
    src = textwrap.dedent(inspect.getsource(getattr(cls, method_name)))
    fn = ast.parse(src).body[0]
    tn = fn.body[0]
    assert isinstance(tn, ast.Try), method_name
    collected = []
    for s in tn.body:
        if isinstance(s, ast.Assign) and len(s.targets) == 1 and isinstance(s.targets[0], ast.Name) and s.targets[0].id in stop_targets:
            break
        collected.append(s)
    mod = ast.Module(body=collected, type_ignores=[]); ast.fix_missing_locations(mod)
    return compile(mod, "<" + method_name + ">", "exec")
lxmf_code = try_prologue("handle_lxmf_link", {"existing_conversations"})
urls = json.load(sys.stdin)
out = []
for u in urls:
    ns = {"RNS": RNS, "__builtins__": __builtins__, "link_target": u}
    try:
        exec(lxmf_code, ns)
    except Exception as e:
        out.append({"error": str(e)})
        continue
    out.append({"error": None})
json.dump(out, sys.stdout)
`

	var want []*lxmfLiveWant
	runPythonNomadnet(t, urls, script, &want)

	for i, url := range urls {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			w := want[i]
			err := ValidateLXMFLink(url)
			pyErr := w != nil && w.Error != nil
			goErr := err != nil
			if goErr != pyErr {
				t.Errorf("ValidateLXMFLink(%q) error = %v, want error=%v (Python: %q)",
					url, err, pyErr, func() string {
						if w != nil && w.Error != nil {
							return *w.Error
						}
						return ""
					}())
			}
		})
	}
}

type rrcLiveWant struct {
	Hub   *string `json:"hub"`
	Dest  string  `json:"dest"`
	Room  string  `json:"room"`
	Error *string `json:"error"`
}

type lxmfLiveWant struct {
	Error *string `json:"error"`
}
