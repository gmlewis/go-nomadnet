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

// RoomWidget displays a single RRC chat room with messages, users, and editor.
// Matches Python's RoomWidget at Channels.py:590.
type RoomWidget struct {
	app          *tview.Application
	hubName      string
	roomName     string
	widget       tview.Primitive
	columns      *tview.Flex
	chatBox      *tview.Flex
	messages     *tview.TextView
	usersList    *tview.List
	editor       *ReadlineEdit
	usersVisible bool

	// Callbacks
	OnSendMessage func(text string)
	OnLeaveRoom   func()
	OnToggleUsers func()

	// Message data
	chatMessages []ChannelMessage
	members      []ChannelMember
}

// NewRoomWidget creates a chat room view for the given hub and room.
// Matches Python's RoomWidget.__init__().
func NewRoomWidget(app *tview.Application, hubName, roomName string) *RoomWidget {
	rw := &RoomWidget{
		app:          app,
		hubName:      hubName,
		roomName:     roomName,
		usersVisible: true,
	}

	// Messages view
	rw.messages = tview.NewTextView()
	rw.messages.SetDynamicColors(true)
	rw.messages.SetScrollable(true)
	rw.messages.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	rw.messages.SetBackgroundColor(tcell.ColorDefault)

	// Editor
	rw.editor = NewReadlineEdit("", "Type a message...")
	rw.editor.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	rw.editor.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Header: room title
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignCenter)
	header.SetDynamicColors(true)
	header.SetTextColor(tcell.NewHexColor(0xdddddd))
	header.SetText(fmt.Sprintf("[::b]#%s[-] @ %s", roomName, hubName))

	// Chat box: header + messages + editor
	rw.chatBox = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(rw.messages, 0, 1, false).
		AddItem(rw.editor, 1, 0, true)
	rw.chatBox.SetBorder(true)

	// Users list
	rw.usersList = tview.NewList()
	rw.usersList.SetHighlightFullLine(true)
	rw.usersList.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	usersBox := tview.NewFlex().SetDirection(tview.FlexRow)
	usersTitle := tview.NewTextView()
	usersTitle.SetTextAlign(tview.AlignCenter)
	usersTitle.SetDynamicColors(true)
	usersTitle.SetTextColor(tcell.NewHexColor(0xdddddd))
	usersTitle.SetText("[::b]Users[-]")
	usersBox.AddItem(usersTitle, 1, 0, false)
	usersBox.AddItem(rw.usersList, 0, 1, true)
	usersBox.SetBorder(true)

	// Columns: chat + users
	rw.columns = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(rw.chatBox, 0, 1, true).
		AddItem(usersBox, 20, 0, false)

	rw.widget = rw.columns
	rw.widget.(*tview.Flex).SetInputCapture(rw.handleInput)

	return rw
}

// Widget returns the tview primitive.
func (rw *RoomWidget) Widget() tview.Primitive {
	return rw.widget
}

// handleInput processes keyboard shortcuts for the room.
// Matches Python's RoomFrame.keypress() at Channels.py:522.
func (rw *RoomWidget) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlD:
		rw.sendMessage()
		return nil
	case tcell.KeyCtrlX:
		if rw.OnLeaveRoom != nil {
			rw.OnLeaveRoom()
		}
		return nil
	case tcell.KeyCtrlU:
		rw.toggleUsers()
		return nil
	}

	return event
}

// sendMessage sends the current editor content.
func (rw *RoomWidget) sendMessage() {
	text := rw.editor.GetText()
	if text == "" {
		return
	}
	if rw.OnSendMessage != nil {
		rw.OnSendMessage(text)
	}
	rw.editor.SetText("")
}

// toggleUsers shows/hides the users pane.
func (rw *RoomWidget) toggleUsers() {
	rw.usersVisible = !rw.usersVisible
	if rw.OnToggleUsers != nil {
		rw.OnToggleUsers()
	}
}

// SetMessages replaces the message list.
func (rw *RoomWidget) SetMessages(msgs []ChannelMessage) {
	rw.chatMessages = msgs
	rw.renderMessages()
}

// SetMembers replaces the member list.
func (rw *RoomWidget) SetMembers(members []ChannelMember) {
	rw.members = members
	rw.renderMembers()
}

// renderMessages renders all chat messages.
func (rw *RoomWidget) renderMessages() {
	var sb strings.Builder
	for _, msg := range rw.chatMessages {
		switch {
		case msg.IsSystem:
			sb.WriteString(fmt.Sprintf("[gray]%s[-]\n", msg.Text))
		case msg.IsNotice:
			sb.WriteString(fmt.Sprintf("[yellow]%s[-]\n", msg.Text))
		case msg.IsError:
			sb.WriteString(fmt.Sprintf("[red]%s[-]\n", msg.Text))
		default:
			if msg.IsSelf {
				sb.WriteString(fmt.Sprintf("[#66cc55]<%s>[-] %s\n", msg.Nick, msg.Text))
			} else {
				nickCol := nickColor(msg.Nick)
				sb.WriteString(fmt.Sprintf("%s<%s>[-] %s\n", nickCol, msg.Nick, msg.Text))
			}
		}
	}

	if len(rw.chatMessages) == 0 {
		sb.WriteString("[gray]No messages yet. Type below to send.[-]\n")
	}

	rw.messages.SetText(sb.String())
}

// renderMembers renders the users list.
func (rw *RoomWidget) renderMembers() {
	rw.usersList.Clear()
	for _, m := range rw.members {
		icon := "○"
		if m.Online {
			icon = "●"
		}
		text := fmt.Sprintf("%s %s", icon, m.Nick)
		rw.usersList.AddItem(text, "", 0, nil)
	}
	if len(rw.members) == 0 {
		rw.usersList.AddItem("[gray]No users[-]", "", 0, nil)
	}
}
