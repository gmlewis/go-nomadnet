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
	app              *App
	hubName          string
	roomName         string
	widget           tview.Primitive
	columns          *tview.Flex
	chatBox          *tview.Flex
	header           *tview.TextView
	usersBox         *tview.Flex
	usersTitle       *tview.TextView
	messages         *tview.TextView
	usersList        *tview.List
	editor           *ReadlineEdit
	usersVisible     bool
	usersWidth       int
	hubConnected     bool
	maxMessageBytes  int
	collapseJoinPart bool

	// Callbacks
	OnSendMessage    func(text string)
	OnLeaveRoom      func()
	OnToggleUsers    func()
	OnToggleCollapse func()
	OnTabComplete    func()
	OnSplitDialog    func(text string, limit int)
	// OnMemberClick fires when the user activates a member row (Python
	// connects each ChannelListEntry's click signal to
	// display.show_user_info with the peer hash, Channels.py:713).
	OnMemberClick func(nick, hash string)

	// Message data
	chatMessages []ChannelMessage
	members      []ChannelMember

	// Tab-completion cycling state (mirrors Python RoomMessageEdit._tab_state,
	// Channels.py:458) and the local user's nick (excluded from candidates).
	tabState *TabState
	ownNick  string
}

// NewRoomWidget creates a chat room view for the given hub and room.
// Matches Python's RoomWidget.__init__().
func NewRoomWidget(app *App, hubName, roomName string) *RoomWidget {
	rw := &RoomWidget{
		app:             app,
		hubName:         hubName,
		roomName:        roomName,
		usersVisible:    true,
		hubConnected:    true,
		maxMessageBytes: 350,
	}

	// Messages view
	rw.messages = applyWheelMultiplier(tview.NewTextView())
	rw.messages.SetDynamicColors(true)
	rw.messages.SetScrollable(true)
	// Message bodies are rendered as `[#66cc55]<nick>[-] <text>` (renderMessages),
	// so the body text carries no color tag and inherits this SetTextColor.
	// Python colors the body with the body_text palette attr (Channels.py:1333
	// _body_markup(body, body_attr="body_text")); body_text is 3-hex #ddd dark /
	// #222 light (ui/TextUI.py:26,80), cube-quantized to #d7d7d7 / #000000.
	tc := GetThemeColors(app.Theme)
	rw.messages.SetTextColor(tc["body_text"])
	rw.messages.SetBackgroundColor(tcell.ColorDefault)

	// Editor: Python wraps it in AttrMap(editor, "msg_editor") (Channels.py:609);
	// msg_editor is #111/#0bb (both themes, ui/TextUI.py:32,85), cube-quantized
	// to #000000/#00afaf.
	rw.editor = NewReadlineEdit(app.killRing, "", "Type a message...")
	rw.editor.SetFieldBackgroundColor(tc["msg_editor_bg"])
	rw.editor.SetFieldTextColor(tc["msg_editor_fg"])

	// Header: room title
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignCenter)
	header.SetDynamicColors(true)
	header.SetTextColor(tc["msg_header_sent_fg"])
	header.SetBackgroundColor(tc["msg_header_sent_bg"])
	header.SetText(fmt.Sprintf("[::b]#%v[-] @ %v", roomName, hubName))
	rw.header = header

	// Chat box: header + messages + editor
	rw.chatBox = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(rw.messages, 0, 1, false).
		AddItem(rw.editor, 1, 0, true)
	rw.chatBox.SetBorder(true)

	// Users list
	rw.usersList = tview.NewList()
	rw.usersList.SetHighlightFullLine(true)
	ApplyListFocusStyle(rw.usersList, app.Theme)

	usersBox := tview.NewFlex().SetDirection(tview.FlexRow)
	usersTitle := tview.NewTextView()
	usersTitle.SetTextAlign(tview.AlignCenter)
	usersTitle.SetDynamicColors(true)
	usersTitle.SetTextColor(tcell.ColorDefault)
	usersTitle.SetText("[::b]Users[-]")
	rw.usersTitle = usersTitle
	usersBox.AddItem(usersTitle, 1, 0, false)
	usersBox.AddItem(rw.usersList, 0, 1, true)
	usersBox.SetBorder(true)
	rw.usersBox = usersBox
	rw.usersWidth = 22

	// Columns: chat + users
	rw.columns = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(rw.chatBox, 0, 1, true).
		AddItem(usersBox, rw.usersWidth, 0, false)

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
		// Python's RoomMessageEdit sends on ctrl d (Channels.py
		// RoomMessageEdit.keypress); Enter is NOT a send key.
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
	case tcell.KeyF8:
		rw.toggleCollapse()
		return nil
	case tcell.KeyTab:
		if rw.doTabComplete() {
			return nil
		}
		return event
	}

	return event
}

// sendMessage sends the current editor content.
// Matches Python's RoomWidget.send_message() at Channels.py:864.
func (rw *RoomWidget) sendMessage() {
	text := strings.TrimSpace(rw.editor.GetText())
	if text == "" {
		return
	}
	if strings.HasPrefix(text, "/") {
		rw.handleSlashCommand(text)
		return
	}
	if !rw.hubConnected {
		if rw.OnSendMessage != nil {
			rw.OnSendMessage("/connect")
		}
		return
	}
	if NeedsSplit(text, rw.maxMessageBytes) {
		if rw.OnSplitDialog != nil {
			rw.OnSplitDialog(text, rw.maxMessageBytes)
		}
		return
	}
	if rw.OnSendMessage != nil {
		rw.OnSendMessage(text)
	}
	rw.editor.SetText("")
}

// handleSlashCommand dispatches slash commands matching Python's
// _handle_slash_command at Channels.py:997.
func (rw *RoomWidget) handleSlashCommand(text string) {
	parts := strings.SplitN(text, " ", 2)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/join", "/j":
		if rw.OnSendMessage != nil {
			rw.OnSendMessage(text)
		}
	case "/part", "/leave":
		if rw.OnLeaveRoom != nil {
			rw.OnLeaveRoom()
		}
	case "/quit", "/q", "/disconnect":
		if rw.OnLeaveRoom != nil {
			rw.OnLeaveRoom()
		}
	case "/nick", "/who", "/names", "/topic", "/mode", "/me":
		if rw.OnSendMessage != nil {
			rw.OnSendMessage(text)
		}
	default:
		if rw.OnSendMessage != nil {
			rw.OnSendMessage(text)
		}
	}
	rw.editor.SetText("")
}

// toggleUsers shows/hides the users pane, mirroring Python's
// RoomWidget.toggle_user_list (Channels.py:749) which removes the user list
// column entirely when toggled off.
func (rw *RoomWidget) toggleUsers() {
	rw.usersVisible = !rw.usersVisible
	if rw.usersVisible {
		// Re-add the users column if it is not present.
		present := false
		for i := 0; i < rw.columns.GetItemCount(); i++ {
			if rw.columns.GetItem(i) == rw.usersBox {
				present = true
				break
			}
		}
		if !present {
			rw.columns.AddItem(rw.usersBox, rw.usersWidth, 0, false)
		}
	} else {
		rw.columns.RemoveItem(rw.usersBox)
	}
	if rw.OnToggleUsers != nil {
		rw.OnToggleUsers()
	}
	// tview redraws automatically after the input handler returns; calling
	// QueueUpdateDraw here would deadlock when invoked from the input handler
	// (the event loop is busy in this call and cannot drain the queue) and also
	// blocks forever in tests where no event loop is running.
}

// RoomName returns the room this widget displays.
func (rw *RoomWidget) RoomName() string { return rw.roomName }

// SetMessages replaces the message list.
func (rw *RoomWidget) SetMessages(msgs []ChannelMessage) {
	rw.chatMessages = msgs
	rw.renderMessages()
}

// toggleCollapse flips the join/leave collapse flag and re-renders, matching
// Python's toggle_join_part_collapse (Channels.py:1537). The optional
// OnToggleCollapse callback fires after the state flip.
func (rw *RoomWidget) toggleCollapse() {
	rw.collapseJoinPart = !rw.collapseJoinPart
	rw.renderMessages()
	if rw.OnToggleCollapse != nil {
		rw.OnToggleCollapse()
	}
}

// CollapseJoinPart reports whether join/leave system messages are collapsed.
func (rw *RoomWidget) CollapseJoinPart() bool {
	return rw.collapseJoinPart
}

// SetCollapseJoinPart sets the join/leave collapse flag and re-renders.
func (rw *RoomWidget) SetCollapseJoinPart(v bool) {
	rw.collapseJoinPart = v
	rw.renderMessages()
}

// SetMembers replaces the member list.
func (rw *RoomWidget) SetMembers(members []ChannelMember) {
	rw.members = members
	rw.renderMembers()
}

// SetOwnNick sets the local user's nick, which is excluded from tab-completion
// candidates (Python excludes the user's own nick in _candidates,
// Channels.py:439).
func (rw *RoomWidget) SetOwnNick(nick string) {
	rw.ownNick = nick
}

// doTabComplete performs one nick tab-completion step in the editor, mirroring
// Python's RoomMessageEdit._try_tab_complete (Channels.py:458). Candidates are
// the deduplicated member nicks (minus the local user's own nick). Returns true
// when a completion was applied (Tab consumed), false when there was no token
// or no match (Tab propagates).
func (rw *RoomWidget) doTabComplete() bool {
	candidates := make([]string, 0, len(rw.members))
	seen := make(map[string]bool, len(rw.members))
	for _, m := range rw.members {
		if m.Nick == "" || m.Nick == rw.ownNick {
			continue
		}
		key := strings.ToLower(m.Nick)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, m.Nick)
	}
	text := rw.editor.GetText()
	newText, newCursor, state, ok := TabComplete(text, rw.editor.CursorPos(), rw.tabState, candidates)
	if !ok {
		rw.tabState = nil
		return false
	}
	rw.tabState = state
	rw.editor.SetText(newText)
	rw.editor.SetCursorPos(newCursor)
	return true
}

// renderMessages renders all chat messages.
func (rw *RoomWidget) renderMessages() {
	msgs := rw.chatMessages
	if rw.collapseJoinPart {
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
		default:
			if msg.IsSelf {
				fmt.Fprintf(&sb, "[#66cc55]<%v>[-] %v\n", msg.Nick, msg.Text)
			} else {
				nickCol := nickColor(msg.Nick)
				fmt.Fprintf(&sb, "[%v]<%v>[-] %v\n", nickCol, msg.Nick, msg.Text)
			}
		}
	}

	if len(rw.chatMessages) == 0 {
		sb.WriteString("[gray]No messages yet. Type below to send.[-]\n")
	}

	rw.messages.SetText(sb.String())
}

// renderMembers renders the users list. Each row activates via
// activateMember, which fires OnMemberClick with the member's nick and
// identity hash (Python urwid.connect_signal(entry, "click",
// self.display.show_user_info, (self.hub, peer_hash, full_name)),
// Channels.py:713).
func (rw *RoomWidget) renderMembers() {
	rw.usersList.Clear()
	for _, m := range rw.members {
		icon := "○"
		if m.Online {
			icon = "●"
		}
		text := fmt.Sprintf("%v %v", icon, m.Nick)
		rw.usersList.AddItem(text, "", 0, nil)
	}
	if len(rw.members) == 0 {
		rw.usersList.AddItem("[gray]No users[-]", "", 0, nil)
	}
	rw.usersList.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		rw.ActivateMember(i)
	})
}

// ActivateMember fires OnMemberClick for the member at the given users-pane
// row (Python's entry click signal → show_user_info). Out-of-range indices
// and the "No users" placeholder row are no-ops.
func (rw *RoomWidget) ActivateMember(index int) {
	if rw.OnMemberClick == nil || index < 0 || index >= len(rw.members) {
		return
	}
	m := rw.members[index]
	rw.OnMemberClick(m.Nick, m.Hash)
}

// HubConnected reports whether the hub is currently connected.
func (rw *RoomWidget) HubConnected() bool {
	return rw.hubConnected
}

// SetHubConnected sets the hub connection status.
func (rw *RoomWidget) SetHubConnected(connected bool) {
	rw.hubConnected = connected
}

// MaxMessageBytes returns the per-message byte limit.
func (rw *RoomWidget) MaxMessageBytes() int {
	return rw.maxMessageBytes
}

// SetMaxMessageBytes sets the per-message byte limit.
func (rw *RoomWidget) SetMaxMessageBytes(limit int) {
	rw.maxMessageBytes = limit
}
