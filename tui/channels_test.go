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

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
)

func TestNewChannelsDisplay(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rooms := []ChannelInfo{
		{Name: "general", Members: 10, Joined: true},
		{Name: "random", Members: 5, Unread: true},
	}

	cd := NewChannelsDisplay(app, rooms)
	if cd == nil {
		t.Fatal("NewChannelsDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestChannelsDisplayEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	if cd == nil {
		t.Fatal("NewChannelsDisplay with empty list returned nil")
	}
}

func TestChannelsDisplayShowMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	msgs := []ChannelMessage{
		{Nick: "Alice", Text: "Hello!", IsSelf: false},
		{Nick: "Bob", Text: "Hi there!", IsSelf: true},
		{Text: "System message", IsSystem: true},
	}

	// Should not panic
	cd.ShowMessages(msgs)
}

func TestChannelsDisplayShowMembers(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	members := []ChannelMember{
		{Nick: "Alice", Hash: "abc123def456", Online: true},
		{Nick: "Bob", Hash: "789ghi012jkl", Online: false},
	}

	// Should not panic
	cd.ShowMembers(members)
}

func TestNickColor(t *testing.T) {
	t.Parallel()

	color1 := nickColor("Alice")
	color2 := nickColor("Bob")

	if color1 == "" {
		t.Error("nickColor returned empty string")
	}
	if color2 == "" {
		t.Error("nickColor returned empty string")
	}

	// Same nick should always return same color
	if nickColor("Alice") != color1 {
		t.Error("nickColor not deterministic")
	}
}

func TestNickColorDifferent(t *testing.T) {
	t.Parallel()

	// Different nicks should (usually) get different colors
	color1 := nickColor("Alice12345")
	color2 := nickColor("Bob67890")
	// Not guaranteed to be different, but shouldn't panic
	_ = color1
	_ = color2
}

func TestChannelsDisplayKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	var fired []string
	cd.OnNewHub = func() { fired = append(fired, "new_hub") }
	cd.OnJoinRoom = func() { fired = append(fired, "join_room") }
	cd.OnConnect = func() { fired = append(fired, "connect") }
	cd.OnDisconnect = func() { fired = append(fired, "disconnect") }
	cd.OnToggleAutoReconnect = func() { fired = append(fired, "auto_reconnect") }
	cd.OnEditHub = func() { fired = append(fired, "edit_hub") }
	cd.OnRemoveHub = func() { fired = append(fired, "remove_hub") }
	cd.OnToggleChannelList = func() { fired = append(fired, "toggle_list") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-n", tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModNone), "new_hub"},
		{"ctrl-a", tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone), "join_room"},
		{"ctrl-r", tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone), "connect"},
		{"ctrl-w", tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone), "disconnect"},
		{"ctrl-t", tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModNone), "auto_reconnect"},
		{"ctrl-e", tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "edit_hub"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "remove_hub"},
		{"ctrl-y", tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone), "toggle_list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := cd.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %v was not consumed", tt.name)
			}
			if len(fired) != 1 || fired[0] != tt.want {
				t.Errorf("key %v fired %v, want [%v]", tt.name, fired, tt.want)
			}
		})
	}
}

func TestChannelsDisplayChannelListVisibility(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	if !cd.ChannelListVisible() {
		t.Error("ChannelListVisible should default to true")
	}

	// Python toggle_channel_list guard (Channels.py:1533): the list cannot
	// collapse while the right pane still shows the placeholder.
	cd.ToggleChannelListState()
	if !cd.ChannelListVisible() {
		t.Error("ToggleChannelListState with the placeholder showing should be a no-op")
	}

	// With a room open the toggle applies.
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)
	cd.ToggleChannelListState()
	if cd.ChannelListVisible() {
		t.Error("ToggleChannelListState should toggle visibility once a room is open")
	}

	cd.ToggleChannelListState()
	if !cd.ChannelListVisible() {
		t.Error("Second toggle should restore visibility")
	}
}

func TestShowUserInfoDialog(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	// Verify function exists and can be called without panic
	cd.ShowUserInfoDialog("Alice", "aabb1122334455667788", "ccdd1122334455667788", false, nil)

	// Verify self-user case
	cd.ShowUserInfoDialog("Bob", "aabb1122334455667788", "", true, nil)

	// Verify the identity-not-in-cache branch (no LXMF hash, not self)
	cd.ShowUserInfoDialog("Carol", "aabb1122334455667788", "", false, nil)
}

// TestChannelsDisplayBootLayout pins the Channels page boot (no-hubs) layout
// against the Python original (Channels.py:1459-1468, 1590-1607): a two-pane
// Columns with NO outer border — a bordered "Channels" left pane (width 36)
// holding an IndicativeListBox ("───" indicators + the left-aligned "No hubs
// yet…" empty state), and a bordered untitled right pane holding a top-filled,
// centered "Select or add a hub to begin" placeholder. Capture-verified
// byte-identical to Python at 80x24.
func TestChannelsDisplayBootLayout(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)

	rows := renderPrimitive(t, cd.Widget(), 80, 24)

	// Row 0: left border carries the "Channels" title; the right pane's border
	// begins at column 36 (no title, no outer border).
	if !strings.Contains(rows[0], "Channels") {
		t.Errorf("row 0 = %q, want the left-pane border titled 'Channels'", rows[0])
	}
	if c := cellAt(rows, 36, 0); c != "┌" {
		t.Errorf("right-pane top-left border at (36,0) = %q, want '┌'", c)
	}

	// Row 1: the IndicativeListBox top indicator "───", centered in the left
	// pane (inner 34 → starts at col 17).
	if !strings.Contains(rows[1], "───") {
		t.Errorf("row 1 = %q, want the top '───' indicator", rows[1])
	}
	if c := cellAt(rows, 17, 1); c != "─" {
		t.Errorf("top indicator '─' at (17,1) = %q, want '─' (row1=%q)", c, rows[1])
	}

	// Row 2: left pane blank (the leading newline of the empty-state text);
	// right pane shows the centered "  Select or add a hub to begin" (the text
	// has 2 leading spaces, 30 cols, centered in inner 42 → ceil-left
	// (42-30+1)/2 = 6 left pad + 2 from the text → 'S' at col 36+1+6+2 = 45).
	if c := cellAt(rows, 45, 2); c != "S" {
		t.Errorf("'S' of 'Select…' at (45,2) = %q, want 'S' (row2=%q)", c, rows[2])
	}
	if !strings.Contains(rows[2], "Select or add a hub to begin") {
		t.Errorf("row 2 = %q, want 'Select or add a hub to begin'", rows[2])
	}

	// Rows 3-4: the wrapped no-hubs text, left-aligned with a 2-space indent.
	if !strings.Contains(rows[3], "No hubs yet. Press Ctrl-N to add") {
		t.Errorf("row 3 = %q, want '  No hubs yet. Press Ctrl-N to add'", rows[3])
	}
	if !strings.Contains(rows[4], "one.") {
		t.Errorf("row 4 = %q, want 'one.'", rows[4])
	}

	// Row 22: the bottom "───" indicator (last inner row before the border).
	if !strings.Contains(rows[22], "───") {
		t.Errorf("row 22 = %q, want the bottom '───' indicator", rows[22])
	}

	// Row 23: both panes' bottom borders.
	if c := cellAt(rows, 0, 23); c != "└" {
		t.Errorf("left-pane bottom-left border at (0,23) = %q, want '└'", c)
	}
	if c := cellAt(rows, 36, 23); c != "└" {
		t.Errorf("right-pane bottom-left border at (36,23) = %q, want '└'", c)
	}
}

func TestMaybeAutoconnect(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := rrc.NewManager(dir, nil)
	hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "Hub 1")

	// Nil hub is a no-op
	MaybeAutoconnect(nil)

	// StatusDisconnected -> attempts connect
	hub.Status = rrc.StatusDisconnected
	MaybeAutoconnect(hub)

	// StatusFailed -> attempts connect
	hub.Status = rrc.StatusFailed
	MaybeAutoconnect(hub)

	// StatusConnected -> no-op
	hub.Status = rrc.StatusConnected
	MaybeAutoconnect(hub)
}

func TestChannelsDisplaySelectedEntry(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	_, ok := cd.SelectedEntry()
	if ok {
		t.Error("SelectedEntry on empty list should return false")
	}

	hubs := []HubView{
		&fakeHub{name: "Hub A", status: 2, joined: []string{"general"}},
	}
	cd.SetHubs(hubs)

	entry, ok := cd.SelectedEntry()
	if !ok {
		t.Fatal("SelectedEntry returned false after SetHubs")
	}
	if entry.Kind != RowHub {
		t.Errorf("entry.Kind = %v, want RowHub", entry.Kind)
	}
}

func TestChannelsDisplayF8AndCtrlY(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)

	// Ctrl-Y toggles channel list state (Python toggle_channel_list; the
	// placeholder guard is pinned separately in channels-dialogs_test.go).
	if !cd.ChannelListVisible() {
		t.Error("ChannelListVisible should default to true")
	}
	cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone))
	if cd.ChannelListVisible() {
		t.Error("ChannelListVisible should be false after Ctrl-Y")
	}

	// F8 toggles join/part collapse
	if cd.collapseJoinPart {
		t.Error("collapseJoinPart should default to false")
	}
	cd.handleInput(tcell.NewEventKey(tcell.KeyF8, 0, tcell.ModNone))
	if !cd.collapseJoinPart {
		t.Error("collapseJoinPart should be true after F8")
	}
}
