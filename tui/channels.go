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
	rooms    *tview.List
	messages *tview.TextView
	members  *tview.List
	input    *ReadlineEdit
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

	cd.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(content, 0, 1, true)

	return cd
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

// nickColor returns a consistent color for a nickname.
func nickColor(nick string) string {
	colors := []string{
		"red", "green", "yellow", "blue", "magenta", "cyan",
		"lightred", "lightgreen", "lightyellow", "lightblue",
	}
	hash := 0
	for _, c := range nick {
		hash += int(c)
	}
	return colors[hash%len(colors)]
}
