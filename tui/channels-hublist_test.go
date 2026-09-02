// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// fakeHub is a test-only HubView for pinning ComposeHubList against Python
// _compose_list_widgets (Channels.py:1599-1662) without importing the rrc
// package into the tui layer.
type fakeHub struct {
	name      string
	status    int
	joined    []string
	messages  []string
	unread    []string
	mentioned []string
	// Hub info panel + edit-hub dialog fields.
	addressHex    string
	serverName    string
	autoReconnect bool
	autoList      bool
	autoWho       bool
}

func (f fakeHub) Name() string           { return f.name }
func (f fakeHub) Status() int            { return f.status }
func (f fakeHub) JoinedRooms() []string  { return f.joined }
func (f fakeHub) MessageRooms() []string { return f.messages }
func (f fakeHub) UnreadRooms() []string  { return f.unread }
func (f fakeHub) MentionRooms() []string { return f.mentioned }

// Hub info panel fields (defaults keep the existing tests focused on the
// hub-list composition).
func (f fakeHub) AddressHex() string          { return f.addressHex }
func (f fakeHub) StatusText() string          { return "" }
func (f fakeHub) ServerName() string          { return f.serverName }
func (f fakeHub) MOTD() string                { return "" }
func (f fakeHub) AutoReconnect() bool         { return f.autoReconnect }
func (f fakeHub) AutoList() bool              { return f.autoList }
func (f fakeHub) AutoWho() bool               { return f.autoWho }
func (f fakeHub) AvailableRoomList() []string { return nil }

// TestComposeHubListGolden pins Python Channels._compose_list_widgets
// (Channels.py:1599-1662): for each hub a status-glyph + name entry, followed
// by the sorted union of its joined rooms and message-bearing rooms (empty
// names skipped), each as a "   <marker> #<room>" entry styled by room state
// (mentioned → irc_mention, unread → list_unresponsive, not-joined →
// list_unknown, joined → list_trusted only when the hub is connected else
// list_unknown). A blank spacer entry separates consecutive hubs. Golden
// labels use the plain (ASCII) glyph set: check "=", cross "X", info "i",
// warning "!", unread "[!]".
func TestComposeHubListGolden(t *testing.T) {
	t.Parallel()
	g := glyphsPlain

	cases := []struct {
		name string
		hubs []HubView
		want []HubListEntry
	}{
		{"no hubs", nil, nil},
		{
			name: "connected hub, two joined rooms",
			hubs: []HubView{
				fakeHub{name: "hub1", status: hubStatusConnected, joined: []string{"general", "random"}},
			},
			want: []HubListEntry{
				{Kind: RowHub, Label: "= hub1", Style: "list_trusted", HubIdx: 0},
				{Kind: RowRoom, Label: "     #general", Style: "list_trusted", HubIdx: 0, Room: "general"},
				{Kind: RowRoom, Label: "     #random", Style: "list_trusted", HubIdx: 0, Room: "random"},
			},
		},
		{
			name: "hub status glyphs and styles",
			hubs: []HubView{
				fakeHub{name: "conn", status: hubStatusConnected},
				fakeHub{name: "conn-ing", status: hubStatusConnecting},
				fakeHub{name: "failed", status: hubStatusFailed},
				fakeHub{name: "disconn", status: hubStatusDisconnected},
			},
			want: []HubListEntry{
				{Kind: RowHub, Label: "= conn", Style: "list_trusted", HubIdx: 0},
				{Kind: RowSpacer, HubIdx: 1},
				{Kind: RowHub, Label: "i conn-ing", Style: "list_unresponsive", HubIdx: 1},
				{Kind: RowSpacer, HubIdx: 2},
				{Kind: RowHub, Label: "X failed", Style: "list_untrusted", HubIdx: 2},
				{Kind: RowSpacer, HubIdx: 3},
				{Kind: RowHub, Label: "  disconn", Style: "list_unknown", HubIdx: 3},
			},
		},
		{
			name: "room markers and styles on connected hub",
			hubs: []HubView{
				fakeHub{
					name: "hub1", status: hubStatusConnected,
					joined:    []string{"joined"},
					messages:  []string{"onlymsg"},
					unread:    []string{"onlymsg"},
					mentioned: []string{"joined"},
				},
			},
			// Room order = sorted union of joined+messages = [joined, onlymsg].
			// "joined": mentioned → irc_mention, marker "!".
			// "onlymsg": unread (not mentioned) → list_unresponsive, marker "[!]".
			// Room row = "   "+marker+" #"+room → 3 spaces before a 1-char marker.
			want: []HubListEntry{
				{Kind: RowHub, Label: "= hub1", Style: "list_trusted", HubIdx: 0},
				{Kind: RowRoom, Label: "   ! #joined", Style: "irc_mention", HubIdx: 0, Room: "joined"},
				{Kind: RowRoom, Label: "   [!] #onlymsg", Style: "list_unresponsive", HubIdx: 0, Room: "onlymsg"},
			},
		},
		{
			name: "not-joined room and joined-room on disconnected hub",
			hubs: []HubView{
				fakeHub{
					name: "hub1", status: hubStatusDisconnected,
					joined:   []string{"joined"},
					messages: []string{"notjoined"}, // appears in list but not joined
				},
			},
			// "joined": joined but hub disconnected → list_unknown, marker " ".
			// "notjoined": not joined, not unread/mention → list_unknown, marker " ".
			// Disconnected hub header = " "+" "+name → 2 leading spaces.
			want: []HubListEntry{
				{Kind: RowHub, Label: "  hub1", Style: "list_unknown", HubIdx: 0},
				{Kind: RowRoom, Label: "     #joined", Style: "list_unknown", HubIdx: 0, Room: "joined"},
				{Kind: RowRoom, Label: "     #notjoined", Style: "list_unknown", HubIdx: 0, Room: "notjoined"},
			},
		},
		{
			name: "room union sorted and empty skipped",
			hubs: []HubView{
				fakeHub{
					name: "hub1", status: hubStatusConnected,
					joined:   []string{"random", "general"},
					messages: []string{"zzz", ""}, // empty message room must be skipped
				},
			},
			want: []HubListEntry{
				{Kind: RowHub, Label: "= hub1", Style: "list_trusted", HubIdx: 0},
				{Kind: RowRoom, Label: "     #general", Style: "list_trusted", HubIdx: 0, Room: "general"},
				{Kind: RowRoom, Label: "     #random", Style: "list_trusted", HubIdx: 0, Room: "random"},
				{Kind: RowRoom, Label: "     #zzz", Style: "list_unknown", HubIdx: 0, Room: "zzz"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := ComposeHubList(c.hubs, g)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ComposeHubList mismatch:\n got: %#v\n want: %#v", got, c.want)
			}
		})
	}
}

// TestHubListRowText pins the tview color-tag rendering of a HubListEntry.
// The unfocused foreground color is the entry's style palette color; spacer
// rows render as an empty string. Dark theme palette 3-hex colors are cube-
// quantized to the 256 palette even in truecolor (urwid _parse_color_true):
// list_trusted #6b2→#5faf00, list_unknown #bbb→#afafaf, list_unresponsive
// #b92→#af8700.
func TestHubListRowText(t *testing.T) {
	t.Parallel()
	colors := GetThemeColors(ThemeDark)

	cases := []struct {
		name  string
		entry HubListEntry
		want  string
	}{
		{"spacer", HubListEntry{Kind: RowSpacer, HubIdx: 1}, ""},
		{"hub trusted", HubListEntry{Kind: RowHub, Label: "= hub1", Style: "list_trusted"}, "[#5faf00]= hub1[-]"},
		{"room unknown", HubListEntry{Kind: RowRoom, Label: "     #general", Style: "list_unknown", Room: "general"}, "[#afafaf]     #general[-]"},
		{"room mention unresponsive", HubListEntry{Kind: RowRoom, Label: "   ! #joined", Style: "list_unresponsive"}, "[#af8700]   ! #joined[-]"},
		{"missing style falls back to plain", HubListEntry{Kind: RowHub, Label: "X failed", Style: "no_such_style"}, "X failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := HubListRowText(c.entry, colors); got != c.want {
				t.Errorf("HubListRowText = %q, want %q", got, c.want)
			}
		})
	}
}

// TestChannelsDisplaySetHubs pins SetHubs: the tview list is repopulated from
// ComposeHubList with one styled row per entry (hub header, spacer, room rows),
// matching Python _compose_list_widgets (Channels.py:1599-1662). The selection
// callbacks are dispatched per entry kind (hub vs room) via OnSelectHub /
// OnSelectRoom.
func TestChannelsDisplaySetHubs(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = glyphsPlain
	cd := NewChannelsDisplay(app, nil)

	hubs := []HubView{
		fakeHub{name: "hub1", status: hubStatusConnected, joined: []string{"general", "random"}},
		fakeHub{name: "hub2", status: hubStatusDisconnected},
	}
	cd.SetHubs(hubs)

	// Expected entries: hub1 header, 2 rooms, spacer, hub2 header = 5 rows.
	if got := cd.rooms.GetItemCount(); got != 5 {
		t.Fatalf("room count = %v, want 5", got)
	}
	// Rows render the label WITHOUT an embedded color tag: the palette color
	// is applied per-row via SetItemStyle instead, so tview's selected style
	// (dark text on the list_focus background) wins on the HIGHLIGHTED row.
	// The former tagged form overrode the selected colors, rendering the
	// selected row invisible (a disconnected hub's list_unknown #afafaf
	// foreground on the list_focus #afafaf background — the Pi's empty
	// highlighted hub line).
	wantTexts := []string{
		"= hub1",
		"     #general",
		"     #random",
		"",
		"  hub2",
	}
	for i, want := range wantTexts {
		main, _ := cd.rooms.GetItemText(i)
		if main != want {
			t.Errorf("row %v main = %q, want %q", i, main, want)
		}
	}

	// Selection dispatch: selecting a hub row fires OnSelectHub with the hub
	// index; selecting a room row fires OnSelectRoom with hub index + room.
	var gotHubIdx, gotRoom int
	var gotRoomName string
	cd.OnSelectHub = func(idx int) { gotHubIdx = idx }
	cd.OnSelectRoom = func(hubIdx int, room string) {
		gotRoom = hubIdx
		gotRoomName = room
	}
	cd.selectEntry(0) // hub1 header
	if gotHubIdx != 0 {
		t.Errorf("OnSelectHub hubIdx = %v, want 0", gotHubIdx)
	}
	cd.selectEntry(1) // general room
	if gotRoom != 0 || gotRoomName != "general" {
		t.Errorf("OnSelectRoom = (%v, %q), want (0, %q)", gotRoom, gotRoomName, "general")
	}
	cd.selectEntry(4) // hub2 header
	if gotHubIdx != 1 {
		t.Errorf("OnSelectHub hubIdx = %v, want 1", gotHubIdx)
	}
}

// The HIGHLIGHTED hub row must render its display name with the selected
// colors (dark text on the list_focus background) — the Pi's live bug was the
// selected hub line rendering EMPTY because the row's embedded color tag
// (a disconnected hub's list_unknown #afafaf foreground) overrode the
// selected colors, leaving #afafaf text on the #afafaf highlight.
func TestChannelsDisplaySelectedHubRowRendersName(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	cd := NewChannelsDisplay(app, nil)

	cd.SetHubs([]HubView{
		fakeHub{name: "hub1", status: hubStatusConnected, joined: []string{"general"}},
		fakeHub{name: "hub2", status: hubStatusDisconnected},
	})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	cd.widget.SetRect(0, 0, 80, 24)
	cd.widget.Draw(screen)
	screen.Sync()

	cells, width, _ := screen.GetContents()
	found := false
	for row := range 24 {
		var sb strings.Builder
		for col := 0; col < width && row*width+col < len(cells); col++ {
			if len(cells[row*width+col].Runes) > 0 {
				sb.WriteRune(cells[row*width+col].Runes[0])
			}
		}
		line := sb.String()
		if !strings.Contains(line, "hub2") {
			continue
		}
		found = true
		// The name's cells must NOT be #afafaf-on-#afafaf (invisible gray on
		// the gray highlight): the selected style's dark foreground wins.
		for col := range width {
			cell := cells[row*width+col]
			if len(cell.Runes) == 0 {
				continue
			}
			fg, bg, _ := cell.Style.Decompose()
			if cell.Runes[0] == '2' && uint32(fg) == 0xafafaf && uint32(bg) == 0xafafaf {
				t.Errorf("selected hub row renders #afafaf text on the #afafaf highlight (invisible): %q", line)
			}
		}
	}
	if !found {
		t.Fatal("the selected disconnected hub row did not render its display name")
	}
}

// C-n on the Channels page must open the New Hub dialog (the OnAddHub stub
// regression: the Pi's flow opened it, but the dialog mechanism needs a
// regression pin).
func TestChannelsDisplayNewHubDialogOpens(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.OnNewHub = func() { cd.NewHubDialog() }

	cd.OnNewHub()

	if cd.dialogOverlay == nil {
		t.Fatal("C-n did not open the New Hub dialog overlay")
	}
	dialog := cd.dialogOverlay.Dialog()
	if title := dialog.GetTitle(); title != "New Hub" {
		t.Errorf("dialog title = %q, want %q", title, "New Hub")
	}
	if cd.pages.GetPageCount() != 2 {
		t.Errorf("pages = %v, want the dialog page switched in", cd.pages.GetPageCount())
	}
}
