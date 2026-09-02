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
	"github.com/rivo/tview"
)

// dispatchToDialog feeds a key event through the open channels dialog overlay
// (SlotOverlay → DialogLineBox → focused content), the same path the running
// event loop uses once the overlay page is on top.
func dispatchToDialog(t *testing.T, cd *ChannelsDisplay, ev *tcell.EventKey) {
	t.Helper()
	if cd.dialogOverlay == nil {
		t.Fatal("no dialog overlay open")
	}
	h := cd.dialogOverlay.InputHandler()
	if h == nil {
		t.Fatal("dialog overlay has no input handler")
	}
	h(ev, func(p tview.Primitive) { cd.app.SetFocus(p) })
}

// dialogItems walks the open dialog's content Flex and returns its items.
func dialogItems(t *testing.T, cd *ChannelsDisplay) []tview.Primitive {
	t.Helper()
	dialog := cd.dialogOverlay.Dialog()
	flex, ok := dialog.Content().(*tview.Flex)
	if !ok {
		t.Fatalf("dialog content is %T, want *tview.Flex", dialog.Content())
	}
	var items []tview.Primitive
	for i := 0; i < flex.GetItemCount(); i++ {
		items = append(items, flex.GetItem(i))
	}
	return items
}

// dialogButton finds the button rendered in the dialog's button row.
func dialogButton(t *testing.T, cd *ChannelsDisplay, label string) *UrwidButton {
	t.Helper()
	for _, item := range dialogItems(t, cd) {
		if row, ok := item.(*urwidColumns); ok {
			for _, child := range row.children {
				if btn, ok := child.(*UrwidButton); ok && btn.Label() == label {
					return btn
				}
			}
		}
	}
	t.Fatalf("dialog has no %q button", label)
	return nil
}

// dialogInputField finds the dialog's input field, if any.
func dialogInputField(t *testing.T, cd *ChannelsDisplay) *tview.InputField {
	t.Helper()
	for _, item := range dialogItems(t, cd) {
		if f, ok := item.(*tview.InputField); ok {
			return f
		}
	}
	return nil
}

// dialogCheckBoxes finds the dialog's checkboxes in layout order.
func dialogCheckBoxes(t *testing.T, cd *ChannelsDisplay) []*UrwidCheckBox {
	t.Helper()
	var cbs []*UrwidCheckBox
	for _, item := range dialogItems(t, cd) {
		if cb, ok := item.(*UrwidCheckBox); ok {
			cbs = append(cbs, cb)
		}
	}
	return cbs
}

// dialogText joins every text widget's content in the dialog so tests can
// assert rendered prompt strings.
func dialogText(t *testing.T, cd *ChannelsDisplay) string {
	t.Helper()
	var sb strings.Builder
	for _, item := range dialogItems(t, cd) {
		switch v := item.(type) {
		case *tview.TextView:
			sb.WriteString(v.GetText(true))
			sb.WriteString("\n")
		case *urwidCenterText:
			sb.WriteString(v.text)
			sb.WriteString("\n")
		case *urwidLeftText:
			sb.WriteString(v.text)
			sb.WriteString("\n")
		case *centeredText:
			sb.WriteString(v.GetText())
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// TestChannelsRemoveSelectedDialogRoom pins the room branch of Python
// remove_selected_dialog (Channels.py:1882-1925): with a room row selected the
// dialog is titled "?" with the centered prompt
// "Leave and remove room\n#<room>\non hub <name>?\n", and Yes fires the
// remove-selected callback with (hubIdx, room) after closing the overlay.
func TestChannelsRemoveSelectedDialogRoom(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general", "random"}},
	})
	// Rows: 0 = hub header, 1 = #general, 2 = #random.
	cd.rooms.SetCurrentItem(1)

	var gotHubIdx = -1
	var gotRoom = "?"
	cd.OnRemoveSelected = func(hubIdx int, room string) { gotHubIdx, gotRoom = hubIdx, room }

	cd.RemoveSelectedDialog()

	if cd.dialogOverlay == nil {
		t.Fatal("RemoveSelectedDialog did not open the confirm overlay")
	}
	dialog := cd.dialogOverlay.Dialog()
	if title := dialog.GetTitle(); title != "?" {
		t.Errorf("dialog title = %q, want %q", title, "?")
	}
	text := dialogText(t, cd)
	for _, want := range []string{"Leave and remove room", "#general", "on hub Hub A?"} {
		if !strings.Contains(text, want) {
			t.Errorf("dialog text %q missing %q", text, want)
		}
	}

	yes := dialogButton(t, cd, "Yes")
	yes.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if gotHubIdx != 0 || gotRoom != "general" {
		t.Errorf("Yes = (%v, %q), want (0, %q)", gotHubIdx, gotRoom, "general")
	}
	if cd.dialogOverlay != nil {
		t.Error("Yes should close the dialog overlay")
	}
}

// TestChannelsRemoveSelectedDialogHub pins the hub branch of Python
// remove_selected_dialog: with a hub header row selected the prompt is
// "Remove hub\n<name>\nfrom this client?\n All Message history will be
// discarded.\n" and Yes fires (hubIdx, "").
func TestChannelsRemoveSelectedDialogHub(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{
		fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}},
	})
	// Row 0 is the hub header (the list defaults to item 0).
	var gotHubIdx = -1
	var gotRoom = "x"
	cd.OnRemoveSelected = func(hubIdx int, room string) { gotHubIdx, gotRoom = hubIdx, room }

	cd.RemoveSelectedDialog()

	if cd.dialogOverlay == nil {
		t.Fatal("RemoveSelectedDialog did not open the confirm overlay")
	}
	text := dialogText(t, cd)
	for _, want := range []string{"Remove hub", "Hub A", "from this client?", "All Message history will be discarded."} {
		if !strings.Contains(text, want) {
			t.Errorf("dialog text %q missing %q", text, want)
		}
	}

	yes := dialogButton(t, cd, "Yes")
	yes.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if gotHubIdx != 0 || gotRoom != "" {
		t.Errorf("Yes = (%v, %q), want (0, %q)", gotHubIdx, gotRoom, "")
	}
	if cd.dialogOverlay != nil {
		t.Error("Yes should close the dialog overlay")
	}
}

// TestChannelsRemoveSelectedDialogNoSelection pins Python's guard
// (Channels.py:1884-1886): with no hub-bearing row selected the dialog does
// not open and no callback fires.
func TestChannelsRemoveSelectedDialogNoSelection(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	fired := false
	cd.OnRemoveSelected = func(int, string) { fired = true }

	cd.RemoveSelectedDialog()

	if cd.dialogOverlay != nil {
		t.Error("RemoveSelectedDialog with no hubs should not open an overlay")
	}
	if fired {
		t.Error("OnRemoveSelected must not fire without a selection")
	}
}

// TestChannelsEditHubDialogLayout pins Python edit_hub_dialog's layout
// (Channels.py:2005-2055): title "Edit Hub", the " Address : " and
// " Server  : " lines (server falls back to "(unknown until connected)"),
// a divider, the "Display name : " input pre-filled with the hub's display
// name, and the three auto checkboxes with the hub's live states.
func TestChannelsEditHubDialogLayout(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{
		name:          "My Hub",
		addressHex:    "0123456789abcdef0123456789abcdef",
		serverName:    "",
		autoReconnect: true,
		autoList:      false,
		autoWho:       true,
	}})

	cd.EditHubDialog()

	if cd.dialogOverlay == nil {
		t.Fatal("EditHubDialog did not open the overlay")
	}
	dialog := cd.dialogOverlay.Dialog()
	if title := dialog.GetTitle(); title != "Edit Hub" {
		t.Errorf("dialog title = %q, want %q", title, "Edit Hub")
	}
	text := dialogText(t, cd)
	for _, want := range []string{
		"Address : 0123456789abcdef0123456789abcdef",
		"Server  : (unknown until connected)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dialog text %q missing %q", text, want)
		}
	}
	// The name input is pre-filled with the hub's display name (Python
	// e_name = ReadlineEdit(caption="Display name : ", edit_text=hub.name)).
	input := dialogInputField(t, cd)
	if input == nil {
		t.Fatal("edit hub dialog has no name input field")
	}
	if got := input.GetText(); got != "My Hub" {
		t.Errorf("name input = %q, want %q", got, "My Hub")
	}
	cbs := dialogCheckBoxes(t, cd)
	if len(cbs) != 3 {
		t.Fatalf("checkboxes = %v, want 3", len(cbs))
	}
	wantLabels := []string{
		"Auto-reconnect on disconnect",
		"Auto-fetch room list on connect",
		"Auto-fetch members on room join",
	}
	wantStates := []bool{true, false, true}
	for i, cb := range cbs {
		if cb.Label() != wantLabels[i] {
			t.Errorf("checkbox %v label = %q, want %q", i, cb.Label(), wantLabels[i])
		}
		if cb.IsChecked() != wantStates[i] {
			t.Errorf("checkbox %v (%v) checked = %v, want %v", i, cb.Label(), cb.IsChecked(), wantStates[i])
		}
	}
}

// TestChannelsEditHubDialogSave pins Python confirmed()'s value collection
// (Channels.py:2023-2037): the Save button fires the submit callback with the
// edited (or existing) display name and the three checkbox states, after
// closing the overlay. A blank name falls back to the hub's existing name.
func TestChannelsEditHubDialogSave(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{
		name:          "My Hub",
		addressHex:    "0123456789abcdef0123456789abcdef",
		autoReconnect: false,
		autoList:      false,
		autoWho:       false,
	}})

	cd.EditHubDialog()

	var gotHubIdx = -1
	var gotName = "?"
	var gotReconnect, gotList, gotWho = false, false, false
	saved := false
	cd.OnEditHubSubmitted = func(hubIdx int, name string, ar, al, aw bool) {
		saved = true
		gotHubIdx, gotName, gotReconnect, gotList, gotWho = hubIdx, name, ar, al, aw
	}

	// Toggle all three checkboxes on (Space toggles, urwid CheckBox.keypress).
	for _, cb := range dialogCheckBoxes(t, cd) {
		cb.InputHandler()(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), func(p tview.Primitive) {})
	}
	// Clear the name field to exercise the fall-back branch, then submit via
	// the field's Enter (Python confirmed() runs on the Save press; the field's
	// done-func submits the same values).
	input := dialogInputField(t, cd)
	if input == nil {
		t.Fatal("edit hub dialog has no name input field")
	}
	input.SetText("")

	yes := dialogButton(t, cd, "Save")
	yes.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})

	if !saved {
		t.Fatal("Save did not fire OnEditHubSubmitted")
	}
	if gotHubIdx != 0 {
		t.Errorf("hubIdx = %v, want 0", gotHubIdx)
	}
	// Python: nm = e_name.get_edit_text().strip() or hub.name — an empty edit
	// falls back to the existing display name.
	if gotName != "My Hub" {
		t.Errorf("name = %q, want the existing %q", gotName, "My Hub")
	}
	if !gotReconnect || !gotList || !gotWho {
		t.Errorf("checkbox states = (%v,%v,%v), want all true", gotReconnect, gotList, gotWho)
	}
	if cd.dialogOverlay != nil {
		t.Error("Save should close the dialog overlay")
	}
}

// TestChannelsEditHubDialogNoSelection pins Python's guard: with no hub row
// selected (spacer/none) the dialog does not open.
func TestChannelsEditHubDialogNoSelection(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	fired := false
	cd.OnEditHubSubmitted = func(int, string, bool, bool, bool) { fired = true }

	cd.EditHubDialog()

	if cd.dialogOverlay != nil {
		t.Error("EditHubDialog with no hubs should not open an overlay")
	}
	if fired {
		t.Error("OnEditHubSubmitted must not fire without a selection")
	}
}

// TestChannelsCtrlYGuardPinnedOnPlaceholder pins Python toggle_channel_list's
// guard (Channels.py:1531-1535): while the channel list is visible and the
// right pane still shows the placeholder, Ctrl-Y does NOT collapse the list.
func TestChannelsCtrlYGuardPinnedOnPlaceholder(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)

	if !cd.ChannelListVisible() {
		t.Fatal("channel list should start visible")
	}
	cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone))
	if !cd.ChannelListVisible() {
		t.Error("Ctrl-Y with the placeholder showing must keep the list visible (Python toggle_channel_list guard)")
	}

	// With a room open the toggle applies.
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)
	cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone))
	if cd.ChannelListVisible() {
		t.Error("Ctrl-Y with a room open should collapse the channel list")
	}
	cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone))
	if !cd.ChannelListVisible() {
		t.Error("second Ctrl-Y should restore the channel list")
	}
}

// TestChannelsToggleCollapseRerendersRoom pins Python
// toggle_join_part_collapse (Channels.py:1537-1543): F8 flips the collapse
// flag and re-renders the OPEN room's messages with the new state.
func TestChannelsToggleCollapseRerendersRoom(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	msgs := []ChannelMessage{
		{Nick: "alice", Text: "hello", IsSelf: true},
		{Text: "alice joined", IsSystem: true},
		{Text: "alice left", IsSystem: true},
	}
	cd.ShowRoom(0, "general", msgs)

	cd.handleInput(tcell.NewEventKey(tcell.KeyF8, 0, tcell.ModNone))
	if !cd.CollapseJoinPart() {
		t.Fatal("F8 should set CollapseJoinPart")
	}
	if !cd.roomWidget.CollapseJoinPart() {
		t.Error("the open room widget should mirror the collapse flag after F8")
	}
	roomRows := strings.Join(renderPrimitive(t, cd.roomWidget.Widget(), 80, 24), "\n")
	if strings.Contains(roomRows, "alice joined") || strings.Contains(roomRows, "alice left") {
		t.Error("collapsed room view still renders join/leave lines")
	}
	if !strings.Contains(roomRows, "2 join/leave events") {
		t.Error("collapsed room view missing the collapsed summary label")
	}
	if !strings.Contains(roomRows, "hello") {
		t.Error("collapsed room view lost normal messages")
	}

	cd.handleInput(tcell.NewEventKey(tcell.KeyF8, 0, tcell.ModNone))
	if cd.CollapseJoinPart() {
		t.Fatal("second F8 should clear CollapseJoinPart")
	}
	if cd.roomWidget.CollapseJoinPart() {
		t.Error("the open room widget should un-collapse after the second F8")
	}
}

// TestChannelsMemberClickFiresUserInfo pins the member-row activation path:
// activating a user in the open room's users pane fires the display's
// OnMemberClick with the member's nick and identity hash (Python connects
// each ChannelListEntry's click signal to display.show_user_info with the
// peer hash, Channels.py:713).
func TestChannelsMemberClickFiresUserInfo(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{fakeHub{name: "Hub A", status: hubStatusConnected, joined: []string{"general"}}})
	cd.ShowRoom(0, "general", nil)
	cd.roomWidget.SetMembers([]ChannelMember{
		{Nick: "alice", Hash: "aa11", Online: true},
	})

	var gotNick, gotHash string
	fired := false
	cd.OnMemberClick = func(nick, hash string) { fired = true; gotNick, gotHash = nick, hash }

	cd.roomWidget.ActivateMember(0)

	if !fired {
		t.Fatal("activating a member row did not fire OnMemberClick")
	}
	if gotNick != "alice" || gotHash != "aa11" {
		t.Errorf("OnMemberClick = (%q, %q), want (%q, %q)", gotNick, gotHash, "alice", "aa11")
	}
}
