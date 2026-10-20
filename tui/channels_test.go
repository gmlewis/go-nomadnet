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
	"sync"
	"testing"

	"github.com/gmlewis/go-reticulum/rrc"
	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
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
		{Text: "A notice", IsNotice: true},
		{Text: "An error", IsError: true},
	}

	cd.ShowMessages(msgs)
	text := cd.messages.GetText(false)
	for _, want := range []string{
		"[" + nickColor("Alice") + "]Alice[-] Hello!",
		"[green]Bob[-] Hi there!",
		"[gray]System message[-]",
		"[yellow]A notice[-]",
		"[red]An error[-]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("messages view %q missing %q", text, want)
		}
	}

	// With join/part collapsing on, a run of system join/leave events is
	// summarised instead of printed one per line.
	cd.collapseJoinPart = true
	cd.ShowMessages([]ChannelMessage{
		{Text: "alice joined", IsSystem: true},
		{Text: "bob left", IsSystem: true},
	})
	collapsed := cd.messages.GetText(false)
	if strings.Contains(collapsed, "alice joined") {
		t.Errorf("collapsed view still prints each join/leave: %q", collapsed)
	}
	if !strings.Contains(collapsed, "join/leave events") {
		t.Errorf("collapsed view %q missing the summary label", collapsed)
	}
}

func TestChannelsDisplayShowMembers(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	members := []ChannelMember{
		{Nick: "Alice", Hash: "abc123def456", Online: true},
		{Nick: "Bob", Hash: "789ghi012jkl", Online: false},
	}

	cd.ShowMembers(members)
	if got := cd.members.GetItemCount(); got != 2 {
		t.Fatalf("member list has %v rows, want 2", got)
	}
	wantRows := []struct{ main, secondary string }{
		{"● Alice", "abc123def456"},
		{"○ Bob", "789ghi012jkl"},
	}
	for i, want := range wantRows {
		main, secondary := cd.members.GetItemText(i)
		if main != want.main || secondary != want.secondary {
			t.Errorf("member row %v = (%q, %q), want (%q, %q)", i, main, secondary, want.main, want.secondary)
		}
	}

	// A later update replaces the list.
	cd.ShowMembers([]ChannelMember{{Nick: "Carol", Hash: "abcdef123456", Online: true}})
	if got := cd.members.GetItemCount(); got != 1 {
		t.Errorf("ShowMembers did not clear the previous rows (%v left)", got)
	}
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

	// The full profile: nick, identity and LXMF address, plus the Open
	// Conversation action when the peer's LXMF address is known.
	var opened int
	cd.ShowUserInfoDialog("Alice", "aabb1122334455667788", "ccdd1122334455667788", false, func() { opened++ })
	if cd.dialogOverlay == nil {
		t.Fatal("ShowUserInfoDialog did not open the overlay")
	}
	if title := cd.dialogOverlay.Dialog().GetTitle(); title != "User Info" {
		t.Errorf("dialog title = %q, want User Info", title)
	}
	text := dialogText(t, cd)
	for _, want := range []string{
		" Nick     : Alice",
		" Identity : aabb1122334455667788",
		" LXMF     : ccdd1122334455667788",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("user info dialog %q missing %q", text, want)
		}
	}
	dialogButton(t, cd, "Open Conversation").InputHandler()(
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if cd.dialogOverlay != nil {
		t.Error("Open Conversation left the dialog open")
	}
	if opened != 1 {
		t.Errorf("Open Conversation fired %v times, want 1", opened)
	}

	// Self: a marker instead of an action, and no LXMF row.
	cd.ShowUserInfoDialog("Bob", "aabb1122334455667788", "ccdd1122334455667788", true, nil)
	text = dialogText(t, cd)
	if !strings.Contains(text, " (This is you)") {
		t.Errorf("self dialog %q missing the self marker", text)
	}
	if strings.Contains(text, "Open Conversation") {
		t.Errorf("self dialog offers Open Conversation")
	}

	// Unknown peer: the cache-miss explanation, no action and no LXMF row.
	cd.ShowUserInfoDialog("Carol", "aabb1122334455667788", "", false, nil)
	text = dialogText(t, cd)
	for _, want := range []string{
		" Identity not in local cache;",
		" conversation can't be opened until",
		" the peer announces.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("cache-miss dialog %q missing %q", text, want)
		}
	}
	if strings.Contains(text, " LXMF     :") {
		t.Errorf("cache-miss dialog shows an LXMF row")
	}
}

// TestChannelsDisplayBootLayout pins the Channels page boot (no-hubs) layout
// against the Python original (Channels.py:1459-1468, 1590-1607): a two-pane
// Columns with NO outer border — a bordered "Channels" left pane (width 36)
// holding an IndicativeListBox ("───" indicators + the left-aligned "No hubs
// yet…" empty state), and a bordered untitled right pane holding a top-filled,
// centered "Select or add a hub to begin" placeholder. Byte-identical to
// Python at 80x24.
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

// statusCapture collects the hub statuses reported by a manager's change
// callback. The callback is not a single-goroutine observable: ConnectAsync
// fires it on the calling goroutine before it spawns the connect worker, and
// that worker fires it again from its own goroutine, so the collector is
// locked rather than assuming the two never overlap.
type statusCapture struct {
	mu       sync.Mutex
	statuses []int
}

func (c *statusCapture) append(status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses = append(c.statuses, status)
}

// snapshot returns a copy of the statuses seen so far, so an assertion never
// reads the slice the callback is appending to.
func (c *statusCapture) snapshot() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.statuses...)
}

// MaybeAutoconnect must connect a disconnected or failed hub only when that
// hub's AutoRcn (auto-reconnect) toggle is on. A hub the user has taken off
// automatic connection is never dialed by a mere selection: the only ways to
// bring it up are the explicit Ctrl-R connect, or sending in one of its rooms.
//
// The status is set through the hub's locked setter and read through its
// snapshot: a connect worker writes the status from its own goroutine, so a raw
// field read here would race with the very attempt this helper starts. The
// manager's change callback is the observable — ConnectAsync flips the status to
// Connecting and notifies synchronously, before it spawns the worker.
func TestMaybeAutoconnect(t *testing.T) {
	t.Parallel()

	// Nil hub is a no-op.
	MaybeAutoconnect(nil)

	tests := []struct {
		name          string
		autoReconnect bool
		status        int
		wantConnect   bool
	}{
		{name: "disconnected hub with AutoRcn on connects", autoReconnect: true, status: rrc.StatusDisconnected, wantConnect: true},
		{name: "failed hub with AutoRcn on connects", autoReconnect: true, status: rrc.StatusFailed, wantConnect: true},
		{name: "connected hub with AutoRcn on is left alone", autoReconnect: true, status: rrc.StatusConnected},
		{name: "disconnected hub with AutoRcn off is left alone", status: rrc.StatusDisconnected},
		{name: "failed hub with AutoRcn off is left alone", status: rrc.StatusFailed},
		{name: "connected hub with AutoRcn off is left alone", status: rrc.StatusConnected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := rrc.NewManager(tempDir(t), nil)
			hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "Hub 1")
			// Both toggles notify the manager, so they are set before the
			// change callback is wired and the capture reflects only the
			// connect attempt under test.
			hub.SetAutoReconnect(tt.autoReconnect, false)
			hub.SetStatus(tt.status, "")

			var statuses statusCapture
			mgr.SetChangeCallback(func() {
				status, _, _, _ := hub.Snapshot()
				statuses.append(status)
			})
			// Stop the connect worker before the test returns: with no
			// transport configured it fails, and a failed hub arms reconnects,
			// which would keep notifying for the life of the test binary.
			t.Cleanup(mgr.Shutdown)

			MaybeAutoconnect(hub)

			seen := statuses.snapshot()
			if !tt.wantConnect {
				if len(seen) != 0 {
					t.Errorf("MaybeAutoconnect fired %v change notification(s) %v, want none", len(seen), seen)
				}
				if got := hub.GetHubStatus(); got != tt.status {
					t.Errorf("hub status after MaybeAutoconnect = %v, want %v", got, tt.status)
				}
				return
			}
			if len(seen) == 0 {
				t.Fatal("MaybeAutoconnect did not start a connect")
			}
			if seen[0] != rrc.StatusConnecting {
				t.Errorf("status at the connect notification = %v, want %v", seen[0], rrc.StatusConnecting)
			}
		})
	}
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
