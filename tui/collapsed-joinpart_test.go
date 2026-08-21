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

// TestCollapsedJoinPartLabelPythonParity is a LIVE cross-implementation
// check: it AST-extracts the `label = ...` expression from Python's
// _collapsed_joinpart_widget (nomadnet.ui.textui.Channels) and derives the
// expected label freshly on every run for each count n. The ellipsis is
// U+22EF (MIDLINE HORIZONTAL ELLIPSIS). Go owns the input battery; Python
// owns the reference behavior. The test SKIPs, not fails, when the Python
// reference is not importable.
func TestCollapsedJoinPartLabelPythonParity(t *testing.T) {
	t.Parallel()

	nums := []int{0, 1, 2, 3, 5, 10, 100, 1000, 999999}

	const script = `
import sys, json, ast, inspect, textwrap
import nomadnet.ui.textui.Channels as C
src = textwrap.dedent(inspect.getsource(C._collapsed_joinpart_widget))
fn = ast.parse(src).body[0]
label_assign = None
for s in fn.body:
    if isinstance(s, ast.Assign) and len(s.targets) == 1 and isinstance(s.targets[0], ast.Name) and s.targets[0].id == "label":
        label_assign = s; break
assert label_assign, "no label assignment in _collapsed_joinpart_widget"
mod = ast.Module(body=[label_assign], type_ignores=[]); ast.fix_missing_locations(mod)
code = compile(mod, "<label>", "exec")
nums = json.load(sys.stdin)
out = []
for n in nums:
    ns = {"__builtins__": __builtins__, "n": n}
    exec(code, ns)
    out.append(ns["label"])
json.dump(out, sys.stdout)
`

	var want []string
	runPythonNomadnet(t, nums, script, &want)

	for i, n := range nums {
		t.Run(collapsedLabelCaseName(n), func(t *testing.T) {
			t.Parallel()
			got := CollapsedJoinPartLabel(n)
			if got != want[i] {
				t.Errorf("CollapsedJoinPartLabel(%v) = %q, want %q (Python)", n, got, want[i])
			}
		})
	}
}

func collapsedLabelCaseName(n int) string {
	switch n {
	case 0:
		return "zero"
	case 1:
		return "one_singular"
	default:
		return "plural"
	}
}

// TestIsJoinPartSystemPythonParity is a LIVE cross-implementation check: it
// execs Python's real _is_joinpart_system (nomadnet.ui.textui.Channels) with
// mock message objects carrying .kind and .text, and derives the expected
// bool freshly on every run. Go's ChannelMessage is mapped to the Python mock
// as: kind="system" when IsSystem, else "notice" when IsNotice, else "msg";
// text=Text (Python strips and checks endswith " joined"/" left", rejecting
// "You " prefixes and non-system kinds). Go owns the input battery; Python
// owns the reference behavior. The test SKIPs, not fails, when the Python
// reference is not importable.
func TestIsJoinPartSystemPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  ChannelMessage
	}{
		{"system joined", ChannelMessage{IsSystem: true, Text: "alice joined"}},
		{"system left", ChannelMessage{IsSystem: true, Text: "alice left"}},
		{"you joined", ChannelMessage{IsSystem: true, Text: "You joined"}},
		{"you left", ChannelMessage{IsSystem: true, Text: "You left"}},
		{"padded joined", ChannelMessage{IsSystem: true, Text: "  bob joined  "}},
		{"quit", ChannelMessage{IsSystem: true, Text: "alice quit"}},
		{"msg kind", ChannelMessage{IsSystem: false, Text: "alice joined"}},
		{"empty", ChannelMessage{IsSystem: true, Text: ""}},
		{"whitespace", ChannelMessage{IsSystem: true, Text: "   "}},
		{"joined the room", ChannelMessage{IsSystem: true, Text: "carol joined the room"}},
		{"left the room", ChannelMessage{IsSystem: true, Text: "carol left the room"}},
		{"carol left", ChannelMessage{IsSystem: true, Text: "carol left"}},
		{"notice joined", ChannelMessage{IsNotice: true, Text: "x joined"}},
		{"you left the room", ChannelMessage{IsSystem: true, Text: "You left the room"}},
	}

	type joinpartInput struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	inputs := make([]joinpartInput, len(tests))
	for i, tt := range tests {
		kind := "msg"
		if tt.msg.IsSystem {
			kind = "system"
		} else if tt.msg.IsNotice {
			kind = "notice"
		}
		inputs[i] = joinpartInput{Kind: kind, Text: tt.msg.Text}
	}

	const script = `
import sys, json
import nomadnet.ui.textui.Channels as C
class M: pass
cases = json.load(sys.stdin)
out = []
for c in cases:
    m = M(); m.kind = c["kind"]; m.text = c["text"]
    out.append(bool(C._is_joinpart_system(m)))
json.dump(out, sys.stdout)
`

	var want []bool
	runPythonNomadnet(t, inputs, script, &want)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsJoinPartSystem(tt.msg); got != want[i] {
				t.Errorf("IsJoinPartSystem(%+v) = %v, want %v (Python)", tt.msg, got, want[i])
			}
		})
	}
}

// TestCollapseJoinPartMessages verifies the run/flush collapse logic mirrors
// Python's RoomWidget.update_messages (Channels.py:758-776): consecutive
// join/leave system messages collapse into one synthetic summary message;
// other messages flush the run and pass through unchanged.
func TestCollapseJoinPartMessages(t *testing.T) {
	t.Parallel()

	msgs := []ChannelMessage{
		{IsSystem: true, Text: "alice joined"},
		{IsSystem: true, Text: "bob left"},
		{IsSystem: true, Text: "carol joined"},
		{Nick: "dave", Text: "hi"},
		{IsSystem: true, Text: "eve left"},
		{IsSystem: true, Text: "You joined"}, // not collapsible
	}
	got := CollapseJoinPartMessages(msgs)
	if len(got) != 4 {
		t.Fatalf("got %v messages, want 4: %+v", len(got), got)
	}
	if got[0].Text != CollapsedJoinPartLabel(3) {
		t.Errorf("got[0].Text = %q, want %q", got[0].Text, CollapsedJoinPartLabel(3))
	}
	if !got[0].IsSystem {
		t.Error("collapsed summary should be IsSystem")
	}
	if got[1].Nick != "dave" {
		t.Errorf("got[1].Nick = %q, want dave", got[1].Nick)
	}
	if got[2].Text != CollapsedJoinPartLabel(1) {
		t.Errorf("got[2].Text = %q, want %q", got[2].Text, CollapsedJoinPartLabel(1))
	}
	if got[3].Text != "You joined" {
		t.Errorf("got[3].Text = %q, want \"You joined\"", got[3].Text)
	}

	// Empty input yields empty output.
	if out := CollapseJoinPartMessages(nil); len(out) != 0 {
		t.Errorf("nil input got %v messages, want 0", len(out))
	}

	// All-collapsible input collapses to a single summary.
	all := []ChannelMessage{
		{IsSystem: true, Text: "a joined"},
		{IsSystem: true, Text: "b left"},
	}
	if out := CollapseJoinPartMessages(all); len(out) != 1 || out[0].Text != CollapsedJoinPartLabel(2) {
		t.Errorf("all-collapsible got %+v, want single 2-count summary", out)
	}
}
