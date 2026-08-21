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
	"reflect"
	"sync"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// partialInnerInputs is the input battery for the live cross-implementation
// parity check of parsePartial. Each entry is the "inner" text that follows the
// leading "`{" partial directive and includes the closing brace, matching the
// Python call parse_partial(line[2:]) on a line "`{" + inner. The Go test calls
// Parse("`{" + inner) which dispatches to parsePartial(inner). The expected
// structure is derived FRESH on every run by executing the real Python
// nomadnet.ui.textui.MicronParser.parse_partial reference (see
// partialPythonOnce).
//
// The battery covers: url-only, url+refresh, url+refresh+pid, refresh below one
// (dropped), multi-field pid, empty url (no node), too many components (no
// node), pid-only field, non-numeric refresh (no node), and a pid value
// containing an "=" (pid=a=b → Python keeps only the segment between the first
// and second "=", i.e. "a").
var partialInnerInputs = []string{
	"page_name}",
	"page_name`5}",
	"page_name`5`pid=foo}",
	"page_name`0.5}",
	"page_name`2`field1|pid=abc}",
	"page_name`10`a|b|pid=xyz}",
	"}",
	"a`b`c`d}",
	"page_name`2`pid=bar}",
	"page_name`abc}",
	"page_name`2`pid=a=b}",
}

// partialParityScript imports the real nomadnet.ui.textui.MicronParser
// reference and applies parse_partial to each inner string supplied as JSON on
// stdin, emitting the fresh parsed structure as JSON on stdout. Each result is
// either {"node":0} (Python returned None/empty) or {"node":1,...} with the
// fields the Go test inspects (url, has_refresh, refresh, fields, partial_id).
const partialParityScript = `
import sys, json
import nomadnet.ui.textui.MicronParser as M
inners = json.loads(sys.stdin.read() or "[]")
out = []
for inner in inners:
    r = M.parse_partial(inner)
    if not r:
        out.append({"node": 0})
        continue
    p = r[0]
    refresh = p.partial_refresh
    out.append({
        "node": 1,
        "url": p.partial_url,
        "has_refresh": refresh is not None,
        "refresh": refresh if refresh is not None else 0.0,
        "fields": p.partial_fields,
        "partial_id": p.partial_id if p.partial_id is not None else "",
    })
print(json.dumps(out, ensure_ascii=False))
`

// partialPythonOnce caches the single live Python run that derives fresh
// expected parse_partial structures, so every subtest shares one python3 exec.
var (
	partialPythonOnce sync.Once
	partialPythonOut  []map[string]any
)

func partialPython(t *testing.T) []map[string]any {
	t.Helper()
	partialPythonOnce.Do(func() {
		testutils.RunPythonNomadnet(t, partialInnerInputs, partialParityScript, &partialPythonOut)
	})
	return partialPythonOut
}

func TestParsePartialParity(t *testing.T) {
	t.Parallel()
	want := partialPython(t)
	for i, inner := range partialInnerInputs {
		name := inner
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := want[i]
			nodeCount, _ := w["node"].(float64)
			nodes := Parse("`{" + inner)
			if len(nodes) != int(nodeCount) {
				t.Fatalf("Parse partial len = %v, want %v (inner %q)", len(nodes), int(nodeCount), inner)
			}
			if nodeCount == 0 {
				return
			}
			n := nodes[0]
			if n.Type != NodePartial {
				t.Fatalf("node type = %v, want %v", n.Type, NodePartial)
			}
			wantURL, _ := w["url"].(string)
			if n.PartialURL != wantURL {
				t.Errorf("partial url = %q, want %q", n.PartialURL, wantURL)
			}
			wantHasRefresh, _ := w["has_refresh"].(bool)
			if n.HasRefresh != wantHasRefresh {
				t.Errorf("has refresh = %v, want %v", n.HasRefresh, wantHasRefresh)
			}
			wantRefresh, _ := w["refresh"].(float64)
			if n.PartialRefresh != wantRefresh {
				t.Errorf("partial refresh = %v, want %v", n.PartialRefresh, wantRefresh)
			}
			wantFields, _ := w["fields"].([]any)
			gotFields := make([]string, len(n.PartialFields))
			copy(gotFields, n.PartialFields)
			wantStrs := make([]string, len(wantFields))
			for j, f := range wantFields {
				wantStrs[j], _ = f.(string)
			}
			if !reflect.DeepEqual(gotFields, wantStrs) {
				t.Errorf("partial fields = %v, want %v", gotFields, wantStrs)
			}
			wantPID, _ := w["partial_id"].(string)
			if n.PartialID != wantPID {
				t.Errorf("partial id = %q, want %q", n.PartialID, wantPID)
			}
		})
	}
}
