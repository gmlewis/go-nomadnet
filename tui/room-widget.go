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
	messages         *tview.TextView
	messagesArea     *IndicativeMessages
	usersList        *tview.List
	usersCount       *tview.TextView
	editor           *ReadlineEdit
	usersVisible     bool
	usersWidth       int
	hubConnected     bool
	maxMessageBytes  int
	collapseJoinPart bool
	// editorRows caches the composer's current wrapped row height so the
	// draw-func only calls ResizeItem when it changes (initially 1 row, the
	// layout's AddItem height).
	editorRows int

	// hubStatusFn reports the hub's LIVE connection status (the rrc status
	// enum), mirroring Python's RoomWidget.send_message reading self.hub.status
	// directly at send time (Channels.py:873). The hubConnected snapshot alone
	// goes stale between list rebuilds: a hub whose link dropped after the last
	// refresh would still report connected, so sends vanish into a dead link
	// while the composer keeps echoing them locally.
	hubStatusFn func() int

	// Callbacks
	OnSendMessage    func(text string)
	OnLeaveRoom      func()
	OnToggleUsers    func()
	OnToggleCollapse func()
	OnTabComplete    func()
	OnSplitDialog    func(text string, limit int)
	// OnConnectHub fires when the user sends while the hub is disconnected or
	// runs /connect (Python RoomWidget.send_message Channels.py:873-876 and
	// _handle_slash_command "connect" Channels.py:1094-1100: hub.connect()
	// plus a "Connecting..." system notice — the literal command text is never
	// transmitted as chat).
	OnConnectHub func()
	// OnMemberClick fires when the user activates a member row (Python
	// connects each ChannelListEntry's click signal to
	// display.show_user_info with the peer hash, Channels.py:713).
	OnMemberClick func(nick, hash string)
	// OnFocusRegion fires when the room's editor gains focus so the shortcut
	// bar switches to the editor region (Python RoomFrame.focus_position
	// setter → update_active_shortcuts, Channels.py:509-520).
	OnFocusRegion func(region string)
	// OnFocusMenu fires when the room body's Up reaches the visible top of
	// the message list, moving focus to the main display's menu bar (Python
	// RoomFrame.keypress body "up" → main_display.frame.focus_position =
	// "header", Channels.py:541-544).
	OnFocusMenu func()

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
		editorRows:      1,
	}

	// Messages view
	rw.messages = applyWheelMultiplier(tview.NewTextView())
	rw.messages.SetDynamicColors(true)
	rw.messages.SetScrollable(true)
	// Message bodies are rendered without per-span color tags for plain text,
	// so they inherit this SetTextColor. Python colors the body with the RRC
	// render's default fg (Channels.py _render_body fg=t["text"]): 3-hex
	// "ddd"/"111" nibble-doubled to #dddddd / #111111 — NOT the static UI
	// palette's cube-quantized body_text (#d7d7d7).
	rw.messages.SetTextColor(rrcRenderColors(app.Theme)["text"])
	rw.messages.SetBackgroundColor(tcell.ColorDefault)
	// Message rows are PRE-WRAPPED at the chat inner width (the justified
	// two-column layout, Channels.py:1408-1413 — continuation lines indent to
	// the "<" column), so the TextView must render the wrapped lines
	// verbatim; letting tview wrap at draw time diverged from Python's break
	// positions (the conversations.go:3071 precedent). IndicativeMessages
	// re-renders on width change via OnWidthChange.
	rw.messages.SetWrap(false)

	// Editor: Python wraps it in AttrMap(editor, "msg_editor") (Channels.py:609);
	// msg_editor is #111/#0bb (both themes, ui/TextUI.py:32,85), cube-quantized
	// to #000000/#00afaf. Python builds RoomMessageEdit(caption="", edit_text="",
	// multiline=True) (Channels.py:605) — NO placeholder, so the idle composer
	// row renders empty, and the multiline Edit WRAPS the draft: the urwid
	// Frame footer grows by one row per wrapped line, shrinking the message
	// list (the parity behavior pinned by TestRoomEditorGrowsPanelParity).
	tc := GetThemeColors(app.Theme)
	rw.editor = NewReadlineEdit(app.killRing, "", "")
	rw.editor.SetMultiline(true)
	rw.editor.SetFieldBackgroundColor(tc["msg_editor_bg"])
	rw.editor.SetFieldTextColor(tc["msg_editor_fg"])
	rw.editor.SetFocusFunc(func() {
		if rw.OnFocusRegion != nil {
			rw.OnFocusRegion("editor")
		}
	})
	// Up on the composer's top wrapped row hands focus to the message body
	// (Python RoomMessageEdit.keypress "up" y==0 → frame.focus_position =
	// "body", Channels.py:429-434).
	rw.editor.OnFocusTopRow = func() {
		rw.app.SetFocus(rw.messagesArea)
	}

	// Header: room title — Python RoomWidget._update_peer_info
	// (Channels.py:737-756) renders it left-aligned via the peer_info_widget;
	// SetRoomHeader fills it from the live hub state.
	header := tview.NewTextView()
	header.SetTextAlign(tview.AlignLeft)
	header.SetDynamicColors(true)
	header.SetTextColor(tc["msg_header_sent_fg"])
	header.SetBackgroundColor(tc["msg_header_sent_bg"])
	rw.header = header

	// Chat box: indicator-wrapped messages + header + editor. The message
	// area is wrapped in IndicativeMessages (Python wraps the messagelist in
	// _StickyMessageListBox, an IndicativeListBox) and the tail is followed
	// sticky-bottom (Channels.py:553-587).
	rw.messagesArea = NewIndicativeMessages(rw.messages)
	// The justified layout pre-wraps at the message view's inner width; when
	// the view is resized (users pane toggle, terminal resize) the render
	// re-wraps at the new width before the TextView draws (the
	// resizeShortcutBar DrawFunc pattern — the hook runs before the child
	// draw in the same pass).
	rw.messagesArea.OnWidthChange = func() { rw.renderMessages() }
	// The body's focus switches the shortcut bar to the body region (Python
	// RoomFrame focus setter → update_active_shortcuts).
	rw.messagesArea.SetFocusFunc(func() {
		if rw.OnFocusRegion != nil {
			rw.OnFocusRegion("body")
		}
	})
	rw.chatBox = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(rw.messagesArea, 0, 1, false).
		AddItem(rw.editor, 1, 0, true)
	// Composer growth: urwid's Frame packs the multiline footer (the Edit is
	// a flow widget sized to its wrapped rows), so the message list shrinks
	// by one row per editor line — the whole panel content moves up as the
	// user keeps typing. The DrawFunc runs before the Flex lays out its
	// children, so the resized composer height takes effect in the same draw
	// (the resizeShortcutBar precedent, main-display.go). The returned rect
	// becomes the Flex's inner rect, so the bordered box returns its inner
	// area for the children.
	rw.chatBox.SetDrawFunc(func(screen tcell.Screen, x, y, w, h int) (int, int, int, int) {
		rw.resizeEditor(w-2, h-2)
		return x + 1, y + 1, w - 2, h - 2
	})
	rw.chatBox.SetBorder(true)

	// Users list: ShowSecondaryText(false) — the fork's List defaults to
	// rendering each item's (empty) secondary text as a phantom row, which
	// painted a blank row after EVERY member (the channels hub list already
	// calls it for this exact reason). Python renders members on consecutive
	// rows (Channels.py:694-705).
	rw.usersList = tview.NewList()
	rw.usersList.ShowSecondaryText(false)
	rw.usersList.SetHighlightFullLine(true)
	ApplyListFocusStyle(rw.usersList, app.Theme)
	// The " N users" count row is a PLAIN urwid.Text row in Python
	// (Channels.py:694 — default-styled, never the selection highlight), so
	// it lives OUT of the List in a one-row TextView above it; a tview List
	// item would paint the selection highlight on it (measured live: the
	// count row rendered fg 0,0,0 / bg 175,175,175).
	rw.usersCount = tview.NewTextView()
	rw.usersCount.SetTextColor(tcell.ColorDefault)

	// Users box: Python UsersBox(self.users_listbox, title="Users")
	// (Channels.py:625) is a LineBox titled "Users" — the title lives in the
	// BORDER, not in a title row inside.
	usersBox := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(rw.usersCount, 1, 0, false).
		AddItem(rw.usersList, 0, 1, true)
	usersBox.SetBorder(true)
	SetTitledBorder(usersBox, "Users")
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

// resizeEditor grows the composer footer to its wrapped row count at the
// chat inner width (the urwid Frame flow footer), shrinking the message
// list. The height is capped so the header and at least one message row
// stay visible when a draft grows taller than the panel — urwid's Frame
// squeezes the body the same way for an over-tall footer.
func (rw *RoomWidget) resizeEditor(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	rows := rw.editor.MultilineRows(w)
	rows = min(rows, max(h-2, 1)) // header (1) + at least one message row
	if rows == rw.editorRows {
		return
	}
	rw.editorRows = rows
	rw.chatBox.ResizeItem(rw.editor, rows, 0)
}

// bodyHasFocus reports whether the room's message body currently holds focus
// (Python RoomFrame.focus_position == "body").
func (rw *RoomWidget) bodyHasFocus() bool {
	if rw.app == nil {
		return false
	}
	return rw.app.GetFocus() == tview.Primitive(rw.messagesArea)
}

// messagesAtBottom reports whether the message body's tail is visible —
// Python RoomFrame body "down" gate: messagelist.bottom_is_visible.
func (rw *RoomWidget) messagesAtBottom() bool {
	_, _, _, vh := rw.messages.GetInnerRect()
	row, _ := rw.messages.GetScrollOffset()
	total := rw.messages.GetWrappedLineCount()
	return vh <= 0 || row+vh >= total
}

// messagesAtTop reports whether the message body's head is visible — Python:
// messagelist.top_is_visible.
func (rw *RoomWidget) messagesAtTop() bool {
	row, _ := rw.messages.GetScrollOffset()
	return row <= 0
}

// handleInput processes keyboard shortcuts for the room.
// Matches Python's RoomFrame.keypress() at Channels.py:522.
func (rw *RoomWidget) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// Body-focused keys (Python RoomFrame.keypress body branch,
	// Channels.py:536-548): Down at the visible bottom returns to the
	// composer footer, Up at the visible top moves to the main display
	// header, and Tab always lands in the footer. Otherwise the key falls
	// through to the message list (scrolling).
	if rw.bodyHasFocus() {
		switch event.Key() {
		case tcell.KeyDown:
			if rw.messagesAtBottom() {
				rw.app.SetFocus(rw.editor)
				return nil
			}
		case tcell.KeyUp:
			if rw.messagesAtTop() {
				if rw.OnFocusMenu != nil {
					rw.OnFocusMenu()
				}
				return nil
			}
		case tcell.KeyTab:
			// Python RoomFrame.keypress tab: focus_position = "footer".
			rw.app.SetFocus(rw.editor)
			return nil
		}
		return event
	}
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

// SetRoomHeader renders the room's peer-info header exactly like Python
// RoomWidget._update_peer_info (Channels.py:737-756):
// " #<room> ┄ <advertised-server>[ v<version>]  (<hub display name>) | <Status> "
// left-aligned, with the ┄ divider glyph between the room and the advertised
// server name.
func (rw *RoomWidget) SetRoomHeader(serverName, hubVersion, statusLabel string) {
	server := ""
	if serverName != "" {
		server = " " + rw.app.Glyphs["divider1"] + " " + serverName
		if hubVersion != "" {
			server += " v" + hubVersion
		}
	}
	left := " #" + rw.roomName + server + "  (" + rw.hubName + ")"
	rw.header.SetText(left + " | " + statusLabel + " ")
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
	// Python RoomWidget.send_message (Channels.py:873-876) reads the hub's
	// LIVE status at send time: when disconnected it calls hub.connect() and
	// KEEPS the draft — no local echo, no literal command transmission. The
	// hubConnected snapshot alone goes stale between rebuilds.
	if !rw.hubIsConnected() {
		if rw.OnConnectHub != nil {
			rw.OnConnectHub()
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

// hubIsConnected reports the hub's live connection status when a live-status
// hook is wired, falling back to the hubConnected snapshot otherwise.
func (rw *RoomWidget) hubIsConnected() bool {
	if rw.hubStatusFn != nil {
		return rw.hubStatusFn() == hubStatusConnected
	}
	return rw.hubConnected
}

// appendLocalNotice renders a local-only notice in the room view without
// transmitting anything (Python RoomWidget._local_message, Channels.py:1205).
func (rw *RoomWidget) appendLocalNotice(text string, isError bool) {
	rw.chatMessages = append(rw.chatMessages, ChannelMessage{
		Room: rw.roomName, Text: text, IsSystem: !isError, IsError: isError,
	})
	rw.renderMessages()
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
	case "/connect":
		// Python _handle_slash_command "connect" (Channels.py:1094-1100):
		// hub.connect() plus a "Connecting..." system notice.
		if rw.OnConnectHub != nil {
			rw.OnConnectHub()
		}
		rw.appendLocalNotice("Connecting...", false)
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

// renderOpts builds the RRC message-render options for this widget: the app's
// config-derived options with the local user's nick merged in.
func (rw *RoomWidget) renderOpts() RRCRenderOpts {
	opts := rw.app.RRCRender
	opts.OwnNick = rw.ownNick
	if opts.Glyphs == nil {
		opts.Glyphs = rw.app.Glyphs
	}
	if len(opts.Palette) == 0 {
		opts.Palette = DefaultNickPalette(opts.Theme)
	}
	return opts
}

// renderMessages renders all chat messages through the Python-parity message
// formatter (grey [HH:MM:SS] prefix, palette-colored <sender> by the sender
// hash, #dddddd body, linkified hash runs). With rrc_ui_justify_msgs (the
// Python default) the rows render in the justified two-column layout,
// pre-wrapped at the message view's inner width so every continuation line
// indents to the "<" column (Channels.py:1408-1413).
func (rw *RoomWidget) renderMessages() {
	msgs := rw.chatMessages
	if rw.collapseJoinPart {
		msgs = CollapseJoinPartMessages(msgs)
	}
	opts := rw.renderOpts()
	_, _, width, _ := rw.messages.GetInnerRect()
	var sb strings.Builder
	for _, msg := range msgs {
		for _, line := range formatRRCMessageLines(msg, opts, width) {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	if len(rw.chatMessages) == 0 {
		// Python update_messages (Channels.py:778-782): the empty placeholder
		// is an irc_system-styled " ℹ  No messages yet" row.
		sys := rrcRenderColors(opts.Theme)["system"]
		sb.WriteString(colorTag(sys, "") + " " + opts.Glyphs["info"] + "  No messages yet" + colorReset + "\n")
	}

	// Sticky bottom (Python _StickyMessageListBox + append_message,
	// Channels.py:799-805): keep the tail visible only when the user was
	// already at the bottom. Before the first draw the inner rect is zero —
	// treat that as at-bottom so a fresh room view opens at the tail like
	// Python's initial bottom focus. ScrollToEnd re-arms tview's trackEnd
	// latch, which the wheel capture clears on user scroll-up.
	_, _, _, vh := rw.messages.GetInnerRect()
	row, _ := rw.messages.GetScrollOffset()
	atBottom := vh <= 0 || row+vh >= rw.messages.GetWrappedLineCount()
	rw.messages.SetText(sb.String())
	if atBottom {
		rw.messages.ScrollToEnd()
	}
}

// renderMembers renders the users list. Each row activates via
// activateMember, which fires OnMemberClick with the member's nick and
// identity hash (Python urwid.connect_signal(entry, "click",
// self.display.show_user_info, (self.hub, peer_hash, full_name)),
// Channels.py:713).
func (rw *RoomWidget) renderMembers() {
	rw.usersList.Clear()
	// Python _refresh_users_pane (Channels.py:694): the pane opens with
	// " N users" then one colored entry per member — the hash-based nick
	// palette for EVERY user, with the self entry marked by the arrow
	// glyph. The count row renders in its own TextView above the List (a
	// List item would take the selection highlight; Python's is a plain
	// urwid.Text row).
	rw.usersCount.SetText(fmt.Sprintf(" %v user%v", len(rw.members),
		map[bool]string{true: "", false: "s"}[len(rw.members) == 1]))
	palette := rw.renderOpts().Palette
	for _, m := range rw.members {
		// Python (Channels.py:695-705): the hash-based palette color for
		// EVERY user; is_self only swaps the peer glyph for the arrow.
		color := NickColorByHashHexColor(m.Hash, palette)
		// Python truncates the display name to its first 15 chars plus an
		// ellipsis once it exceeds 16 (Channels.py:681) — a hard-cut 15-char
		// name is a Go-port artifact.
		name := m.Nick
		if runes := []rune(name); len(runes) > 16 {
			name = string(runes[:15]) + "…"
		}
		label := " " + rw.app.Glyphs["peer"] + " " + name
		if m.IsSelf {
			label = " " + rw.app.Glyphs["arrow_r"] + " " + name
		}
		// Python wraps the entry in AttrMap(entry, style) (Channels.py:705)
		// whose fill paints the member's fg across the FULL pane width —
		// the padded trailing spaces carry the fg in the item text itself.
		label = padToWidth(label, ' ', rw.usersWidth-2)
		rw.usersList.AddItem(fmt.Sprintf("[%v]%v[-]", colorName(color), label), "", 0, nil)
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
