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
	"time"
)

// TestNetworkStatLabelsPythonParity is a LIVE cross-implementation check: it
// AST-extracts the seven NetworkDisplay stat-widget update methods
// (nomadnet.ui.textui.Network: update_time/update_stat for AnnounceTime,
// NodeAnnounceTime, NodeActiveConnections, NodeStorageStats,
// NodeTotalConnections, NodeTotalPages, NodeTotalFiles), execs each with a
// mock app reflecting the Go inputs, and captures the exact "label : value"
// string Python passes to display_widget.set_text — freshly on every run.
//
// pretty_date (the relative-time formatter used by the announce-time labels)
// is AST-patched so datetime.now() becomes an injected NOW and
// datetime.fromtimestamp becomes a UTC version, making the timestamp-based
// labels deterministic and timezone-independent (Go computes its diff in UTC
// via time.Time.Sub, so both sides share one fixed instant). Go owns the input
// battery; Python owns the reference behavior. The test SKIPs, not fails,
// when the Python reference is not importable.
//
// The fixed now is 2026-07-31 12:00:00 UTC. The storage cases exercise the
// pct = round(used/limit*100, 1) computation (including Python's banker's
// rounding, e.g. 2.25 -> 2.2), the no-limit branch (bare prettysize), and the
// propagation-disabled / no-node guards (-> "None").
func TestNetworkStatLabelsPythonParity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	nowUnix := now.Unix()
	sec := func(offset time.Duration) *int64 {
		v := now.Add(-offset).Unix()
		return &v
	}
	iptr := func(v int64) *int64 { return &v }

	type statCase struct {
		MethodIdx int // 0..6
		Name      string
		TS        *int64
		HasNode   bool
		Links     int
		Used      *int64
		Limit     *int64
		PropOn    bool
		N         int
	}
	cases := []statCase{
		// 0 AnnounceTime
		{MethodIdx: 0, Name: "AnnounceTime/never", TS: nil},
		{MethodIdx: 0, Name: "AnnounceTime/a minute ago", TS: sec(90 * time.Second)},
		{MethodIdx: 0, Name: "AnnounceTime/hours ago", TS: sec(5 * time.Hour)},
		{MethodIdx: 0, Name: "AnnounceTime/days ago", TS: sec(48 * time.Hour)},
		// 1 NodeAnnounceTime
		{MethodIdx: 1, Name: "NodeAnnounceTime/never", TS: nil},
		{MethodIdx: 1, Name: "NodeAnnounceTime/a minute ago", TS: sec(90 * time.Second)},
		// 2 NodeActiveConnections
		{MethodIdx: 2, Name: "ActiveConnections/three", HasNode: true, Links: 3},
		{MethodIdx: 2, Name: "ActiveConnections/zero", HasNode: true, Links: 0},
		{MethodIdx: 2, Name: "ActiveConnections/no node", HasNode: false},
		// 3 NodeStorageStats
		{MethodIdx: 3, Name: "Storage/50pct", HasNode: true, Used: iptr(500000), Limit: iptr(1000000), PropOn: true},
		{MethodIdx: 3, Name: "Storage/zero used", HasNode: true, Used: iptr(0), Limit: iptr(1000000), PropOn: true},
		{MethodIdx: 3, Name: "Storage/no limit", HasNode: true, Used: iptr(512), Limit: nil, PropOn: true},
		{MethodIdx: 3, Name: "Storage/bankers pct", HasNode: true, Used: iptr(225), Limit: iptr(10000), PropOn: true},
		{MethodIdx: 3, Name: "Storage/37.5 pct", HasNode: true, Used: iptr(3750), Limit: iptr(10000), PropOn: true},
		{MethodIdx: 3, Name: "Storage/propagation disabled", HasNode: true, Used: iptr(500), Limit: iptr(1000), PropOn: false},
		{MethodIdx: 3, Name: "Storage/no node", HasNode: false, Used: iptr(500), Limit: iptr(1000), PropOn: true},
		{MethodIdx: 3, Name: "Storage/big GB", HasNode: true, Used: iptr(1073741824), Limit: iptr(2147483648), PropOn: true},
		// 4 NodeTotalConnections
		{MethodIdx: 4, Name: "TotalConnections/42", HasNode: true, N: 42},
		{MethodIdx: 4, Name: "TotalConnections/zero", HasNode: true, N: 0},
		{MethodIdx: 4, Name: "TotalConnections/no node", HasNode: false},
		// 5 NodeTotalPages
		{MethodIdx: 5, Name: "TotalPages/7", HasNode: true, N: 7},
		{MethodIdx: 5, Name: "TotalPages/no node", HasNode: false},
		// 6 NodeTotalFiles
		{MethodIdx: 6, Name: "TotalFiles/13", HasNode: true, N: 13},
		{MethodIdx: 6, Name: "TotalFiles/no node", HasNode: false},
	}

	type statInput struct {
		MethodIdx int    `json:"method_idx"`
		TS        *int64 `json:"ts"`
		HasNode   bool   `json:"has_node"`
		Links     int    `json:"links"`
		Used      *int64 `json:"used"`
		Limit     *int64 `json:"limit"`
		PropOn    bool   `json:"prop_on"`
		N         int    `json:"n"`
		NowUnix   int64  `json:"now_unix"`
	}
	inputs := make([]statInput, len(cases))
	for i, c := range cases {
		inputs[i] = statInput{MethodIdx: c.MethodIdx, TS: c.TS, HasNode: c.HasNode,
			Links: c.Links, Used: c.Used, Limit: c.Limit, PropOn: c.PropOn, N: c.N, NowUnix: nowUnix}
	}

	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Network as N
import RNS
from datetime import datetime, timezone

src = inspect.getsource(N)

# Patch pretty_date: now=datetime.now() -> NOW (injected); datetime.fromtimestamp(X) -> UTCFROMTS(X).
pd_src = textwrap.dedent(inspect.getsource(N.pretty_date))
pd_fn = ast.parse(pd_src).body[0]
class T(ast.NodeTransformer):
    def visit_Assign(self, node):
        self.generic_visit(node)
        if len(node.targets) == 1 and isinstance(node.targets[0], ast.Name) and node.targets[0].id == "now":
            node.value = ast.Name(id="NOW", ctx=ast.Load())
        return node
    def visit_Call(self, node):
        self.generic_visit(node)
        if isinstance(node.func, ast.Attribute) and node.func.attr == "fromtimestamp":
            node.func = ast.Name(id="UTCFROMTS", ctx=ast.Load())
        return node
T().visit(pd_fn); ast.fix_missing_locations(pd_fn)
g = {"__builtins__": __builtins__}
exec(compile(ast.Module(body=[pd_fn], type_ignores=[]), "<pd>", "exec"), g)
pretty_date = g["pretty_date"]

# Extract the 7 update_time/update_stat blocks in source order.
tree = ast.parse(src)
methods = [n for n in ast.walk(tree) if isinstance(n, ast.FunctionDef) and n.name in ("update_time", "update_stat")]
methods.sort(key=lambda n: n.lineno)
blocks = []
for fn in methods:
    collected = []
    for s in fn.body:
        collected.append(s)
        if isinstance(s, ast.Expr) and isinstance(s.value, ast.Call) and isinstance(s.value.func, ast.Attribute) and s.value.func.attr == "set_text":
            break
    mod = ast.Module(body=collected, type_ignores=[]); ast.fix_missing_locations(mod)
    blocks.append(compile(mod, "<b>", "exec"))

class DW:
    def __init__(self): self.text = None
    def set_text(self, s): self.text = s
class Node:
    def __init__(self, links): self.destination = type("D", (), {"links": links})()
class Router:
    def __init__(self, limit, used): self.message_storage_limit = limit; self._used = used
    def message_storage_size(self): return self._used
class App:
    def __init__(self, ps, node, dp, router):
        self.peer_settings = ps; self.node = node; self.disable_propagation = dp; self.message_router = router

cases = json.load(sys.stdin)
now_unix = cases[0]["now_unix"] if cases else 0
g["NOW"] = datetime.fromtimestamp(now_unix, tz=timezone.utc).replace(tzinfo=None)
g["UTCFROMTS"] = lambda t: datetime.fromtimestamp(t, tz=timezone.utc).replace(tzinfo=None)

out = []
for c in cases:
    ps = {"last_announce": c["ts"], "node_last_announce": c["ts"],
          "node_connects": c["n"], "served_page_requests": c["n"], "served_file_requests": c["n"]}
    node = Node([0]*c["links"]) if c["has_node"] else None
    router = Router(c["limit"], c["used"])
    app = App(ps, node, (not c["prop_on"]), router)
    sobj = type("S", (), {})(); sobj.app = app
    dw = DW(); sobj.display_widget = dw
    ns = {"__builtins__": __builtins__, "self": sobj, "pretty_date": pretty_date, "RNS": RNS}
    exec(blocks[c["method_idx"]], ns)
    out.append(dw.text)
json.dump(out, sys.stdout)
`

	var want []string
	runPythonNomadnet(t, inputs, script, &want)

	for i, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			got := goStatLabel(c, now)
			if got != want[i] {
				t.Errorf("%s = %q, want %q (Python)", c.Name, got, want[i])
			}
		})
	}
}

func goStatLabel(c struct {
	MethodIdx int
	Name      string
	TS        *int64
	HasNode   bool
	Links     int
	Used      *int64
	Limit     *int64
	PropOn    bool
	N         int
}, now time.Time) string {
	switch c.MethodIdx {
	case 0:
		return AnnounceTimeLabel(c.TS, now)
	case 1:
		return NodeAnnounceTimeLabel(c.TS, now)
	case 2:
		return NodeActiveConnectionsLabel(c.Links, c.HasNode)
	case 3:
		return NodeStorageStatsLabel(c.Used, c.Limit, c.HasNode, c.PropOn)
	case 4:
		return NodeTotalConnectionsLabel(c.N, c.HasNode)
	case 5:
		return NodeTotalPagesLabel(c.N, c.HasNode)
	case 6:
		return NodeTotalFilesLabel(c.N, c.HasNode)
	}
	return ""
}
