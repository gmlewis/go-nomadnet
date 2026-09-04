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
	SrcHash   string // sender identity-hash hex (nick color + fallback display)
	Text      string
	Timestamp string
	TsMs      int64 // message timestamp in ms since epoch (millisecond precision)
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
	IsSelf bool // true for the local user (Python is_self, Channels.py:699)
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
	app          *App
	widget       tview.Primitive
	content      *tview.Flex
	leftPanel    *tview.Flex
	rightPane    *tview.Flex
	placeholder  *tview.TextView
	hubInfo      *HubInfoArea
	roomWidget   *RoomWidget
	selectedRoom string
	// paneMode tracks what the right pane shows ("", "info", "room") so
	// state-driven refreshes re-render the right widget.
	paneMode string
	// selectedHubIdx is the hub row whose info panel is showing; the
	// zero-value is safe because RefreshHubInfoIfVisible no-ops until the
	// first ShowHubInfo creates the panel.
	selectedHubIdx int
	// HubViewsCache is the last HubView slice passed to SetHubs, kept so the
	// hub info panel can read the selected hub's live state.
	HubViewsCache      []HubView
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
	OnSelectHub func(hubIdx int)
	// OnShowHubInfo renders the hub info panel for the entry at hubIdx
	// (Python _select_hub → _show_hub_info). The wiring layer builds the
	// snapshot from the app's RRC hub state.
	OnShowHubInfo func(hubIdx int)
	OnSelectRoom  func(hubIdx int, room string)

	// Keyboard shortcut callbacks (Python: ChannelsListArea.keypress, RoomFrame.keypress)
	OnNewHub func()
	// OnAddHub adds the hub to the app's RRC manager and persists it (Python
	// rrc.add_hub, RRC.py:389-401), then the wiring layer refreshes the hub
	// list. nil disables the New Hub dialog's add path.
	OnAddHub   func(hubHash []byte, destName, name string)
	OnJoinRoom func()
	// OnJoinRoomSubmitted runs the add+join+select flow for the room typed in
	// the Add Room dialog (Python join_room_dialog confirmed(), which calls
	// hub.add_room + hub.join_room + _select_room).
	OnJoinRoomSubmitted   func(room, key string)
	OnConnect             func()
	OnDisconnect          func()
	OnToggleAutoReconnect func()
	OnEditHub             func()
	OnRemoveHub           func()
	OnToggleChannelList   func()
	OnSendMessage         func(text string)
	OnLeaveRoom           func(room string)
	OnToggleCollapse      func()
	OnMemberClick         func(nick, hash string)
	// The room-composer slash commands (Python _handle_slash_command,
	// Channels.py:997-1120): the callbacks perform the hub mutation and
	// return errors for local error notices.
	OnSendAction    func(text string) error
	OnSendPing      func() error
	OnJoinRoomNamed func(room string) error
	OnClearMessages func()
	OnNickInfo      func() (nick string, isOverride bool)
	OnSetNick       func(name string) error
	OnDisconnectHub func()
	// OnConnectHub triggers the ACTIVE hub's connect from the room composer
	// (Python RoomWidget.send_message's disconnected branch, Channels.py:873,
	// and the /connect slash command, Channels.py:1094).
	OnConnectHub func()
	// OnRemoveSelected runs the confirmed() branch of Python
	// remove_selected_dialog (Channels.py:1892-1904): room set → part_room +
	// remove_room; empty room → rrc.remove_hub. The wiring layer performs the
	// hub mutation, refreshes the list, and shows the placeholder.
	OnRemoveSelected func(hubIdx int, room string)
	// OnEditHubSubmitted runs the confirmed() branch of Python edit_hub_dialog
	// (Channels.py:2023-2037): apply the edited display name and the three
	// auto toggles, save the RRC manager, refresh the list, and re-show the
	// hub info panel when this hub is still selected.
	OnEditHubSubmitted func(hubIdx int, name string, autoReconnect, autoList, autoWho bool)
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
	// Single-line rows: the fork's List defaults showSecondaryText to true,
	// which renders each item's (empty) secondary text as a phantom blank row
	// between the entries — Python's Channels list rows are one line each
	// (Channels.py:1599-1662 compose one Text per row).
	cd.rooms.ShowSecondaryText(false)
	cd.ilb = NewIndicativeListBox(cd.rooms)
	// Python's ILB skips non-selectable rows (the spacer is a plain
	// urwid.Text, which urwid's ListBox cursor walks past); mirror that so
	// the arrows never land on the blank separator.
	cd.ilb.SetSkipUnselectable(func(idx int) bool {
		return idx >= 0 && idx < len(cd.hubEntries) && cd.hubEntries[idx].Kind == RowSpacer
	})

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
	// The members pane rows are single-line in Python as well.
	cd.members.ShowSecondaryText(false)
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
// display (Python's _show_dialog_overlay, 60% width, PACK). title carries the
// dialog's LineBox title (Python's "?" for remove_selected_dialog).
func (cd *ChannelsDisplay) showDialogOverlayConfirm(title, message string, onYes, onNo func()) {
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
	dialog := NewDialogLineBox(title, layout, close)
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
	// Python update_list (Channels.py:1664-1685): capture the selected row's
	// stable key before rebuilding, then re-select the matching row after — a
	// background status change (connect/disconnect/welcome) must never yank
	// the selection back to the first row while the user is navigating.
	prevKey := cd.selectedRowKey()
	entries := ComposeHubList(hubs, glyphs)
	cd.hubEntries = entries
	cd.HubViewsCache = hubs
	cd.rooms.Clear()
	for i, e := range entries {
		// Render the label PADDED to the list's inner width (padToWidth
		// truncates an over-wide one) and set the row's palette color via
		// SetItemStyle instead. Python's urwid AttrMap rows paint the
		// row's attr across the FULL pane width (the connected hub row's
		// fg #5faf00 covers all 34 left-pane columns — the 2026-09-03
		// 12:32 full-fleet capture), while the fork's List paints item
		// text only and full-line-fills the SELECTED row (tview list.go:
		// the fill block fires when selected && highlightFullLine) — the
		// padded trailing spaces carry the row's fg in the item text
		// itself, byte-exact with Python's fill. The former un-padded
		// tagged-text form overrode the list's selected colors on the
		// HIGHLIGHTED row: a disconnected hub's list_unknown foreground
		// (#afafaf) on the list_focus background (#afafaf) rendered the
		// selected row INVISIBLE (gray on gray). tview's selected style
		// replaces item styles on selection but cannot override tags
		// embedded in the text.
		label := e.Label
		if e.Kind != RowSpacer {
			label = padToWidth(e.Label, ' ', channelsListInnerWidth)
		}
		cd.rooms.AddItem(label, "", 0, nil)
		if c, ok := colors[e.Style]; ok && e.Kind != RowSpacer {
			cd.rooms.SetItemStyle(i, tcell.StyleDefault.Foreground(c))
		}
	}
	cd.restoreRowSelection(prevKey)
}

// hubRowKey is the stable identity of one channels-list row across rebuilds,
// mirroring Python's _row_key tuples (Channels.py:1688-1697): a hub row is
// ("hub", hub_hash, dest_name) and a room row is ("room", hash, dest_name,
// room). The Go port keys on (kind, hub position, room) instead of the hash —
// hub positions are append-stable, so a row keeps its identity across the
// status-driven rebuilds that this restore exists for.
type hubRowKey struct {
	kind   HubListEntryKind
	hubIdx int
	room   string
}

// selectedRowKey returns the stable key of the row the list currently selects,
// or an unknown key when nothing is selected (Python prev_key = None).
func (cd *ChannelsDisplay) selectedRowKey() hubRowKey {
	idx := cd.rooms.GetCurrentItem()
	if idx < 0 || idx >= len(cd.hubEntries) {
		return hubRowKey{kind: -1}
	}
	e := cd.hubEntries[idx]
	return hubRowKey{kind: e.Kind, hubIdx: e.HubIdx, room: e.Room}
}

// restoreRowSelection re-selects the row matching prevKey after a rebuild
// (Python update_list's prev_key loop, Channels.py:1686-1692). An unknown key
// selects nothing, leaving the list at its default first item.
func (cd *ChannelsDisplay) restoreRowSelection(prevKey hubRowKey) {
	if prevKey.kind < 0 {
		return
	}
	for i, e := range cd.hubEntries {
		if e.Kind == prevKey.kind && e.HubIdx == prevKey.hubIdx && e.Room == prevKey.room {
			cd.rooms.SetCurrentItem(i)
			return
		}
	}
}

// ShowHubInfo renders the hub info panel for the hub entry at hubIdx,
// mirroring Python's _show_hub_info (Channels.py:1745-1816): the right pane
// swaps from the placeholder to the hub's details (the header block, the
// status, the auto toggles, the MOTD, the joined and available rooms).
func (cd *ChannelsDisplay) ShowHubInfo(hubIdx int) {
	if cd.app == nil || hubIdx < 0 || hubIdx >= len(cd.HubViewsCache) {
		return
	}
	// Locate the hub's LIST row: the hub index is NOT the row index — a
	// spacer row separates consecutive hubs (ComposeHubList), so hub 1's row
	// is entry 2+. The previous hubEntries[hubIdx] lookup hit the spacer and
	// silently returned, which meant the info panel never opened for the
	// second and later hubs.
	rowIdx := -1
	for i, e := range cd.hubEntries {
		if e.Kind == RowHub && e.HubIdx == hubIdx {
			rowIdx = i
			break
		}
	}
	if rowIdx < 0 {
		return
	}
	hv := cd.HubViewsCache[hubIdx]
	snap := &HubInfoSnapshot{
		Name:        hv.Name(),
		Address:     hv.AddressHex(),
		Status:      hv.Status(),
		StatusText:  hv.StatusText(),
		ServerName:  hv.ServerName(),
		HubVersion:  hv.HubVersion(),
		MOTD:        hv.MOTD(),
		AutoReconn:  hv.AutoReconnect(),
		AutoList:    hv.AutoList(),
		AutoWho:     hv.AutoWho(),
		JoinedRooms: hv.JoinedRooms(),
		AvailRooms:  hv.AvailableRoomList(),
	}
	if cd.hubInfo == nil {
		cd.hubInfo = NewHubInfoArea(cd.app, hv.Name())
		// Python's HubInfoArea.keypress delegates EVERY shortcut to
		// self.delegate (the Channels display), Channels.py:381-409. Wire the
		// panel's callbacks to the display's own dispatch so Ctrl-Y (and the
		// other hub-info shortcuts) behave identically when the panel holds
		// focus — including the Ctrl-Y channel-list toggle, which the panel's
		// own handler previously swallowed with no action.
		hia := cd.hubInfo
		hia.OnNewHub = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModNone)) }
		hia.OnJoinRoom = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone)) }
		hia.OnConnect = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone)) }
		hia.OnDisconnect = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone)) }
		hia.OnToggleAutoReconnect = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModNone)) }
		hia.OnEditHub = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone)) }
		hia.OnRemoveHub = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone)) }
		hia.OnToggleChannelList = func() { cd.handleInput(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone)) }
	}
	cd.hubInfo.SetHubInfo(*snap)
	cd.paneMode = "info"
	// The hub info area carries its own titled border (Python HubInfoArea,
	// Channels.py:1827); the outer placeholder border goes away (item 14).
	cd.rightPane.SetBorder(false)
	// Swap the right pane's sole item to the hub info widget.
	cd.rightPane.Clear()
	cd.rightPane.AddItem(cd.hubInfo.Widget(), 0, 1, false)
	// Python's _select_hub sets selected_key to the hub row (Channels.py:
	// 1710-1717) — the info panel's hub row carries the list highlight.
	for i, e := range cd.hubEntries {
		if e.Kind == RowHub && e.HubIdx == hubIdx {
			cd.rooms.SetCurrentItem(i)
			break
		}
	}
}

// RoomMessagesFunc returns the current message buffer for a hub's room.
type RoomMessagesFunc func(hubIdx int, room string) []ChannelMessage

// RoomMembersFunc returns the member list for a hub's room (Python
// RoomWidget._refresh_users_pane reading hub.get_members).
type RoomMembersFunc func(hubIdx int, room string) []ChannelMember

// ShowRoom swaps the right pane to the room chat view (Python
// _show_room, Channels.py:1841-1851): the RoomWidget for the hub+room with
// the message buffer loaded; the composer routes through OnSendMessage.
func (cd *ChannelsDisplay) ShowRoom(hubIdx int, room string, msgs []ChannelMessage) {
	if cd.app == nil || hubIdx < 0 || hubIdx >= len(cd.HubViewsCache) {
		return
	}
	hv := cd.HubViewsCache[hubIdx]
	room = strings.ToLower(room)
	cd.selectedHubIdx = hubIdx
	cd.selectedRoom = room

	if cd.roomWidget == nil || cd.roomWidget.RoomName() != room {
		cd.roomWidget = NewRoomWidget(cd.app, hv.Name(), room)
		rw := cd.roomWidget
		// The room's editor focus switches the main shortcut bar to the
		// editor region (Python RoomFrame focus setter →
		// update_active_shortcuts, Channels.py:509-520).
		rw.OnFocusRegion = cd.setShortcutRegion
		// The room body's Up at the visible top moves focus to the menu bar
		// (Python: main_display.frame.focus_position = "header").
		rw.OnFocusMenu = func() {
			if cd.app != nil && cd.app.Main != nil {
				cd.app.Main.FocusMenu()
			}
		}
		rw.OnSendMessage = func(text string) {
			if cd.OnSendMessage != nil {
				cd.OnSendMessage(text)
			}
		}
		// Python RoomWidget.send_message reads self.hub.status LIVE at send
		// time (Channels.py:873); the hubConnected snapshot goes stale between
		// rebuilds, so the composer gets the live status through the HubView.
		rw.hubStatusFn = hv.Status
		rw.OnConnectHub = func() {
			if cd.OnConnectHub != nil {
				cd.OnConnectHub()
			}
		}
		rw.OnLeaveRoom = func() {
			if cd.OnLeaveRoom != nil {
				cd.OnLeaveRoom(room)
			}
		}
		// Member-row activation surfaces the user info dialog (Python
		// show_user_info, Channels.py:2119).
		rw.OnMemberClick = func(nick, hash string) {
			if cd.OnMemberClick != nil {
				cd.OnMemberClick(nick, hash)
			}
		}
		// The composer's slash commands (Python _handle_slash_command,
		// Channels.py:997-1120) delegate to the display-level callbacks.
		rw.OnSendAction = func(text string) error {
			if cd.OnSendAction == nil {
				return nil
			}
			return cd.OnSendAction(text)
		}
		rw.OnSendPing = func() error {
			if cd.OnSendPing == nil {
				return nil
			}
			return cd.OnSendPing()
		}
		rw.OnJoinRoomNamed = func(named string) error {
			if cd.OnJoinRoomNamed == nil {
				return nil
			}
			return cd.OnJoinRoomNamed(named)
		}
		rw.OnClearMessages = func() {
			if cd.OnClearMessages != nil {
				cd.OnClearMessages()
			}
		}
		rw.OnNickInfo = func() (string, bool) {
			if cd.OnNickInfo == nil {
				return "", false
			}
			return cd.OnNickInfo()
		}
		rw.OnSetNick = func(name string) error {
			if cd.OnSetNick == nil {
				return nil
			}
			return cd.OnSetNick(name)
		}
		rw.OnDisconnectHub = func() {
			if cd.OnDisconnectHub != nil {
				cd.OnDisconnectHub()
			}
		}
	}
	cd.roomWidget.SetMessages(msgs)
	// Python _update_peer_info (Channels.py:737-756): the room header reads
	// the hub's live state (advertised name/version, display name, status).
	cd.roomWidget.SetRoomHeader(hv.ServerName(), hv.HubVersion(), hubStatusLabel(hv.Status()))
	cd.paneMode = "room"
	// Python replaces the right pane's content with the room widget, which
	// carries its own borders — the outer placeholder border goes away
	// (item 14: no double border around the room area).
	cd.rightPane.SetBorder(false)
	cd.rightPane.Clear()
	cd.rightPane.AddItem(cd.roomWidget.Widget(), 0, 1, true)
	// Python's selected_key invariant (Channels.py:1722-1724): the shown
	// room's row IS the channels-list selection — the opened room's row
	// carries the list's highlight (the joined-room highlight the user
	// reads). The 2026-09-03 fleet captures showed the Go selection stuck on
	// the hub row while a room was open. Moving the highlight does NOT move
	// the tview focus, so the keyboard stays with the room pane (the
	// eaten-cursor note below still holds).
	for i, e := range cd.hubEntries {
		if e.Kind == RowRoom && e.HubIdx == hubIdx && e.Room == room {
			cd.rooms.SetCurrentItem(i)
			break
		}
	}
}

// RefreshRoomIfVisible reloads the showing room's message buffer and member
// list so new messages and joins appear live (Python RoomWidget
// update_messages + _refresh_users_pane via the delegates), and refreshes the
// room header from the hub's live state (Python _update_peer_info on notify).
func (cd *ChannelsDisplay) RefreshRoomIfVisible(refresh RoomMessagesFunc, members RoomMembersFunc) {
	if cd.paneMode != "room" || cd.selectedHubIdx < 0 || cd.selectedHubIdx >= len(cd.HubViewsCache) {
		return
	}
	room := cd.selectedRoom
	if room == "" {
		return
	}
	if hv := cd.HubViewsCache[cd.selectedHubIdx]; hv != nil {
		cd.roomWidget.SetRoomHeader(hv.ServerName(), hv.HubVersion(), hubStatusLabel(hv.Status()))
	}
	cd.roomWidget.SetMessages(refresh(cd.selectedHubIdx, room))
	if members != nil {
		cd.roomWidget.SetMembers(members(cd.selectedHubIdx, room))
	}
}

// RefreshHubInfoIfVisible re-renders the hub info panel from the current
// HubViewsCache when it is showing, so the status/server/MOTD lines track
// live hub state changes (Python's info panel updates via the delegate).
func (cd *ChannelsDisplay) RefreshHubInfoIfVisible() {
	if cd.paneMode != "info" || cd.selectedHubIdx < 0 || cd.selectedHubIdx >= len(cd.HubViewsCache) {
		return
	}
	cd.ShowHubInfo(cd.selectedHubIdx)
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
		cd.selectedHubIdx = e.HubIdx
		if cd.OnSelectHub != nil {
			cd.OnSelectHub(e.HubIdx)
		}
		if cd.OnShowHubInfo != nil {
			cd.OnShowHubInfo(e.HubIdx)
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
	case tcell.KeyLeft:
		// Python's urwid Columns: the Left from the room view moves focus
		// back to the channels list column (Main.py columns focus chain) —
		// except Left from the USERS pane, which steps back into the room's
		// message body (the room's own Left/Right pane walk, Right being the
		// way into the users pane below).
		if cd.paneMode == "room" && cd.roomWidget != nil && cd.app != nil {
			if cd.roomWidget.usersPaneHasFocus() {
				cd.app.SetFocus(cd.roomWidget.messagesArea)
				return nil
			}
			cd.app.SetFocus(cd.ilb)
			return nil
		}
		return event
	case tcell.KeyRight:
		// Symmetric: the Right from the list returns to the room view or the
		// hub info panel. Right from the room's message body steps INTO the
		// users pane (the room's Left/Right walk: Left above steps back).
		if cd.app != nil && (cd.paneMode == "room" || cd.paneMode == "info") {
			if cd.paneMode == "room" && cd.roomWidget != nil {
				if cd.roomWidget.bodyHasFocus() {
					cd.app.SetFocus(cd.roomWidget.usersList)
					return nil
				}
				cd.app.SetFocus(cd.roomWidget.Widget())
				return nil
			}
			if cd.paneMode == "info" && cd.hubInfo != nil {
				cd.app.SetFocus(cd.hubInfo.Widget())
				return nil
			}
		}
		return event
	case tcell.KeyCtrlD:
		// In the room view C-d belongs to the room composer (Python
		// RoomMessageEdit "ctrl d" → send); pass it through so the room
		// widget's own capture handles it.
		if cd.paneMode == "room" {
			return event
		}
		if cd.OnLeaveRoom != nil {
			cd.OnLeaveRoom(cd.selectedRoom)
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
// toggle_channel_list + _apply_channel_list_visibility (Channels.py:1531-1568):
// Python GUARDS the toggle — while the list is visible and the right pane
// still shows the placeholder (nothing opened yet) Ctrl-Y is a no-op — and
// once applied the layout is [left(36), right(1)] when visible and
// [gutter(1), right(1)] when hidden (show_gutters defaults True).
func (cd *ChannelsDisplay) ToggleChannelListState() {
	// Python toggle_channel_list guard (Channels.py:1533-1534): the list cannot
	// collapse while the right pane still shows the placeholder.
	if cd.channelListVisible && cd.paneMode == "" {
		return
	}
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

// ShowPlaceholder restores the right pane to the "Select or add a hub to
// begin" placeholder (Python show_placeholder, Channels.py:1834-1840).
func (cd *ChannelsDisplay) ShowPlaceholder() {
	cd.paneMode = ""
	// Python's placeholder IS the bordered LineBox (Channels.py:1459), so the
	// right pane border renders only in the placeholder state.
	cd.rightPane.SetBorder(true)
	cd.rightPane.Clear()
	cd.rightPane.AddItem(cd.placeholder, 0, 1, false)
}

// SetMessages replaces the messages view content.
func (cd *ChannelsDisplay) SetMessages(text string) {
	cd.messages.SetText(text)
}

// ToggleCollapse flips the join/leave collapse flag, matching Python's
// toggle_join_part_collapse (Channels.py:1537-1543): when a room widget is
// showing its message buffer re-renders with the new state (Python
// update_messages(replace=True)); then OnToggleCollapse fires.
func (cd *ChannelsDisplay) ToggleCollapse() {
	cd.collapseJoinPart = !cd.collapseJoinPart
	if cd.paneMode == "room" && cd.roomWidget != nil {
		cd.roomWidget.SetCollapseJoinPart(cd.collapseJoinPart)
	}
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

// ShowUserInfoDialog displays user information for a channel member, matching
// Python's show_user_info (Channels.py:2119-2197): the Nick/Identity lines,
// the recalled " LXMF : " line when the peer's identity is cached, the
// "(This is you)" branch for the local user, the "Identity not in local
// cache" branch when no LXMF address could be derived, and the
// Open Conversation(0.55)/spacer/Close(0.40) button row (Close alone takes
// weight 1 in the self and cache-miss branches).
func (cd *ChannelsDisplay) ShowUserInfoDialog(nick, identityHash, lxmfHash string, isSelf bool, onOpenConversation func()) {
	close := cd.closeDialog

	// urwid Pile rows: each entry is one row; a Pile item of plain Text is
	// 1 row, a Columns row is 1 row.
	var rows []tview.Primitive
	addLeft := func(text string) {
		rows = append(rows, NewUrwidLeftText(text))
	}
	addCenter := func(text string) {
		rows = append(rows, NewUrwidCenterText(text))
	}
	addLeft("")
	addLeft(" Nick     : " + nick)
	addLeft(" Identity : " + identityHash)
	if lxmfHash != "" {
		addLeft(" LXMF     : " + lxmfHash)
	}

	var row *urwidColumns
	closeFull := func() *urwidColumns {
		// Python's self / cache-miss branch: Columns([(WEIGHT, 1, Close)]) —
		// a single full-width column carrying the Close button.
		r := newURWIDColumns(0, NewUrwidButton("Close").SetSelectedFunc(close))
		r.SetWeight(0, 1)
		return r
	}
	if isSelf {
		addLeft("")
		addCenter(" (This is you)")
		addLeft("")
		row = closeFull()
	} else if lxmfHash == "" {
		addLeft("")
		addCenter(" Identity not in local cache;")
		addCenter(" conversation can't be opened until")
		addCenter(" the peer announces.")
		addLeft("")
		row = closeFull()
	} else {
		addLeft("")
		openBtn := NewUrwidButton("Open Conversation").SetSelectedFunc(func() {
			close()
			if onOpenConversation != nil {
				onOpenConversation()
			}
		})
		closeBtn := NewUrwidButton("Close").SetSelectedFunc(close)
		row = CreateUrwidButtonRow(openBtn, closeBtn)
		// Python weights: Open Conversation 0.55, spacer 0.05, Close 0.40.
		// CreateUrwidButtonRow uses 0.45/0.10; re-weight to match.
		row.SetWeight(0, 11) // 0.55
		row.SetWeight(1, 1)  // 0.05
		row.SetWeight(2, 8)  // 0.40
	}
	rows = append(rows, row)

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	for _, r := range rows {
		layout.AddItem(r, 1, 0, false)
	}

	dialog := NewDialogLineBox("User Info", layout, close)
	cd.showDialogOverlay(dialog, len(rows)+2)
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
	// Python new_hub_dialog (Channels.py:1921-1958): ONE dialog titled
	// "New Hub" holding the address field, the display-name field, a blank
	// row, the error line, and an Add/Back button row. The Add handler
	// validates the hash (0x-stripped, lowercase, 16 bytes) and on failure
	// renders "Could not add hub: <error>" on the error line instead of
	// chaining a second dialog.
	tc := GetThemeColors(cd.app.Theme)
	eHash := tview.NewInputField().SetLabel("Hub address : ").SetText("")
	eHash.SetFieldBackgroundColor(tc["msg_editor_bg"])
	eHash.SetFieldTextColor(tc["msg_editor_fg"])
	eName := tview.NewInputField().SetLabel("Display name: ").SetText("")
	eName.SetFieldBackgroundColor(tc["msg_editor_bg"])
	eName.SetFieldTextColor(tc["msg_editor_fg"])
	errorText := tview.NewTextView().SetDynamicColors(true)
	errorText.SetText("")

	confirm := func() {
		// Python confirmed() (Channels.py:1929-1942): strip 0x, lowercase,
		// bytes.fromhex, require TRUNCATED_HASHLENGTH//8 bytes; name or None
		// → rrc.add_hub + update_list. Failures land on the error line.
		hashText := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(eHash.GetText()), "0x"))
		hashBytes, err := hex.DecodeString(hashText)
		if err != nil {
			errorText.SetText("[#ff5555]Could not add hub: invalid hexadecimal hash[-]")
			return
		}
		if len(hashBytes) != 16 {
			errorText.SetText("[#ff5555]Could not add hub: Hash length must be 16 bytes[-]")
			return
		}
		name := strings.TrimSpace(eName.GetText())
		cd.closeDialog()
		if cd.OnAddHub != nil {
			// Python new_hub_dialog confirmed(): rrc.add_hub(hh, name=nm) then
			// update_list (Channels.py:1038-1040).
			cd.OnAddHub(hashBytes, "rrc.hub", name)
		}
	}

	addBtn := NewUrwidButton("Add").SetSelectedFunc(confirm)
	backBtn := NewUrwidButton("Back").SetSelectedFunc(func() { cd.closeDialog() })
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(eHash, 1, 0, true).
		AddItem(eName, 1, 0, false).
		AddItem(tview.NewTextView().SetText(""), 1, 0, false).
		AddItem(errorText, 1, 0, false).
		AddItem(CreateUrwidButtonRow(addBtn, backBtn), 1, 0, false)
	dialog := NewDialogLineBox("New Hub", layout, cd.closeDialog)
	// 5 layout rows + the 2 border rows.
	cd.showDialogOverlay(dialog, 7)
	// Python's Pile traversal: Down/Up (and Tab) walk address → name →
	// buttons; Esc dismisses (wireDialogNav, the shared dialog-nav latch).
	wireDialogNav(cd.app, cd.closeDialog, []tview.Primitive{eHash, eName, addBtn, backBtn})
}

// JoinRoomDialog shows the dialog to join a room on the selected hub (Python
// Channels.join_room_dialog), overlaid on the channels display (60% width).
func (cd *ChannelsDisplay) JoinRoomDialog() {
	if cd.app == nil {
		return
	}
	// Python join_room_dialog (Channels.py:1928-1969): the dialog opens with
	// a " Hub : <name>" line so the user can see WHICH hub the room joins
	// (the selected row's hub, or the first hub), then add_room + join_room +
	// _select_room in the wiring.
	hubName := ""
	if entry, ok := cd.SelectedEntry(); ok && entry.HubIdx >= 0 && entry.HubIdx < len(cd.HubViewsCache) {
		hubName = cd.HubViewsCache[entry.HubIdx].Name()
	} else if len(cd.HubViewsCache) > 0 {
		hubName = cd.HubViewsCache[0].Name()
	}
	// Python join_room_dialog's form (Channels.py:1934-1975): the room field,
	// the keyed-room checkbox, and the key field (masked) with the error line
	// and Join/Back buttons — richer than the single-input helper.
	tc := GetThemeColors(cd.app.Theme)
	eRoom := tview.NewInputField().SetLabel("Room : #").SetText("")
	eRoom.SetFieldBackgroundColor(tc["msg_editor_bg"])
	eRoom.SetFieldTextColor(tc["msg_editor_fg"])
	eKey := tview.NewInputField().SetLabel("Key  : ").SetMaskCharacter('*').SetText("")
	eKey.SetFieldBackgroundColor(tc["msg_editor_bg"])
	eKey.SetFieldTextColor(tc["msg_editor_fg"])
	cbKey := tview.NewCheckbox().SetLabel("Keyed room (+k)")
	// Python's update_key_visibility: the key field swaps with a blank
	// placeholder depending on the checkbox state.
	keyPlaceholder := tview.NewFlex()
	cbKey.SetChangedFunc(func(checked bool) {
		// Python update_key_visibility: the key field appears in place of the
		// blank placeholder when the checkbox is checked.
		if checked {
			keyPlaceholder.RemoveItem(eKey)
			keyPlaceholder.AddItem(eKey, 1, 0, true)
		} else {
			keyPlaceholder.RemoveItem(eKey)
		}
	})
	row := CreateUrwidButtonRow(
		NewUrwidButton("Join").SetSelectedFunc(func() {
			room := strings.TrimSpace(eRoom.GetText())
			if room == "" {
				return
			}
			key := ""
			if cbKey.IsChecked() {
				key = strings.TrimSpace(eKey.GetText())
			}
			cd.closeDialog()
			if cd.OnJoinRoomSubmitted != nil {
				cd.OnJoinRoomSubmitted(room, key)
			}
		}),
		NewUrwidButton("Back").SetSelectedFunc(func() { cd.closeDialog() }))
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText(" Hub : "+hubName).SetTextColor(tcell.ColorDefault), 1, 0, false).
		AddItem(eRoom, 1, 0, true).
		AddItem(cbKey, 1, 0, false).
		AddItem(keyPlaceholder, 1, 0, false).
		AddItem(tview.NewTextView().SetText(""), 1, 0, false).
		AddItem(row, 1, 0, false)
	dialog := NewDialogLineBox("Add Room on "+hubName, layout, cd.closeDialog)
	// 6 layout rows + the 2 border rows: the earlier height 7 clipped the
	// button row onto the bottom border.
	cd.showDialogOverlay(dialog, 8)
	// Python's update_key_visibility placeholder swap (Channels.py:1946-1955)
	// removes the key field from the Pile while "Keyed room (+k)" is unchecked,
	// so Tab from the checkbox reaches the button row. The dynamic nav mirrors
	// that: the DETACHED key field is skipped instead of swallowing every
	// further key.
	keyActive := func(p tview.Primitive) bool {
		return p != eKey || flexContains(keyPlaceholder, eKey)
	}
	wireDialogNavDynamic(cd.app, cd.closeDialog, []tview.Primitive{eRoom, cbKey, eKey, layout.GetItem(5)}, keyActive)
}

// RemoveSelectedDialog shows the confirm dialog for the selected hub/room row
// (Python Channels.remove_selected_dialog, Channels.py:1882-1925): a room row
// prompts "Leave and remove room\n#<room>\non hub <name>?"; a hub header row
// prompts "Remove hub\n<name>\nfrom this client?\n All Message history will be
// discarded."; the overlay is titled "?" and Yes fires OnRemoveSelected after
// closing. With no hub-bearing row selected it is a no-op.
func (cd *ChannelsDisplay) RemoveSelectedDialog() {
	if cd.app == nil {
		return
	}
	entry, ok := cd.SelectedEntry()
	if !ok || entry.Kind == RowSpacer {
		return
	}
	hubName := ""
	if entry.HubIdx >= 0 && entry.HubIdx < len(cd.HubViewsCache) {
		hubName = cd.HubViewsCache[entry.HubIdx].Name()
	}
	room := ""
	prompt := "Remove hub\n" + hubName + "\nfrom this client?\n All Message history will be discarded."
	if entry.Kind == RowRoom {
		room = entry.Room
		prompt = "Leave and remove room\n#" + room + "\non hub " + hubName + "?"
	}
	cd.showDialogOverlayConfirm("?", prompt+"\n", func() {
		if cd.OnRemoveSelected != nil {
			cd.OnRemoveSelected(entry.HubIdx, room)
		}
	}, nil)
}

// EditHubDialog shows the edit-hub dialog for the selected hub (Python
// Channels.edit_hub_dialog, Channels.py:2005-2060): the hub's address and
// server lines, a divider, a "Display name : " input pre-filled with the hub's
// display name, the three auto checkboxes seeded from the hub's live states,
// and Save/Back buttons. Save fires OnEditHubSubmitted with the edited values
// after closing (Python confirmed(): a blank name falls back to the hub's
// existing name). With no hub row selected it is a no-op.
func (cd *ChannelsDisplay) EditHubDialog() {
	if cd.app == nil {
		return
	}
	entry, ok := cd.SelectedEntry()
	if !ok || entry.Kind != RowHub || entry.HubIdx < 0 || entry.HubIdx >= len(cd.HubViewsCache) {
		return
	}
	hv := cd.HubViewsCache[entry.HubIdx]

	server := hv.ServerName()
	if server == "" {
		server = "(unknown until connected)"
	}
	g := cd.app.Glyphs

	nameInput := tview.NewInputField()
	nameInput.SetLabel("Display name : ")
	nameInput.SetText(hv.Name())
	nameInput.SetFieldBackgroundColor(tcell.ColorDefault)
	nameInput.SetFieldTextColor(tcell.ColorDefault)

	cbRcn := NewUrwidCheckBox("Auto-reconnect on disconnect", hv.AutoReconnect())
	cbList := NewUrwidCheckBox("Auto-fetch room list on connect", hv.AutoList())
	cbWho := NewUrwidCheckBox("Auto-fetch members on room join", hv.AutoWho())

	close := cd.closeDialog
	submit := func() {
		// Python confirmed(): nm = e_name.get_edit_text().strip() or hub.name.
		name := strings.TrimSpace(nameInput.GetText())
		if name == "" {
			name = hv.Name()
		}
		close()
		if cd.OnEditHubSubmitted != nil {
			cd.OnEditHubSubmitted(entry.HubIdx, name, cbRcn.IsChecked(), cbList.IsChecked(), cbWho.IsChecked())
		}
	}
	saveBtn := NewUrwidButton("Save").SetSelectedFunc(submit)
	backBtn := NewUrwidButton("Back").SetSelectedFunc(close)
	// Python button order: Save(0.45), spacer(0.10), Back(0.45).
	row := CreateUrwidButtonRow(saveBtn, backBtn)
	nameInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			submit()
		}
	})

	address := NewUrwidLeftText(" Address : " + hv.AddressHex())
	serverLine := NewUrwidLeftText(" Server  : " + server)
	blank1 := tview.NewBox()
	blank2 := tview.NewBox()
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(address, 1, 0, false).
		AddItem(serverLine, 1, 0, false).
		AddItem(newDividerRow(g["divider1"]), 1, 0, false).
		AddItem(nameInput, 1, 0, true).
		AddItem(blank1, 1, 0, false).
		AddItem(cbRcn, 1, 0, false).
		AddItem(cbList, 1, 0, false).
		AddItem(cbWho, 1, 0, false).
		AddItem(blank2, 1, 0, false).
		AddItem(row, 1, 0, false)
	dialog := NewDialogLineBox("Edit Hub", layout, close)
	// PACK height: 9 content rows + button row + 2 border.
	cd.showDialogOverlay(dialog, 12)
	wireDialogNav(cd.app, close, []tview.Primitive{nameInput, cbRcn, cbList, cbWho, saveBtn, backBtn})
}
