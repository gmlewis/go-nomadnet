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
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/rrc"
	"github.com/rivo/tview"
)

// ChannelMessage holds a single chat message.
type ChannelMessage struct {
	Room      string
	Nick      string
	Text      string
	Timestamp string
	IsSelf    bool
	IsSystem  bool
	IsNotice  bool
	IsError   bool
	Mention   bool
}

// ChannelMember holds a room member.
type ChannelMember struct {
	Nick   string
	Hash   string
	Online bool
}

// ChannelInfo holds room information.
type ChannelInfo struct {
	Name    string
	Topic   string
	Members int
	Unread  bool
	Joined  bool
}

// channelsListInnerWidth is the inner width of the bordered "Channels" list
// pane: given_list_width (36, Channels.py:1437) minus the 2 LineBox border
// columns. The no-hubs empty-state text is pre-wrapped to this width.
const channelsListInnerWidth = 34

// ChannelsDisplay shows the RRC chat interface.
type ChannelsDisplay struct {
	app                *App
	widget             tview.Primitive
	content            *tview.Flex
	leftPanel          *tview.Flex
	rightPane          *tview.Flex
	placeholder        *tview.TextView
	ilb                *IndicativeListBox
	noHubsText         *tview.TextView
	chanGutter         tview.Primitive
	leftWidth          int
	rooms              *tview.List
	messages           *tview.TextView
	members            *tview.List
	input              *ReadlineEdit
	channelListVisible bool
	collapseJoinPart   bool
	shortcutFocus      string
	// pages wraps cd.content so a dialog can overlay the WHOLE channels display
	// (Python's self.widget = WidgetPlaceholder(columns_widget) + _show_dialog_
	// overlay setting original_widget = urwid.Overlay(dialog, columns_widget,
	// width=(RELATIVE,60), min_width=40, height=PACK), Channels.py:2196-2210).
	// "main" = cd.content; "dialog" = the SlotOverlay. dialogOverlay tracks the
	// active overlay for closeDialog.
	pages         *tview.Pages
	dialogOverlay *SlotOverlay

	// hubEntries is the last ComposeHubList output rendered by SetHubs, indexed
	// 1:1 with the rooms list. selectEntry uses it to dispatch hub/room
	// selection. Mirrors Python's list_widgets (Channels.py:1599-1662).
	hubEntries []HubListEntry

	// Selection callbacks, mirroring Python's _select_hub / _select_room
	// (Channels.py:1672-1729): selecting a hub header opens/activates the hub;
	// selecting a room opens the room.
	OnSelectHub  func(hubIdx int)
	OnSelectRoom func(hubIdx int, room string)

	// Keyboard shortcut callbacks (Python: ChannelsListArea.keypress, RoomFrame.keypress)
	OnNewHub func()
	// OnAddHub adds the hub to the app's RRC manager and persists it (Python
	// rrc.add_hub, RRC.py:389-401), then the wiring layer refreshes the hub
	// list. nil disables the New Hub dialog's add path.
	OnAddHub              func(hubHash []byte, destName, name string)
	OnJoinRoom            func()
	OnConnect             func()
	OnDisconnect          func()
	OnToggleAutoReconnect func()
	OnEditHub             func()
	OnRemoveHub           func()
	OnToggleChannelList   func()
	OnSendMessage         func()
	OnLeaveRoom           func()
	OnToggleCollapse      func()
	OnMemberClick         func(nick, hash string)
}

// SetShortcutFocus sets which of the three Channels shortcut bars
// GetShortcutText returns, matching Python's Channels.py:217-229 (three
// regions: list / editor / body).
func (cd *ChannelsDisplay) SetShortcutFocus(region string) {
	cd.shortcutFocus = region
}

// setShortcutRegion records the active focus region and refreshes the main
// display's shortcut bar so the footer text tracks the focused pane. It is
// wired as the SetFocusFunc of channels list / room editor / room body.
func (cd *ChannelsDisplay) setShortcutRegion(region string) {
	cd.shortcutFocus = region
	if cd.app != nil && cd.app.Main != nil {
		cd.app.Main.refreshShortcuts()
	}
}

// GetShortcutText returns the appropriate shortcut bar text for the current
// focus context. Matches Python's Channels.py:217-229, whose shortcuts() always
// returns one of the three bars — an open overlay does NOT blank the footer
// (Python's shortcuts() reads the focus path with no dialog suppression), so
// the Ctrl-key menu stays visible whenever the display is shown.
func (cd *ChannelsDisplay) GetShortcutText() string {
	switch cd.shortcutFocus {
	case "editor":
		return "[C-d] Send  [C-x] Leave  [F8] Collapse  [Tab] Complete Nick"
	case "body":
		return "[C-x] Leave  [C-u] Users  [C-y] Channels  [F8] Collapse Joins  [Tab] ↓ Editor"
	default: // "list"
		return "[C-n] New Hub  [C-a] Add Room  [C-r] Connect  [C-w] Disconnect  [C-t] Auto-reconnect  [C-e] Edit Hub  [C-x] Remove"
	}
}

// NewChannelsDisplay creates a new channels display.
func NewChannelsDisplay(app *App, rooms []ChannelInfo) *ChannelsDisplay {
	cd := &ChannelsDisplay{
		app:                app,
		channelListVisible: true,
		leftWidth:          36, // given_list_width (Channels.py:1437)
	}

	// Hub/room list (left pane), wrapped in an IndicativeListBox so the
	// centered "───"/"▲"/"▼" scroll indicators render above and below it
	// (Python IndicativeListBox, Channels.py:1590).
	cd.rooms = tview.NewList()
	cd.rooms.SetHighlightFullLine(true)
	ApplyListFocusStyle(cd.rooms, app.Theme)
	cd.rooms.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		cd.selectEntry(i)
	})
	cd.ilb = NewIndicativeListBox(cd.rooms)

	// No-hubs empty state (Python _compose_list_widgets, Channels.py:1603-1607):
	// a single left-aligned "No hubs yet. Press Ctrl-N to add one." Text in the
	// list_unknown color with a leading blank line, wrapped inside the list area
	// between the indicator bars. Pre-wrapped with urwidSpaceWrap at the fixed
	// inner width (given_list_width 36 − 2 borders = 34) so the break lands where
	// urwid breaks (after "add"), not where tview's WordWrap breaks (after "to").
	colors := GetThemeColors(app.Theme)
	// Python's Channels ILB repaints the selected hub row with list_off_focus
	// while the list pane is unfocused (highlight_offFocus, Channels.py:1594).
	cd.ilb.SetHighlightStyles(colors["list_focus_fg"], colors["list_focus_bg"],
		colors["list_off_focus_fg"], colors["list_off_focus_bg"])
	const noHubsRaw = "\n  No hubs yet. Press Ctrl-N to add one."
	cd.noHubsText = tview.NewTextView().
		SetTextColor(colors["list_unknown"]).
		SetTextAlign(tview.AlignLeft).
		SetWrap(true).
		SetWordWrap(false)
	cd.noHubsText.SetText(strings.Join(urwidSpaceWrap(noHubsRaw, channelsListInnerWidth), "\n"))
	cd.ilb.SetEmptyWidget(cd.noHubsText)
	cd.populateRooms(rooms)

	// Left pane: bordered "Channels" LineBox wrapping the IndicativeListBox
	// (Python ChannelsListArea, Channels.py:342/1596).
	cd.leftPanel = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.ilb, 0, 1, true)
	cd.leftPanel.SetBorder(true)
	SetTitledBorder(cd.leftPanel, "Channels")

	// Right pane placeholder (Python Channels.py:1459): a bordered, untitled
	// LineBox wrapping a top-filled, centered "Select or add a hub to begin"
	// with a leading blank line. A TextView is top-aligned by default, matching
	// urwid Filler("top"); AlignCenter handles the horizontal centering.
	cd.placeholder = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetWrap(false)
	cd.placeholder.SetText("\n  Select or add a hub to begin")
	cd.rightPane = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.placeholder, 0, 1, false)
	cd.rightPane.SetBorder(true)

	// Message view, members list and compose editor are created up front so the
	// Show* methods are safe to call before a room is opened; they are not part
	// of the boot layout (the right pane shows the placeholder until a room is
	// selected, RRC).
	cd.messages = applyWheelMultiplier(tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		// Python's channel message list is a bare _StickyMessageListBox with
		// no AttrMap (Channels.py:784); message widgets carry their own
		// styling. The base is the terminal default, not #bbbbbb.
		SetTextColor(tcell.ColorDefault))
	cd.members = tview.NewList()
	cd.members.SetHighlightFullLine(true)
	ApplyListFocusStyle(cd.members, app.Theme)
	cd.members.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		if cd.OnMemberClick != nil {
			cd.OnMemberClick(mainText, secondaryText)
		}
	})
	// Compose editor — Python wraps it in `AttrMap(editor, "msg_editor")`
	// (Channels.py:609); msg_editor is 3-hex #111/#0bb (ui/TextUI.py:32/85),
	// cube-quantized to #000000/#00afaf. Route through the palette rather than
	// the prior 0x222222/0xdddddd (which is not Python's msg_editor).
	tc := GetThemeColors(app.Theme)
	cd.input = NewReadlineEdit(app.killRing, "", "Type a message...")
	cd.input.SetFieldBackgroundColor(tc["msg_editor_bg"])
	cd.input.SetFieldTextColor(tc["msg_editor_fg"])
	cd.input.SetFocusFunc(func() { cd.setShortcutRegion("editor") })

	// Wire focus-region shortcut bars for list and body.
	cd.ilb.SetFocusFunc(func() { cd.setShortcutRegion("list") })
	cd.messages.SetFocusFunc(func() { cd.setShortcutRegion("body") })

	// Two-pane Columns: left list (given 36) + right pane (weight 1). No outer
	// border — each pane carries its own (Python columns_widget, Channels.py:
	// 1462-1468). The expand gutter is shown only when the list is hidden.
	cd.chanGutter = NewChannelsExpandGutter(func() { cd.ToggleChannelListState() })
	cd.content = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(cd.leftPanel, cd.leftWidth, 0, true).
		AddItem(cd.rightPane, 0, 1, false)
	cd.content.SetInputCapture(cd.handleInput)
	cd.pages = tview.NewPages().AddPage("main", cd.content, true, true)
	cd.widget = cd.pages

	return cd
}

// showDialogOverlay overlays a DialogLineBox on the whole channels display
// (Python's _show_dialog_overlay, Channels.py:2196-2210: urwid.Overlay(dialog,
// columns_widget, align=CENTER, width=(RELATIVE,60), min_width=40,
// valign=MIDDLE, height=PACK)). The display shows through around the 60%-width
// dialog. Esc/confirm dismisses via closeDialog, restoring the display.
// dialogHeight is the dialog's PACK height.
func (cd *ChannelsDisplay) showDialogOverlay(dialog *DialogLineBox, dialogHeight int) {
	if cd.dialogOverlay != nil {
		cd.closeDialog()
	}
	ov := NewSlotOverlay(cd.content, dialog, 60, dialogHeight)
	// Keep the dialog's own onDismiss when set (every channels dialog
	// constructor passes a dismiss closure that closes the overlay). The Esc
	// key is dispatched THROUGH this DialogLineBox before any inner nav item,
	// so a bare overwrite here would discard the constructor's dismiss.
	if dialog.onDismiss == nil {
		dialog.onDismiss = cd.closeDialog
	}
	cd.dialogOverlay = ov
	cd.pages.AddPage("dialog", ov, true, true)
	cd.pages.SwitchToPage("dialog")
	if cd.app != nil {
		cd.app.SetFocus(ov)
	}
}

// closeDialog restores the channels display after a showDialogOverlay.
func (cd *ChannelsDisplay) closeDialog() {
	if cd.dialogOverlay == nil {
		return
	}
	cd.pages.RemovePage("dialog")
	cd.dialogOverlay = nil
	cd.pages.SwitchToPage("main")
	if cd.app != nil {
		cd.app.SetFocus(cd.content)
	}
}

// showDialogOverlayInput shows an input dialog overlaid on the channels display
// (Python's _show_dialog_overlay, 60% width, PACK). Enter on the field or the
// confirm button submits; Esc/Cancel dismisses.
func (cd *ChannelsDisplay) showDialogOverlayInput(title, label, defaultValue, confirmLabel, cancelLabel string, onSubmit func(string), onCancel func()) {
	input := tview.NewInputField()
	input.SetLabel(label)
	input.SetText(defaultValue)
	input.SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetFieldTextColor(tcell.ColorDefault)
	close := cd.closeDialog
	submit := func() {
		v := strings.TrimSpace(input.GetText())
		close()
		if onSubmit != nil {
			onSubmit(v)
		}
	}
	confirmBtn := NewUrwidButton(confirmLabel).SetSelectedFunc(submit)
	cancelBtn := NewUrwidButton(cancelLabel).SetSelectedFunc(func() {
		close()
		if onCancel != nil {
			onCancel()
		}
	})
	row := CreateUrwidButtonRow(confirmBtn, cancelBtn)
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			submit()
		}
	})
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(row, 1, 0, false)
	dialog := NewDialogLineBox(title, layout, close)
	cd.showDialogOverlay(dialog, 4) // input 1 + button row 1 + 2 border
	wireDialogNav(cd.app, close, []tview.Primitive{input, confirmBtn, cancelBtn})
}

// showDialogOverlayConfirm shows a Yes/No confirm overlaid on the channels
// display (Python's _show_dialog_overlay, 60% width, PACK).
func (cd *ChannelsDisplay) showDialogOverlayConfirm(message string, onYes, onNo func()) {
	close := cd.closeDialog
	yes := NewUrwidButton("Yes").SetSelectedFunc(func() {
		close()
		if onYes != nil {
			onYes()
		}
	})
	no := NewUrwidButton("No").SetSelectedFunc(func() {
		close()
		if onNo != nil {
			onNo()
		}
	})
	row := CreateUrwidButtonRow(yes, no)
	msgRows := strings.Count(message, "\n") + 1
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(message), msgRows, 0, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("Confirm", layout, close)
	cd.showDialogOverlay(dialog, msgRows+1+2)
	wireDialogNav(cd.app, close, []tview.Primitive{yes, no})
}

// populateRooms fills the room list from the given channel infos.
func (cd *ChannelsDisplay) populateRooms(rooms []ChannelInfo) {
	cd.rooms.Clear()
	for _, room := range rooms {
		prefix := "  "
		if room.Unread {
			prefix = "[!] "
		}
		if room.Joined {
			prefix = "[*] "
		}
		text := fmt.Sprintf("%v#%v", prefix, room.Name)
		secondary := fmt.Sprintf("%v members — %v", room.Members, room.Topic)
		cd.rooms.AddItem(text, secondary, 0, nil)
	}
}

// SetHubs repopulates the channels hub/room list from the given hubs, mirroring
// Python's _compose_list_widgets (Channels.py:1599-1662). Each hub becomes a
// status-glyph + name header row; the sorted union of its joined and
// message-bearing rooms becomes "   <marker> #<room>" rows; a blank spacer row
// separates consecutive hubs. Rows carry their style's palette color as a
// tview color tag so the unfocused list matches Python's per-row AttrMap
// coloring. The empty (no-hubs) state is rendered by the list's empty widget.
func (cd *ChannelsDisplay) SetHubs(hubs []HubView) {
	colors := GetThemeColors(cd.app.Theme)
	glyphs := cd.app.Glyphs
	entries := ComposeHubList(hubs, glyphs)
	cd.hubEntries = entries
	cd.rooms.Clear()
	for _, e := range entries {
		cd.rooms.AddItem(HubListRowText(e, colors), "", 0, nil)
	}
}

// selectEntry dispatches a list-row selection to the hub/room selection
// callback for the entry at the given index, mirroring Python's _select_hub /
// _select_room (Channels.py:1672-1729). Spacer rows are ignored.
func (cd *ChannelsDisplay) selectEntry(idx int) {
	if idx < 0 || idx >= len(cd.hubEntries) {
		return
	}
	e := cd.hubEntries[idx]
	switch e.Kind {
	case RowHub:
		if cd.OnSelectHub != nil {
			cd.OnSelectHub(e.HubIdx)
		}
	case RowRoom:
		if cd.OnSelectRoom != nil {
			cd.OnSelectRoom(e.HubIdx, e.Room)
		}
	}
}

// handleInput processes keyboard shortcuts for the channels display.
// Matches Python's ChannelsListArea.keypress() at Channels.py:352
// and RoomFrame.keypress() at Channels.py:522.
func (cd *ChannelsDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlN:
		if cd.OnNewHub != nil {
			cd.OnNewHub()
		}
		return nil
	case tcell.KeyCtrlA:
		if cd.OnJoinRoom != nil {
			cd.OnJoinRoom()
		}
		return nil
	case tcell.KeyCtrlR:
		if cd.OnConnect != nil {
			cd.OnConnect()
		}
		return nil
	case tcell.KeyCtrlW:
		if cd.OnDisconnect != nil {
			cd.OnDisconnect()
		}
		return nil
	case tcell.KeyCtrlT:
		if cd.OnToggleAutoReconnect != nil {
			cd.OnToggleAutoReconnect()
		}
		return nil
	case tcell.KeyCtrlE:
		if cd.OnEditHub != nil {
			cd.OnEditHub()
		}
		return nil
	case tcell.KeyCtrlX:
		if cd.OnRemoveHub != nil {
			cd.OnRemoveHub()
		}
		return nil
	case tcell.KeyCtrlY:
		cd.ToggleChannelListState()
		if cd.OnToggleChannelList != nil {
			cd.OnToggleChannelList()
		}
		return nil
	case tcell.KeyCtrlD:
		if cd.OnSendMessage != nil {
			cd.OnSendMessage()
		}
		return nil
	case tcell.KeyF8:
		cd.ToggleCollapse()
		return nil
	case tcell.KeyTab:
		// Python ChannelsListArea.keypress "tab" moves focus to the menubar
		// (frame.focus_position = "header", Channels.py:374-375). Gated to the
		// list region: with the room editor/body focused, Tab belongs to the
		// room (nick completion / ↓ editor), matching Python where
		// ChannelsListArea only sees keys while the list pane has focus.
		if cd.shortcutFocus == "list" && cd.app != nil && cd.app.Main != nil {
			cd.app.Main.FocusMenu()
			return nil
		}
	}

	return event
}

// Widget returns the tview primitive for this display.
func (cd *ChannelsDisplay) Widget() tview.Primitive {
	return cd.widget
}

// UpdateRooms refreshes the room list.
func (cd *ChannelsDisplay) UpdateRooms(rooms []ChannelInfo) {
	cd.rooms.Clear()
	for _, room := range rooms {
		prefix := "  "
		if room.Unread {
			prefix = "[!] "
		}
		if room.Joined {
			prefix = "[*] "
		}
		text := fmt.Sprintf("%v#%v", prefix, room.Name)
		secondary := fmt.Sprintf("%v members — %v", room.Members, room.Topic)
		cd.rooms.AddItem(text, secondary, 0, nil)
	}
}

// ToggleChannelListVisibility shows/hides the channel list panel.
func (cd *ChannelsDisplay) ToggleChannelListVisibility() {
	if cd.OnToggleChannelList != nil {
		cd.OnToggleChannelList()
	}
}

// ChannelListVisible reports whether the channel list panel is visible.
// Matches Python's channel_list_visible at Channels.py:1531.
func (cd *ChannelsDisplay) ChannelListVisible() bool {
	return cd.channelListVisible
}

// ToggleChannelListState toggles the channel list visibility state, rebuilding
// the content Flex so the pane actually hides. Matches Python's
// _apply_channel_list_visibility (Channels.py:1545-1568): [left(36), right(1)]
// when visible; [gutter(1), right(1)] when hidden (show_gutters defaults True).
func (cd *ChannelsDisplay) ToggleChannelListState() {
	cd.channelListVisible = !cd.channelListVisible
	if cd.content == nil {
		return
	}
	cd.content.Clear()
	if cd.channelListVisible {
		cd.content.AddItem(cd.leftPanel, cd.leftWidth, 0, true)
		cd.content.AddItem(cd.rightPane, 0, 1, false)
	} else {
		cd.content.AddItem(cd.chanGutter, 1, 0, false)
		cd.content.AddItem(cd.rightPane, 0, 1, false)
	}
	// No QueueUpdateDraw: this runs on the event loop (input handler or gutter
	// callback), which redraws automatically afterwards, and QueueUpdateDraw
	// would deadlock both there and in tests with no running event loop.
}

// SetMessages replaces the messages view content.
func (cd *ChannelsDisplay) SetMessages(text string) {
	cd.messages.SetText(text)
}

// ToggleCollapse flips the join/leave collapse flag, matching Python's
// toggle_join_part_collapse (Channels.py:1537), then fires OnToggleCollapse.
func (cd *ChannelsDisplay) ToggleCollapse() {
	cd.collapseJoinPart = !cd.collapseJoinPart
	if cd.OnToggleCollapse != nil {
		cd.OnToggleCollapse()
	}
}

// CollapseJoinPart reports whether join/leave system messages are collapsed.
func (cd *ChannelsDisplay) CollapseJoinPart() bool {
	return cd.collapseJoinPart
}

// ShowUserInfo displays a user info dialog for the selected member.
// Matches Python's ChannelsDisplay.show_user_info() at Channels.py:2119.
func (cd *ChannelsDisplay) ShowUserInfo(nick, hash string) {
	info := fmt.Sprintf("[::b]Nick[-]  : %v\n[::b]Hash[-] : %v", nick, hash)
	cd.messages.SetText(info)
}

// ShowMessages displays messages for a room.
func (cd *ChannelsDisplay) ShowMessages(msgs []ChannelMessage) {
	if cd.collapseJoinPart {
		msgs = CollapseJoinPartMessages(msgs)
	}
	var sb strings.Builder
	for _, msg := range msgs {
		switch {
		case msg.IsSystem:
			fmt.Fprintf(&sb, "[gray]%v[-]\n", msg.Text)
		case msg.IsNotice:
			fmt.Fprintf(&sb, "[yellow]%v[-]\n", msg.Text)
		case msg.IsError:
			fmt.Fprintf(&sb, "[red]%v[-]\n", msg.Text)
		case msg.IsSelf:
			fmt.Fprintf(&sb, "[green]%v[-] %v\n", msg.Nick, msg.Text)
		default:
			color := nickColor(msg.Nick)
			fmt.Fprintf(&sb, "[%v]%v[-] %v\n", color, msg.Nick, msg.Text)
		}
	}
	cd.messages.SetText(sb.String())
}

// ShowMembers displays the member list for a room.
func (cd *ChannelsDisplay) ShowMembers(members []ChannelMember) {
	cd.members.Clear()
	for _, m := range members {
		status := "○"
		if m.Online {
			status = "●"
		}
		cd.members.AddItem(fmt.Sprintf("%v %v", status, m.Nick), m.Hash[:12], 0, nil)
	}
}

// nickColor returns a consistent 24-color palette color for a nickname.
func nickColor(nick string) string {
	return NickColor(nick, ThemeDark)
}

// FormatMessage produces a styled message string for display in the
// message list. Returns a tview-compatible formatted string with
// appropriate color tags for the message type.
// Matches Python's message rendering with header, trust styling,
// and type-based coloring.
func FormatMessage(msg ChannelMessage, theme int) string {
	switch {
	case msg.IsSystem:
		return fmt.Sprintf("[gray]system:%v[-]", msg.Text)
	case msg.IsNotice:
		return fmt.Sprintf("[yellow]notice:%v[-]", msg.Text)
	case msg.IsError:
		return fmt.Sprintf("[red]error:%v[-]", msg.Text)
	case msg.IsSelf:
		return fmt.Sprintf("[green]%v[-] %v", msg.Nick, msg.Text)
	default:
		// Check for mention indicator
		extra := ""
		if msg.Mention {
			extra = " [orange]@mention[-]"
		}
		return fmt.Sprintf("[%v]%v[-] %v%v", nickColor(msg.Nick), msg.Nick, msg.Text, extra)
	}
}

// ShowUserInfoDialog displays user information for a channel member.
// Matches Python's ChannelsDisplay.show_user_info() at
// Channels.py:2119-2155.
func (cd *ChannelsDisplay) ShowUserInfoDialog(nick, identityHash string, isSelf bool, onOpenConversation func()) {
	var sb strings.Builder
	sb.WriteString("\n")
	fmt.Fprintf(&sb, " Nick     : %v\n", nick)
	fmt.Fprintf(&sb, " Identity : %v\n", identityHash)

	if isSelf {
		sb.WriteString("\n (This is you)\n")
	}
	text := sb.String()
	textRows := strings.Count(text, "\n") + 1

	close := cd.closeDialog
	var row *urwidColumns
	if isSelf {
		row = CreateUrwidButtonRow(NewUrwidButton("Close").SetSelectedFunc(close))
	} else {
		openBtn := NewUrwidButton("Open Conversation").SetSelectedFunc(func() {
			close()
			if onOpenConversation != nil {
				onOpenConversation()
			}
		})
		closeBtn := NewUrwidButton("Close").SetSelectedFunc(close)
		row = CreateUrwidButtonRow(openBtn, closeBtn)
	}

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.ColorDefault).
		SetText(text)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(info, textRows, 0, false).
		AddItem(row, 1, 0, true)

	dialog := NewDialogLineBox("User Info", layout, close)
	cd.showDialogOverlay(dialog, textRows+1+2)
}

// MaybeAutoconnect connects hub if it is disconnected or in a failed state,
// matching Python's ChannelsDisplay._maybe_autoconnect (Channels.py:1736-1741).
func MaybeAutoconnect(hub *rrc.RRCHub) {
	if hub == nil {
		return
	}
	if hub.Status == rrc.StatusDisconnected || hub.Status == rrc.StatusFailed {
		hub.ConnectAsync()
	}
}

// SelectedEntry returns the currently selected HubListEntry (if any).
func (cd *ChannelsDisplay) SelectedEntry() (HubListEntry, bool) {
	if cd.rooms == nil || len(cd.hubEntries) == 0 {
		return HubListEntry{}, false
	}
	idx := cd.rooms.GetCurrentItem()
	if idx < 0 || idx >= len(cd.hubEntries) {
		return HubListEntry{}, false
	}
	return cd.hubEntries[idx], true
}

// NewHubDialog shows the dialog to add a new RRC hub (Python Channels.new_hub_
// dialog), overlaid on the channels display (60% width).
func (cd *ChannelsDisplay) NewHubDialog() {
	if cd.app == nil {
		return
	}
	cd.showDialogOverlayInput("New Hub", "Hub address (hex hash):", "", "Add", "Back", func(hashText string) {
		hashText = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(hashText), "0x"))
		hashBytes, err := hex.DecodeString(hashText)
		if err != nil || len(hashBytes) != 16 {
			return
		}
		// Search for hub name or default
		cd.showDialogOverlayInput("New Hub Name", "Display name:", "", "Add", "Back", func(nameText string) {
			nameText = strings.TrimSpace(nameText)
			// Python new_hub_dialog confirmed(): rrc.add_hub(hh, name=nm) then
			// update_list (Channels.py:1046-1060 in the installed 1.2.8). The
			// former stub silently discarded both fields, so New Hub did
			// nothing at all.
			if cd.OnAddHub == nil {
				return
			}
			cd.OnAddHub(hashBytes, "rrc.hub", nameText)
		}, nil)
	}, nil)
}

// JoinRoomDialog shows the dialog to join a room on the selected hub (Python
// Channels.join_room_dialog), overlaid on the channels display (60% width).
func (cd *ChannelsDisplay) JoinRoomDialog() {
	if cd.app == nil {
		return
	}
	cd.showDialogOverlayInput("Join Room", "Room name:", "", "Join", "Cancel", func(roomText string) {
		roomText = strings.TrimSpace(strings.TrimPrefix(roomText, "#"))
		if roomText == "" {
			return
		}
	}, nil)
}

// RemoveSelectedDialog shows confirmation dialog to remove selected hub/room
// (Python Channels.remove_selected_dialog), overlaid on the channels display.
func (cd *ChannelsDisplay) RemoveSelectedDialog() {
	if cd.app == nil {
		return
	}
	cd.showDialogOverlayConfirm("Remove selected hub/room?", func() {
	}, nil)
}

// EditHubDialog shows dialog to edit the display name of selected hub (Python
// Channels.edit_hub_dialog), overlaid on the channels display (60% width).
func (cd *ChannelsDisplay) EditHubDialog() {
	if cd.app == nil {
		return
	}
	cd.showDialogOverlayInput("Edit Hub", "Display name:", "", "Save", "Cancel", func(nameText string) {
	}, nil)
}
