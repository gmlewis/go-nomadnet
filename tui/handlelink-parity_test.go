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
	"strings"
	"testing"
)

// TestParseLinkTargetPythonParity is a LIVE cross-implementation check: it
// execs Python's real Browser.handle_link RESOLUTION (the destination_type +
// resolved target / partial_ids produced before any side-effecting dispatch),
// extracted fresh from the current source via AST, and derives the expected
// classification freshly on every run. Go owns the input battery; Python owns
// the reference behavior. The test SKIPs, not fails, when the Python reference
// is not importable.
//
// The anchor ("#...") and rrc:// branches are classified from the live prefix
// constants (extracted from the source); the typed/partial/bare classification
// is the AST-extracted components block executed with the real
// expand_shorthands (exec'd standalone). Python uses an unbounded
// link_target.split("@") and only treats the link as typed when exactly one
// "@" is present (len(components) == 2); a target with two or more "@" falls
// through to the bare-address branch (nomadnetwork.node, first component). Go
// must match this — not split on only the first "@".
func TestParseLinkTargetPythonParity(t *testing.T) {
	t.Parallel()

	const h32 = "a1b2c3d4e5f6a1b2c3d4a1b2c3d4e5f6"
	urls := []string{
		"#section1",
		"#",
		"rrc://abcdef0123456789/roomname",
		"rrc://",
		"nnn@" + h32,
		"lxmf@" + h32,
		"rrc@" + h32,
		"nomadnetwork.node@" + h32,
		"lxmf.delivery@" + h32,
		"rrc.hub.session@" + h32,
		"custom.type@" + h32,
		h32,
		"somenonhexname",
		"p:id1",
		"p:id1:id2:id3",
		"p:",
		"p:id1|id2",
		"a@b@c",
		"unknown@target",
		"@",
		"nnn@",
		"",
	}

	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Browser as B
import RNS
cls = B.Browser
src = textwrap.dedent(inspect.getsource(cls.handle_link))
fn = ast.parse(src).body[0]
# Extract the anchor/rrc prefix constants (first two Ifs with a startswith test).
prefixes = []
for s in fn.body:
    if isinstance(s, ast.If) and isinstance(s.test, ast.Call) and isinstance(s.test.func, ast.Attribute) and s.test.func.attr == "startswith" and s.test.args and isinstance(s.test.args[0], ast.Constant):
        prefixes.append(s.test.args[0].value)
anchor_pf, rrc_pf = prefixes[0], prefixes[1]
# Collect the components/classification block: from the 'components = ...'
# assignment up to (not including) the dispatch If on destination_type.
comp_stmts = []; collecting = False
for s in fn.body:
    if isinstance(s, ast.Assign) and len(s.targets) == 1 and isinstance(s.targets[0], ast.Name) and s.targets[0].id == "components":
        collecting = True
    if collecting and isinstance(s, ast.If) and isinstance(s.test, ast.Compare) and isinstance(s.test.left, ast.Name) and s.test.left.id == "destination_type":
        break
    if collecting:
        comp_stmts.append(s)
mod = ast.Module(body=comp_stmts, type_ignores=[]); ast.fix_missing_locations(mod)
comp_code = compile(mod, "<handle_link_comp>", "exec")
# Exec expand_shorthands as a standalone function (it only reads self/destination_type).
es_src = textwrap.dedent(inspect.getsource(cls.expand_shorthands))
g = {"__builtins__": __builtins__}; exec(es_src, g); expand = g["expand_shorthands"]
class Mock: pass
def classify(url):
    if url.startswith(anchor_pf):
        return {"kind": "anchor", "destination_type": None, "target": url[len(anchor_pf):], "partial_ids": None}
    if url.startswith(rrc_pf):
        return {"kind": "rrc", "destination_type": "rrc.hub.session", "target": url[len(rrc_pf):], "partial_ids": None}
    m = Mock(); m.expand_shorthands = lambda dt: expand(m, dt)
    ns = {"__builtins__": __builtins__, "self": m, "link_target": url}
    try:
        exec(comp_code, ns)
    except Exception as e:
        return {"kind": "typed", "destination_type": None, "target": None, "partial_ids": None, "error": str(e)}
    pid = ns.get("partial_ids")
    return {"kind": "typed", "destination_type": ns.get("destination_type"),
            "target": ns.get("link_target"), "partial_ids": list(pid) if pid else None}
urls = json.load(sys.stdin)
json.dump([classify(u) for u in urls], sys.stdout)
`

	var want []linkTargetLiveWant
	runPythonNomadnet(t, urls, script, &want)

	for i, url := range urls {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			w := want[i]
			gotType, gotHash, _, _ := ParseLinkTargetWithFields(url)

			switch w.Kind {
			case "anchor":
				if gotType != "anchor" {
					t.Errorf("type = %q, want %q (Python)", gotType, "anchor")
				}
				if w.Target == nil {
					t.Fatal("anchor case missing target")
				}
				if gotHash != *w.Target {
					t.Errorf("hash = %q, want %q (Python)", gotHash, *w.Target)
				}

			case "rrc":
				// Python routes rrc:// straight to handle_rrc_link(target[6:]).
				// Go signals this with destType "rrc" and hash = target[6:].
				if gotType != "rrc" {
					t.Errorf("type = %q, want %q (Python)", gotType, "rrc")
				}
				if w.Target == nil {
					t.Fatal("rrc case missing target")
				}
				if gotHash != *w.Target {
					t.Errorf("hash = %q, want %q (Python)", gotHash, *w.Target)
				}

			case "typed":
				if w.DestinationType == nil {
					t.Fatal("typed case missing destination_type")
				}
				wantType := *w.DestinationType
				if gotType != wantType {
					t.Errorf("type = %q, want %q (Python)", gotType, wantType)
				}
				if wantType == "partial" {
					// Go returns the partial spec as the hash (everything after
					// "p:"); Python returns partial_ids as comps[1:]. Splitting
					// Go's hash on ":" reconstructs Python's list.
					gotIDs := strings.Split(gotHash, ":")
					wantIDs := w.PartialIDs
					if wantIDs == nil {
						wantIDs = []string{}
					}
					if !equalStrings(gotIDs, wantIDs) {
						t.Errorf("partial ids = %v, want %v (Python)", gotIDs, wantIDs)
					}
				} else {
					if w.Target == nil {
						t.Fatal("typed case missing target")
					}
					if gotHash != *w.Target {
						t.Errorf("hash = %q, want %q (Python)", gotHash, *w.Target)
					}
				}

			default:
				t.Fatalf("unknown kind %q from Python", w.Kind)
			}
		})
	}
}

type linkTargetLiveWant struct {
	Kind            string   `json:"kind"`
	DestinationType *string  `json:"destination_type"`
	Target          *string  `json:"target"`
	PartialIDs      []string `json:"partial_ids"`
}

// TestHandleLinkDispatchRouting verifies that BrowserDisplay.HandleLink routes
// each link target to the correct On* callback. This is a GO BEHAVIORAL unit
// test of Go's dispatch architecture (OnJumpAnchor/OnOpenRRC/OnOpenLXMF/
// OnRetrieveURL/OnPartialUpdate/OnBrowserError), NOT a Python parity check:
// Python's handle_link dispatches by calling self methods directly
// (self.handle_rrc_link, self.retrieve_url, self.handle_lxmf_link, ...) with no
// callback indirection, so there is no 1:1 Python equivalent to assert against.
// The resolution parity (which handler a target maps to) is covered by
// TestParseLinkTargetPythonParity above.
func TestHandleLinkDispatchRouting(t *testing.T) {
	t.Parallel()

	const h32 = "aabb1122aabb1122aabb1122aabb1122"

	tests := []struct {
		name        string
		link        string
		wantAnchor  string // set to expect OnJumpAnchor
		wantRRC     bool   // expect OnOpenRRC
		wantLXMF    string // expect OnOpenLXMF
		wantNode    string // expect OnRetrieveURL
		wantPartial []string
		wantError   bool
	}{
		{"anchor", "#intro", "intro", false, "", "", nil, false},
		{"empty anchor", "#", "", false, "", "", nil, false},
		{"rrc scheme", "rrc://" + h32 + "/general", "", true, "", "", nil, false},
		{"nnn@ node", "nnn@" + h32, "", false, "", h32, nil, false},
		{"lxmf@ delivery", "lxmf@" + h32, "", false, h32, "", nil, false},
		{"rrc@ hub", "rrc@" + h32 + "/general", "", true, "", "", nil, false},
		{"plain bare address", h32, "", false, "", h32, nil, false},
		{"non-hex bare address", "somenode", "", false, "", "somenode", nil, false},
		{"partial", "p:sidebar:header", "", false, "", "", []string{"sidebar", "header"}, false},
		// Multiple "@" must fall through to the bare-address branch in Python
		// (nomadnetwork.node, first component), not be treated as typed.
		{"multi at is bare node", "a@b@c", "", false, "", "a", nil, false},
		{"unknown typed -> error", "unknowntype@abc", "", false, "", "", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			bd := NewBrowserDisplay(app)

			var gotAnchor, gotLXMF, gotNode string
			var gotRRC bool
			var gotPartial []string
			var gotError string
			gotAnchorFired := false
			gotNodeFired := false

			bd.OnJumpAnchor = func(name string) { gotAnchor = name; gotAnchorFired = true }
			bd.OnOpenRRC = func(hub, room string) { gotRRC = true }
			bd.OnOpenLXMF = func(hash string) { gotLXMF = hash }
			bd.OnRetrieveURL = func(url string, requestData map[string]string) { gotNode = url; gotNodeFired = true }
			bd.OnPartialUpdate = func(ids []string) { gotPartial = ids }
			bd.OnBrowserError = func(msg string) { gotError = msg }

			bd.HandleLink(tt.link, "")

			if tt.wantAnchor != "" || (tt.name == "empty anchor") {
				if !gotAnchorFired {
					t.Error("expected OnJumpAnchor to fire")
				}
				if gotAnchor != tt.wantAnchor {
					t.Errorf("anchor = %q, want %q", gotAnchor, tt.wantAnchor)
				}
			}
			if tt.wantRRC && !gotRRC {
				t.Error("expected OnOpenRRC to fire")
			}
			if tt.wantLXMF != "" && gotLXMF != tt.wantLXMF {
				t.Errorf("lxmf = %q, want %q", gotLXMF, tt.wantLXMF)
			}
			if tt.wantNode != "" {
				if !gotNodeFired {
					t.Error("expected OnRetrieveURL to fire")
				}
				if gotNode != tt.wantNode {
					t.Errorf("node = %q, want %q", gotNode, tt.wantNode)
				}
			}
			if tt.wantPartial != nil && !equalStrings(gotPartial, tt.wantPartial) {
				t.Errorf("partial = %v, want %v", gotPartial, tt.wantPartial)
			}
			if tt.wantError && gotError == "" {
				t.Error("expected OnBrowserError to fire")
			}
			if !tt.wantError && gotError != "" {
				t.Errorf("unexpected error: %q", gotError)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
