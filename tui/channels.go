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

// ChannelsDisplay shows the RRC chat interface.
type ChannelsDisplay struct {
	app      *tview.Application
	widget   tview.Primitive
	layout   *tview.Flex
	rooms    *tview.List
	messages *tview.TextView
	members  *tview.List
	input    *ReadlineEdit

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
}

// NewChannelsDisplay creates a new channels display.
func NewChannelsDisplay(app *tview.Application, rooms []ChannelInfo) *ChannelsDisplay {
	cd := &ChannelsDisplay{app: app}

	// Title
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Channels[-]")

	// Rooms list
	cd.rooms = tview.NewList().
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	cd.rooms.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		// Room selected — could load messages for that room
	})

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

	// Messages view
	cd.messages = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText("[gray]Select a room to view messages[-]")

	// Members list
	cd.members = tview.NewList().
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	// Input field
	cd.input = NewReadlineEdit("", "Type a message...")
	cd.input.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cd.input.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Layout: rooms on left, messages+members on right
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.rooms, 0, 1, true)

	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cd.messages, 0, 3, false).
		AddItem(cd.members, 5, 0, false).
		AddItem(cd.input, 1, 0, true)

	content := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 0, 1, true).
		AddItem(rightPanel, 0, 3, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(content, 0, 1, true)
	layout.SetBorder(true)
	layout.SetInputCapture(cd.handleInput)

	cd.layout = layout
	cd.widget = layout

	return cd
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
		if cd.OnRemoveHub != nil {
			cd.OnRemoveHub()
		}
		return nil
	}

	return event
}

// Widget returns the tview primitive for this display.
func (cd *ChannelsDisplay) Widget() tview.Primitive {
	return cd.widget
}

// ShowMessages displays messages for a room.
func (cd *ChannelsDisplay) ShowMessages(msgs []ChannelMessage) {
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
