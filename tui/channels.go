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
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
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

	// Keyboard shortcut callbacks (Python: ChannelsListArea.keypress, RoomFrame.keypress)
	OnNewHub              func()
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
		// Room selected — load its messages (wired by the app layer).
	})
	cd.ilb = NewIndicativeListBox(cd.rooms)

	// No-hubs empty state (Python _compose_list_widgets, Channels.py:1603-1607):
	// a single left-aligned "No hubs yet. Press Ctrl-N to add one." Text in the
	// list_unknown color with a leading blank line, wrapped inside the list area
	// between the indicator bars. Pre-wrapped with urwidSpaceWrap at the fixed
	// inner width (given_list_width 36 − 2 borders = 34) so the break lands where
	// urwid breaks (after "add"), not where tview's WordWrap breaks (after "to").
	colors := GetThemeColors(app.Theme)
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
	// selected, Phase 5 RRC).
	cd.messages = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb))
	cd.members = tview.NewList()
	cd.members.SetHighlightFullLine(true)
	ApplyListFocusStyle(cd.members, app.Theme)
	cd.members.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		if cd.OnMemberClick != nil {
			cd.OnMemberClick(mainText, secondaryText)
		}
	})
	cd.input = NewReadlineEdit(app.killRing, "", "Type a message...")
	cd.input.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cd.input.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Two-pane Columns: left list (given 36) + right pane (weight 1). No outer
	// border — each pane carries its own (Python columns_widget, Channels.py:
	// 1462-1468). The expand gutter is shown only when the list is hidden.
	cd.chanGutter = NewChannelsExpandGutter(func() { cd.ToggleChannelListState() })
	cd.content = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(cd.leftPanel, cd.leftWidth, 0, true).
		AddItem(cd.rightPane, 0, 1, false)
	cd.content.SetInputCapture(cd.handleInput)
	cd.widget = cd.content

	return cd
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
		text := fmt.Sprintf("%s#%s", prefix, room.Name)
		secondary := fmt.Sprintf("%d members — %s", room.Members, room.Topic)
		cd.rooms.AddItem(text, secondary, 0, nil)
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
		text := fmt.Sprintf("%s#%s", prefix, room.Name)
		secondary := fmt.Sprintf("%d members — %s", room.Members, room.Topic)
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
	info := fmt.Sprintf("[::b]Nick[-]  : %s\n[::b]Hash[-] : %s", nick, hash)
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
			sb.WriteString(fmt.Sprintf("[gray]%s[-]\n", msg.Text))
		case msg.IsNotice:
			sb.WriteString(fmt.Sprintf("[yellow]%s[-]\n", msg.Text))
		case msg.IsError:
			sb.WriteString(fmt.Sprintf("[red]%s[-]\n", msg.Text))
		case msg.IsSelf:
			sb.WriteString(fmt.Sprintf("[green]%s[-] %s\n", msg.Nick, msg.Text))
		default:
			color := nickColor(msg.Nick)
			sb.WriteString(fmt.Sprintf("[%s]%s[-] %s\n", color, msg.Nick, msg.Text))
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
		cd.members.AddItem(fmt.Sprintf("%s %s", status, m.Nick), m.Hash[:12], 0, nil)
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
		return fmt.Sprintf("[gray]system:%s[-]", msg.Text)
	case msg.IsNotice:
		return fmt.Sprintf("[yellow]notice:%s[-]", msg.Text)
	case msg.IsError:
		return fmt.Sprintf("[red]error:%s[-]", msg.Text)
	case msg.IsSelf:
		return fmt.Sprintf("[green]%s[-] %s", msg.Nick, msg.Text)
	default:
		// Check for mention indicator
		extra := ""
		if msg.Mention {
			extra = " [orange]@mention[-]"
		}
		return fmt.Sprintf("[%s]%s[-] %s%s", nickColor(msg.Nick), msg.Nick, msg.Text, extra)
	}
}

// ShowUserInfoDialog displays user information for a channel member.
// Matches Python's ChannelsDisplay.show_user_info() at
// Channels.py:2119-2155.
func (cd *ChannelsDisplay) ShowUserInfoDialog(nick, identityHash string, isSelf bool, onOpenConversation func()) {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(" Nick     : %s\n", nick))
	sb.WriteString(fmt.Sprintf(" Identity : %s\n", identityHash))

	if isSelf {
		sb.WriteString("\n (This is you)\n")
	}

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn)
	if isSelf {
		buttons.AddItem(tview.NewButton("Close").SetSelectedFunc(func() {}), 0, 1, true)
	} else {
		buttons.AddItem(tview.NewButton("Open Conversation").SetSelectedFunc(func() {
			if onOpenConversation != nil {
				onOpenConversation()
			}
		}), 0, 1, true)
		buttons.AddItem(tview.NewButton("Close").SetSelectedFunc(func() {}), 0, 1, false)
	}

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetText(sb.String()), 0, 1, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("User Info", layout, 40, 8, nil)
}
