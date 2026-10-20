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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// TestChannelsRefreshRoomIfVisible pins the live-refresh path: only while a
// room is on screen does it reload the room's messages and members and refresh
// the header from the hub's live state.
func TestChannelsRefreshRoomIfVisible(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewChannelsDisplay(app, nil)
	cd.app = app
	cd.SetHubs([]HubView{
		&fakeHubView{name: "Community", status: hubStatusDisconnected},
		&fakeHubView{name: "RaspPi Local Hub", status: hubStatusConnected, rooms: []string{"test"}},
	})
	cd.roomWidget = NewRoomWidget(app, "RaspPi Local Hub", "test")

	refreshCalls, memberCalls := 0, 0
	refresh := func(hubIdx int, room string) []ChannelMessage {
		refreshCalls++
		if hubIdx != 1 || room != "test" {
			t.Errorf("refresh called with (%v, %q), want (1, test)", hubIdx, room)
		}
		return []ChannelMessage{{Nick: "alice", Text: "hello"}}
	}
	members := func(hubIdx int, room string) []ChannelMember {
		memberCalls++
		return []ChannelMember{{Nick: "alice", Hash: "abcd"}}
	}

	// Not showing a room: nothing is refreshed.
	cd.paneMode = "info"
	cd.RefreshRoomIfVisible(refresh, members)
	if refreshCalls != 0 || memberCalls != 0 {
		t.Fatalf("refreshed while pane mode is %q", cd.paneMode)
	}

	cd.paneMode = "room"
	cd.selectedHubIdx = 1
	cd.selectedRoom = "test"
	cd.RefreshRoomIfVisible(refresh, members)
	if refreshCalls != 1 || memberCalls != 1 {
		t.Fatalf("refresh calls = %v/%v, want 1/1", refreshCalls, memberCalls)
	}
	if got := cd.roomWidget.chatMessages; len(got) != 1 || got[0].Text != "hello" {
		t.Errorf("room messages not refreshed: %v", got)
	}
	if got := cd.roomWidget.members; len(got) != 1 || got[0].Nick != "alice" {
		t.Errorf("room members not refreshed: %v", got)
	}
	if hdr := cd.roomWidget.header.GetText(true); !strings.Contains(hdr, "RaspPi Local Hub v0.3.2") ||
		!strings.Contains(hdr, "Connected") {
		t.Errorf("room header not refreshed from hub state: %q", hdr)
	}

	// A nil members provider leaves the member pane alone.
	cd.roomWidget.SetMembers([]ChannelMember{{Nick: "bob"}})
	cd.RefreshRoomIfVisible(refresh, nil)
	if got := cd.roomWidget.members; len(got) != 1 || got[0].Nick != "bob" {
		t.Errorf("nil member provider disturbed the members pane: %v", got)
	}
}

// TestChannelsUpdateRooms pins the row composition: joined, unread and plain
// rooms carry Python's [*] / [!] / two-space prefixes and the topic/member
// secondary line.
func TestChannelsUpdateRooms(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.UpdateRooms([]ChannelInfo{
		{Name: "joined", Members: 3, Topic: "a topic", Joined: true},
		{Name: "unread", Members: 2, Unread: true},
		{Name: "plain"},
	})
	if got := cd.rooms.GetItemCount(); got != 3 {
		t.Fatalf("room list has %v rows, want 3", got)
	}
	wantRows := []string{"[*] #joined", "[!] #unread", "  #plain"}
	for i, want := range wantRows {
		main, secondary := cd.rooms.GetItemText(i)
		if main != want {
			t.Errorf("row %v = %q, want %q", i, main, want)
		}
		if i == 0 && !strings.Contains(secondary, "3 members") || i == 0 && !strings.Contains(secondary, "a topic") {
			t.Errorf("joined room secondary = %q, want members and topic", secondary)
		}
	}

	// A later update replaces the list rather than appending to it.
	cd.UpdateRooms([]ChannelInfo{{Name: "only"}})
	if got := cd.rooms.GetItemCount(); got != 1 {
		t.Errorf("UpdateRooms did not clear the previous rows (%v left)", got)
	}
}

// TestChannelsToggleCollapse pins ToggleCollapse: the flag flips, the room
// widget follows it, and the callback fires.
func TestChannelsToggleCollapse(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.roomWidget = NewRoomWidget(app, "hub", "room")
	cd.paneMode = "room"
	calls := 0
	cd.OnToggleCollapse = func() { calls++ }

	if cd.CollapseJoinPart() {
		t.Fatalf("collapseJoinPart starts true")
	}
	cd.ToggleCollapse()
	if !cd.CollapseJoinPart() || !cd.roomWidget.CollapseJoinPart() {
		t.Errorf("first toggle did not collapse (display=%v widget=%v)",
			cd.CollapseJoinPart(), cd.roomWidget.CollapseJoinPart())
	}
	cd.ToggleCollapse()
	if cd.CollapseJoinPart() || cd.roomWidget.CollapseJoinPart() {
		t.Errorf("second toggle did not restore")
	}
	if calls != 2 {
		t.Errorf("OnToggleCollapse fired %v times, want 2", calls)
	}
}

// TestChannelsJoinRoomDialog pins the join dialog: it opens on the selected
// hub's name and submitting a room routes (room, key) to the callback.
func TestChannelsJoinRoomDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.app = app
	cd.SetHubs([]HubView{&fakeHubView{name: "RaspPi Local Hub", status: hubStatusConnected}})

	type submission struct{ room, key string }
	var got []submission
	cd.OnJoinRoomSubmitted = func(room, key string) {
		got = append(got, submission{room, key})
	}

	cd.JoinRoomDialog()
	if cd.dialogOverlay == nil {
		t.Fatal("JoinRoomDialog did not open the overlay")
	}
	if title := cd.dialogOverlay.Dialog().GetTitle(); title != "Add Room on RaspPi Local Hub" {
		t.Errorf("dialog title = %q, want %q", title, "Add Room on RaspPi Local Hub")
	}
	if text := dialogText(t, cd); !strings.Contains(text, " Hub : RaspPi Local Hub") {
		t.Errorf("dialog text %q missing the hub line", text)
	}

	// Submitting with an empty room does nothing.
	dialogButton(t, cd, "Join").InputHandler()(
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if len(got) != 0 {
		t.Errorf("an empty room name submitted %v", got)
	}

	// Fill the room field and submit; the dialog closes and the callback runs.
	cd.JoinRoomDialog()
	dialogInputField(t, cd).SetText("testing")
	dialogButton(t, cd, "Join").InputHandler()(
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if cd.dialogOverlay != nil {
		t.Errorf("a successful join left the dialog open")
	}
	if len(got) != 1 || got[0].room != "testing" || got[0].key != "" {
		t.Errorf("OnJoinRoomSubmitted = %v, want one (testing, \"\") submission", got)
	}
}

// TestHubManagerSortedAndRooms pins the hub registry: insertion order, the
// connected-first sorted order, room add/remove, and the unread totals.
func TestHubManagerSortedAndRooms(t *testing.T) {
	t.Parallel()

	hm := NewHubManager()
	hm.AddHub("addr-b", "Beta")
	hm.AddHub("addr-a", "Alpha")
	hm.AddHub("addr-c", "Gamma")
	hm.SetStatus("addr-c", HubConnected)
	hm.SetStatus("addr-a", HubReconnecting)

	if got := hm.GetHub("addr-a"); got == nil || got.Name != "Alpha" {
		t.Fatalf("GetHub(addr-a) = %v, want Alpha", got)
	}
	if got := len(hm.ListHubs()); got != 3 {
		t.Fatalf("ListHubs = %v, want 3", got)
	}

	sorted := hm.SortedHubs()
	wantOrder := []string{"Gamma", "Alpha", "Beta"}
	for i, want := range wantOrder {
		if sorted[i].Name != want {
			t.Errorf("SortedHubs[%v] = %q, want %q", i, sorted[i].Name, want)
		}
	}
	if got := hm.ConnectedCount(); got != 1 {
		t.Errorf("ConnectedCount = %v, want 1", got)
	}

	// Rooms and unread accounting.
	room := hm.AddRoom("addr-c", "#one")
	if room == nil {
		t.Fatalf("AddRoom returned nil for a known hub")
	}
	if hm.AddRoom("missing", "#x") != nil {
		t.Errorf("AddRoom on an unknown hub created a room")
	}
	room.Unread = true
	hm.AddRoom("addr-a", "#two").Unread = true
	if got := hm.TotalUnreadCount(); got != 2 {
		t.Errorf("TotalUnreadCount = %v, want 2", got)
	}
	if got := hm.GetHub("addr-a").UnreadCount(); got != 1 {
		t.Errorf("hub UnreadCount = %v, want 1", got)
	}
	hm.RemoveRoom("addr-c", "#one")
	if got := hm.GetHub("addr-c").GetRoom("#one"); got != nil {
		t.Errorf("RemoveRoom left the room")
	}
	if got := hm.TotalUnreadCount(); got != 1 {
		t.Errorf("TotalUnreadCount after remove = %v, want 1", got)
	}

	hm.DeleteHub("addr-a")
	if hm.GetHub("addr-a") != nil {
		t.Errorf("DeleteHub left the hub")
	}
	if got := len(hm.ListHubs()); got != 2 {
		t.Errorf("ListHubs after delete = %v, want 2", got)
	}
}

// TestHubStatusLabels pins Python's status label table (Channels.py:1747-1752).
func TestHubStatusLabels(t *testing.T) {
	t.Parallel()
	want := map[int]string{0: "Disconnected", 1: "Connecting", 2: "Connected", 3: "Failed"}
	for status, label := range want {
		if got := HubStatusLabels[status]; got != label {
			t.Errorf("HubStatusLabels[%v] = %q, want %q", status, got, label)
		}
	}
	if len(HubStatusLabels) != len(want) {
		t.Errorf("HubStatusLabels has %v entries, want %v", len(HubStatusLabels), len(want))
	}
}

// TestCollapsedJoinPartWidget pins the collapsed join/part summary: the label's
// singular/plural wording and the widget's centered text.
func TestCollapsedJoinPartWidget(t *testing.T) {
	t.Parallel()

	if got, want := CollapsedJoinPartLabel(1), "  ⋯  1 join/leave event  ⋯"; got != want {
		t.Errorf("CollapsedJoinPartLabel(1) = %q, want %q", got, want)
	}
	if got, want := CollapsedJoinPartLabel(4), "  ⋯  4 join/leave events  ⋯"; got != want {
		t.Errorf("CollapsedJoinPartLabel(4) = %q, want %q", got, want)
	}
	tv := CollapsedJoinPartWidget(3)
	if got := tv.GetText(true); got != CollapsedJoinPartLabel(3) {
		t.Errorf("widget text = %q, want the label", got)
	}
}

// TestSyncStatusRefreshInterval pins the refresh helper's interval accessor.
func TestSyncStatusRefreshInterval(t *testing.T) {
	t.Parallel()
	for _, want := range []int{0, 1, 30} {
		if got := NewSyncStatusRefresh(want).Interval(); got != want {
			t.Errorf("NewSyncStatusRefresh(%v).Interval() = %v", want, got)
		}
	}
}

// TestRoomDrafts pins the draft store: save/restore/remove keyed by
// (hub hash, dest name, room), with an empty save clearing the draft.
func TestRoomDrafts(t *testing.T) {
	t.Parallel()

	rd := NewRoomDrafts()
	rd.Save("hub1", "rrc.hub", "#room", "hello")
	if got := rd.Restore("hub1", "rrc.hub", "#room"); got != "hello" {
		t.Errorf("Restore = %q, want hello", got)
	}
	// The dest name is part of the key: hubs sharing a hash differ by aspect.
	if got := rd.Restore("hub1", "rrc.custom", "#room"); got != "" {
		t.Errorf("Restore with another dest name = %q, want empty", got)
	}
	rd.Save("hub1", "rrc.hub", "#room", "")
	if got := rd.Restore("hub1", "rrc.hub", "#room"); got != "" {
		t.Errorf("empty Save did not clear the draft")
	}
	rd.Save("hub1", "rrc.hub", "#room", "again")
	rd.Remove("hub1", "rrc.hub", "#room")
	if got := rd.Restore("hub1", "rrc.hub", "#room"); got != "" {
		t.Errorf("Remove left the draft")
	}
}
