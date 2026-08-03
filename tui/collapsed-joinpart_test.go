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

// TestCollapsedJoinPartLabelPythonParity verifies CollapsedJoinPartLabel
// against Python's _collapsed_join_part_widget label (Channels.py:1251):
//
//	"  ⋯  " + str(n) + " join/leave event" + ("" if n == 1 else "s") + "  ⋯"
//
// The ellipsis is U+22EF (MIDLINE HORIZONTAL ELLIPSIS). Expected values were
// captured from /tmp/collapsed_ref.py.
func TestCollapsedJoinPartLabelPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, "  ⋯  0 join/leave events  ⋯"},
		{"one singular", 1, "  ⋯  1 join/leave event  ⋯"},
		{"two plural", 2, "  ⋯  2 join/leave events  ⋯"},
		{"three", 3, "  ⋯  3 join/leave events  ⋯"},
		{"five", 5, "  ⋯  5 join/leave events  ⋯"},
		{"ten", 10, "  ⋯  10 join/leave events  ⋯"},
		{"hundred", 100, "  ⋯  100 join/leave events  ⋯"},
		{"thousand", 1000, "  ⋯  1000 join/leave events  ⋯"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CollapsedJoinPartLabel(tt.n)
			if got != tt.want {
				t.Errorf("CollapsedJoinPartLabel(%v) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestIsJoinPartSystemPythonParity verifies IsJoinPartSystem against Python's
// _is_joinpart_system (Channels.py:1240). Expected values were captured live
// from nomadnet.ui.textui.Channels via a glib-stub harness.
func TestIsJoinPartSystemPythonParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  ChannelMessage
		want bool
	}{
		{"system joined", ChannelMessage{IsSystem: true, Text: "alice joined"}, true},
		{"system left", ChannelMessage{IsSystem: true, Text: "alice left"}, true},
		{"you joined", ChannelMessage{IsSystem: true, Text: "You joined"}, false},
		{"you left", ChannelMessage{IsSystem: true, Text: "You left"}, false},
		{"padded joined", ChannelMessage{IsSystem: true, Text: "  bob joined  "}, true},
		{"quit", ChannelMessage{IsSystem: true, Text: "alice quit"}, false},
		{"msg kind", ChannelMessage{IsSystem: false, Text: "alice joined"}, false},
		{"empty", ChannelMessage{IsSystem: true, Text: ""}, false},
		{"whitespace", ChannelMessage{IsSystem: true, Text: "   "}, false},
		{"joined the room", ChannelMessage{IsSystem: true, Text: "carol joined the room"}, false},
		{"left the room", ChannelMessage{IsSystem: true, Text: "carol left the room"}, false},
		{"carol left", ChannelMessage{IsSystem: true, Text: "carol left"}, true},
		{"notice joined", ChannelMessage{IsNotice: true, Text: "x joined"}, false},
		{"you left the room", ChannelMessage{IsSystem: true, Text: "You left the room"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsJoinPartSystem(tt.msg); got != tt.want {
				t.Errorf("IsJoinPartSystem(%+v) = %v, want %v", tt.msg, got, tt.want)
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
