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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConversationInfo holds summary info for a conversation list entry.
type ConversationInfo struct {
	SourceHash   string
	DisplayName  string
	TrustLevel   string
	LastMessage  string
	LastTime     time.Time
	Unread       bool
	UnreadCount  int
	MessageCount int
	Failed       bool
	FailedCount  int
	Pinned       bool
	SortRank     *int
}

// ConversationsDisplay shows the conversation list and message view.
// Matches Python's ConversationsDisplay (Conversations.py:205-236): a two-pane
// Columns layout — a bordered left pane titled "Conversations" holding the
// Trusted/Untrusted tab buttons, the conversation list (an IndicativeListBox
// with "───"/"▲"/"▼" scroll indicators) and a "Last sync:" footer; and a
// bordered right pane showing the selected conversation (or "No conversation
// selected" when none). There is NO outer border around the two panes; each
// pane carries its own LineBox, matching the original.
type ConversationsDisplay struct {
	app                 *App
	widget              *tview.Flex
	content             *tview.Flex
	leftPanel           *tview.Flex
	list                *tview.List
	ilb                 *IndicativeListBox
	detail              *tview.TextView
	tabBar              *tabBarWidget
	tabTrusted          *UrwidButton
	tabUntrusted        *UrwidButton
	showBlockedCheckbox *tview.Checkbox
	syncStatus          *tview.TextView
	conversations       []ConversationInfo
	listWidth           int
	fullscreen          bool
	selected            int
	showTrusted         bool   // true = trusted tab, false = untrusted
	showBlocked         bool   // show blocked peers in untrusted tab
	currentConversation string // source hash of the conversation shown in the right pane (suppresses its unread/failed badge); "" when none
	dialogOpen          bool
	ingestURIValue      string
	shortcutFocus       string // "list" (default), "editor", or "body" — selects the shortcut bar (Conversations.py:1765-1779)

	// Keyboard shortcut callbacks (Python: ConversationsArea.keypress)
	OnEditPeerInfo     func()
	OnDeleteConv       func()
	OnNewConv          func()
	OnIngestURI        func()
	OnSync             func()
	OnToggleFullscreen func()
	OnToggleSort       func()
	OnShowQR           func()
	OnSyncRequested    func(limit int)

	// Trust banner button callbacks (fired by the in-conversation trust
	// banner; Python _on_trust_click/_on_block_click/_on_ignore_click).
	OnTrustPeer  func(sourceHash string)
	OnBlockPeer  func(sourceHash string)
	OnIgnorePeer func(sourceHash string)

	// OnSend is the display-level send hook fired by the conversation
	// widget's composer (C-d). The wiring layer (cmd/gonomadnet/textui.go)
	// connects it to App.SendConversation so a composed message is built
	// into an outbound lxmf.Message and dispatched through the router.
	// The source hash of the open conversation is forwarded along with the
	// composed content/title.
	OnSend func(sourceHash, content, title string, attachments []string)

	// OnLoadMessages supplies the ConversationMessages for an open
	// conversation (the wiring layer maps App.ConversationMessages →
	// []ConversationMessage). Called from DisplayConversation to populate
	// the widget, and from ReloadCurrentMessages after a send so the new
	// message appears.
	OnLoadMessages func(sourceHash string) []ConversationMessage
	// OnOwnHash supplies this app's LXMF destination hash so the
	// LXMessageWidget header can distinguish outbound (source==own) from
	// inbound messages (Python compares app.lxmf_destination.hash,
	// Conversations.py:2607).
	OnOwnHash func() []byte
	// OnTimeFormat supplies the configured strftime format (Python
	// app.time_format). When nil the widget's "%Y-%m-%d %H:%M:%S" default
	// applies.
	OnTimeFormat func() string
	// OnStampCost supplies the peer's outbound LXMF stamp cost for the peer-info
	// header bar (Python _update_peer_info, Conversations.py:2103-2105). nil
	// omits the "Stamp: N" segment.
	OnStampCost func(sourceHash string) *int
	// OnHops supplies the transport hop count to the peer for the peer-info
	// header bar (Python _update_peer_info, Conversations.py:2107-2112). nil
	// renders "unknown".
	OnHops func(sourceHash string) *int

	// OnPaperMessage performs a paper (offline) message output for the open
	// conversation (Python paper_output, Conversations.py:2474-2503). The
	// wiring layer maps action ("PrintQR"/"SaveQR"/"SaveURI") to
	// App.PaperMessage and returns the saved path + ok. OnPaperMessageSaved /
	// OnPaperMessageFailed render the result dialogs (paper_message_saved /
	// paper_message_failed).
	OnPaperMessage       func(sourceHash, action, content, title string) (path string, ok bool)
	OnPaperMessageSaved  func(path string)
	OnPaperMessageFailed func()
	// OnSaveAttachments copies the selected received attachments to the
	// download directory (Python do_save / _copy_attachment_to_dest,
	// Conversations.py:2368-2394). The wiring layer maps the selected refs
	// (each carrying the owning message's LXMF hash + field index) to
	// App.SaveConversationAttachments and reports the saved paths for the
	// status dialog.
	OnSaveAttachments func(sourceHash string, refs []AttachmentRef) (saved []string, failed int)

	// currentWidget is the ConversationWidget currently shown in the right
	// pane (nil when the empty detail placeholder is shown). Kept so the
	// wiring layer can refresh the open conversation after a send and so
	// tests can drive the composer.
	currentWidget *ConversationWidget

	// Sync dialog live-refresh state (Python update_sync_dialog,
	// Conversations.py:1566-1575). The status/progress widgets are held so
	// updateSyncProgress can mutate them in place each tick; syncHooks supplies
	// the live values; syncStop cancels the refresh goroutine on dismiss.
	syncStatusText  *tview.TextView
	syncProgressBox *tview.TextView
	syncSyncBtn     *tview.Button
	syncHooks       SyncDialogHooks
	syncStop        chan struct{}
	syncWG          sync.WaitGroup
	syncMutex       sync.Mutex
}

// NewConversationsDisplay creates a new conversations display.
func NewConversationsDisplay(app *App, convs []ConversationInfo) *ConversationsDisplay {
	cd := &ConversationsDisplay{
		app:           app,
		conversations: convs,
		selected:      -1,
		showTrusted:   true,
	}

	// Tab bar: two TabButtons "[ Trusted (N) ]" / "[ Untrusted (N) ]" in a
	// single Columns row with one dividing space, matching Python's tab_bar
	// (Conversations.py:392-398). No digit prefixes; unread/failed counts get
	// an envelope glyph (Python _label, Conversations.py:458-465). Each button
	// is weight 1 so the brackets fill the left-pane inner width.
	cd.tabTrusted = NewTabButton("Trusted (0)")
	cd.tabTrusted.SetSelectedFunc(func() { cd.SetShowTrusted(true) })
	cd.tabUntrusted = NewTabButton("Untrusted (0)")
	cd.tabUntrusted.SetSelectedFunc(func() { cd.SetShowTrusted(false) })
	cd.tabBar = newTabBarWidget(cd.tabTrusted, cd.tabUntrusted)

	// Conversation list, wrapped in an IndicativeListBox so the centered
	// "───"/"▲"/"▼" scroll indicators render above and below it (Python
	// IndicativeListBox, Conversations.py:403-408).
	cd.list = tview.NewList()
	cd.list.SetHighlightFullLine(true)
	ApplyListFocusStyle(cd.list, cd.app.Theme)
	cd.ilb = NewIndicativeListBox(cd.list)

	cd.populateList()
	cd.refreshTabBar()

	// "Show blocked (N)" checkbox, shown only in the Untrusted tab (Python
	// _apply_pile_layout, Conversations.py:317-318).
	cd.showBlockedCheckbox = tview.NewCheckbox().
		SetLabel("Show blocked (0)").
		SetChangedFunc(func(checked bool) { cd.SetShowBlocked(checked) })

	// "Last sync: never" footer (Python _sync_status_line, Conversations.py:
	// 517-545), left-aligned in the shortcutbar style.
	cd.syncStatus = tview.NewTextView()
	cd.syncStatus.SetTextAlign(tview.AlignLeft)
	cd.syncStatus.SetTextColor(tcell.NewHexColor(0xaaaaaa))
	cd.syncStatus.SetText(cd.syncStatusLine())

	// Detail view (right pane). Empty state matches Python's ConversationWidget
	// (None): a bordered LineBox with "\n  No conversation selected"
	// (Conversations.py:1884-1886).
	cd.detail = tview.NewTextView()
	cd.detail.SetDynamicColors(true)
	cd.detail.SetScrollable(true)
	cd.detail.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	cd.detail.SetTextAlign(tview.AlignLeft)
	cd.detail.SetBorder(true)
	cd.detail.SetText("\n  No conversation selected")

	// Left pane: bordered, titled "Conversations", holding the tab bar, the
	// (optional) show-blocked checkbox, the list, and the sync footer.
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow)
	leftPanel.SetBorder(true)
	SetTitledBorder(leftPanel, "Conversations")
	cd.leftPanel = leftPanel
	cd.listWidth = 52
	cd.applyPileLayout()

	// Two-pane Columns: left list pane (fixed 52) + right detail pane. No outer
	// border — each pane carries its own, matching Python's columns_widget
	// (Conversations.py:221-229).
	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(leftPanel, cd.listWidth, 0, true)
	content.AddItem(cd.detail, 0, 1, false)
	cd.content = content
	cd.widget = content

	// Set up list callback
	cd.list.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		cd.showDetail(i)
	})

	// Set up keyboard shortcuts matching Python's ConversationsArea.keypress()
	cd.widget.SetInputCapture(cd.handleInput)

	// Wire focus-region shortcut bars: every focusable list-pane primitive
	// switches the footer to the "list" shortcut bar when it gains focus
	// (Python shortcuts() focus_path[0]!=1 → list_shortcuts,
	// Conversations.py:1765-1779). The conversation widget's editor/body
	// primitives get their own focus funcs in DisplayConversation.
	listFocus := func() { cd.setShortcutRegion("list") }
	cd.list.SetFocusFunc(listFocus)
	cd.tabTrusted.SetFocusFunc(listFocus)
	cd.tabUntrusted.SetFocusFunc(listFocus)
	cd.showBlockedCheckbox.SetFocusFunc(listFocus)

	return cd
}

// SetShortcutFocus sets which of the three Conversations shortcut bars
// GetShortcutText returns, matching the focus-path dispatch in Python
// Conversations.py:1765-1779: "list" when the list pane has focus, "editor"
// when the message editor (frame footer) has focus, "body" otherwise.
func (cd *ConversationsDisplay) SetShortcutFocus(region string) {
	cd.shortcutFocus = region
}

// setShortcutRegion records the active focus region and refreshes the main
// display's shortcut bar so the footer text + wrapped height track the focused
// pane, mirroring Python's focus-path dispatch (Conversations.py:1765-1779)
// feeding Main.update_active_shortcuts. It is wired as the SetFocusFunc of
// every focusable Conversations primitive (list pane → "list"; conversation
// editor → "editor"; message body → "body") so the bar follows focus
// automatically with no per-key handling.
func (cd *ConversationsDisplay) setShortcutRegion(region string) {
	cd.shortcutFocus = region
	if cd.app != nil && cd.app.Main != nil {
		// refreshShortcuts (TryLock) avoids deadlocking when the callback
		// fires while MainDisplay.mu is held — notably from SetDisplay's
		// SwitchToPage focus chain during boot wiring.
		cd.app.Main.refreshShortcuts()
	}
}

// GetShortcutText returns the appropriate shortcut bar text for the current
// focus context. Matches Python's shortcuts() method at Conversations.py:1765
// which returns list/editor/body shortcut sets based on which pane has focus.
// An open dialog suppresses the bar.
func (cd *ConversationsDisplay) GetShortcutText() string {
	if cd.dialogOpen {
		return ""
	}
	switch cd.shortcutFocus {
	case "editor":
		return "[C-d] Send  [C-p] Paper Msg  [C-t] Title  [C-f] Attach  [C-s] Save  [Tab] ↑ Messages"
	case "body":
		return "[C-s] Save  [C-u] Purge  [C-o] Sort  [C-x] Clear History  [C-g] Fullscreen  [C-w] Close  [Tab] ↓ Editor"
	default: // "list"
		return "[C-e] Peer Info  [C-x] Delete  [C-r] Sync  [C-n] New  [C-u] Ingest URI  [C-o] Sort  [C-p] My LXMF  [C-g] Fullscreen"
	}
}

// handleInput processes keyboard shortcuts for the conversations display.
// Matches Python's ConversationsArea.keypress() at Conversations.py:88.
func (cd *ConversationsDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlE:
		if cd.OnEditPeerInfo != nil {
			cd.OnEditPeerInfo()
		}
		return nil
	case tcell.KeyCtrlX:
		if cd.OnDeleteConv != nil {
			cd.OnDeleteConv()
		}
		return nil
	case tcell.KeyCtrlN:
		if cd.OnNewConv != nil {
			cd.OnNewConv()
		}
		return nil
	case tcell.KeyCtrlU:
		if cd.OnIngestURI != nil {
			cd.OnIngestURI()
		}
		return nil
	case tcell.KeyCtrlR:
		if cd.OnSync != nil {
			cd.OnSync()
		}
		return nil
	case tcell.KeyCtrlG:
		cd.ToggleFullscreen()
		return nil
	case tcell.KeyCtrlO:
		if cd.OnToggleSort != nil {
			cd.OnToggleSort()
		}
		return nil
	case tcell.KeyCtrlP:
		if cd.OnShowQR != nil {
			cd.OnShowQR()
		}
		return nil
	}

	return event
}

// GetSelectedConversation returns the currently selected ConversationInfo.
func (cd *ConversationsDisplay) GetSelectedConversation() (ConversationInfo, bool) {
	idx := cd.list.GetCurrentItem()
	if idx < 0 || idx >= len(cd.conversations) {
		return ConversationInfo{}, false
	}
	return cd.conversations[idx], true
}

// DisplayConversation replaces the detail panel with a ConversationWidget
// for the given source hash. Matches Python's display_conversation at
// Conversations.py:1630.
func (cd *ConversationsDisplay) DisplayConversation(sourceHash string) {
	cw := NewConversationWidget(cd.app, sourceHash)
	// Seed peer-info + trust-banner data from the known conversation info.
	for _, conv := range cd.conversations {
		if conv.SourceHash == sourceHash {
			cw.TrustLevel = conv.TrustLevel
			cw.DisplayName = conv.DisplayName
			break
		}
	}
	cw.refreshTrustBanner()
	// Wire the trust banner buttons to the display-level peer callbacks so
	// the app layer (which owns the directory) can trust/block the peer.
	cw.OnTrust = func() {
		if cd.OnTrustPeer != nil {
			cd.OnTrustPeer(sourceHash)
		}
		cw.SetTrustLevel("trusted")
	}
	cw.OnBlock = func() {
		if cd.OnBlockPeer != nil {
			cd.OnBlockPeer(sourceHash)
		}
	}
	cw.OnIgnore = func() {
		if cd.OnIgnorePeer != nil {
			cd.OnIgnorePeer(sourceHash)
		}
	}
	cw.OnClose = func() {
		// Restore the empty detail pane: remove the conversation widget (the
		// content Flex's index-1 item) and re-add the bordered detail view.
		cd.content.RemoveItem(cd.content.GetItem(1))
		cd.content.AddItem(cd.detail, 0, 1, false)
	}
	cw.OnSaveFocusedAttachments = func(refs []AttachmentRef) {
		// Python save_focused_attachments hands the collected refs to a dialog.
		// The display-level SaveAttachmentsDialog renders the checkbox list and
		// performs the copy directly via OnSaveAttachments (each ref carries its
		// MessageHash + FieldIndex so the app layer can locate the extracted
		// attachment file), keeping the dialog open to show the result status
		// (Python do_save, Conversations.py:2368-2391).
		cd.SaveAttachmentsDialog(sourceHash, refs)
	}
	cw.OnAttach = func() {
		// Python attach_file (Conversations.py:2438) opens a file browser; the
		// display's AttachFileDialog is the input-dialog equivalent. On
		// selection, ConfirmAttachFile fires OnAttachFiles so the app layer can
		// stage the pending attachment.
		cd.AttachFileDialog("", func(path string) {
			if path == "" {
				return
			}
			cw.ConfirmAttachFile([]string{path})
		})
	}
	cw.OnPaperMessageRequested = func() {
		// Python paper_message (Conversations.py:2505) shows the output-method
		// dialog; the display renders it. Each choice fires the matching widget
		// action, which calls OnPaperMessage for the app layer to act on.
		cd.PaperMessageDialog(
			func() { cw.PaperMessagePrintQR() },
			func() { cw.PaperMessageSaveQR() },
			func() { cw.PaperMessageSaveURI() },
		)
	}
	cw.OnPaperMessage = func(action, content, title string) (string, bool) {
		// Forward the open conversation's source hash so the wiring layer can
		// build the paper LXMF message via App.PaperMessage (the C-p path,
		// Conversations.py:2474-2503).
		if cd.OnPaperMessage != nil {
			return cd.OnPaperMessage(sourceHash, action, content, title)
		}
		return "", false
	}
	cw.OnPaperMessageSaved = func(path string) {
		if cd.OnPaperMessageSaved != nil {
			cd.OnPaperMessageSaved(path)
		}
	}
	cw.OnPaperMessageFailed = func() {
		if cd.OnPaperMessageFailed != nil {
			cd.OnPaperMessageFailed()
		}
	}
	cw.OnSend = func(content, title string, attachments []string) {
		// Forward the open conversation's source hash so the wiring layer can
		// build and dispatch the outbound message via App.SendConversation
		// (the C-d send path, Conversations.py:1834-1841).
		if cd.OnSend != nil {
			cd.OnSend(sourceHash, content, title, attachments)
		}
	}
	// Inject the app's own LXMF hash and time format so the LXMessageWidget
	// header can tell outbound from inbound and format timestamps like the
	// Python original.
	if cd.OnOwnHash != nil {
		cw.OwnHash = cd.OnOwnHash()
	}
	if cd.OnTimeFormat != nil {
		cw.TimeFormat = cd.OnTimeFormat()
	}
	// Inject the RNS-dependent peer-info fields (Python _update_peer_info,
	// Conversations.py:2103-2112): the outbound stamp cost and the transport
	// hop count. The wiring layer resolves these from the LXMF router and RNS
	// transport; nil leaves the "Stamp:" segment off / hops as "unknown".
	if cd.OnStampCost != nil {
		cw.StampCost = cd.OnStampCost(sourceHash)
	}
	if cd.OnHops != nil {
		cw.Hops = cd.OnHops(sourceHash)
	}
	cw.updatePeerInfo()
	// Wire focus-region shortcut bars for the conversation widget's editor and
	// message body (Python shortcuts() frame.focus_position dispatch,
	// Conversations.py:1772-1779): editor/title editor (frame footer) →
	// "editor"; message list (frame body) → "body".
	cw.editor.SetFocusFunc(func() { cd.setShortcutRegion("editor") })
	cw.titleEditor.SetFocusFunc(func() { cd.setShortcutRegion("editor") })
	cw.messageList.SetFocusFunc(func() { cd.setShortcutRegion("body") })
	// Populate the message list from the wiring layer (mirrors Python
	// ConversationWidget.__init__ calling update_message_widgets,
	// Conversations.py:1894).
	if cd.OnLoadMessages != nil {
		cw.SetMessages(cd.OnLoadMessages(sourceHash))
	}
	cd.currentWidget = cw
	cd.content.RemoveItem(cd.detail)
	cd.content.AddItem(cw.Widget(), 0, 1, true)
}

// ReloadCurrentMessages re-fetches and re-renders the message list for the
// currently-open conversation. The wiring layer calls this after a send (and
// on any conversation-changed callback) so a just-sent message appears in the
// open view without re-opening it. No-op when no conversation is open or no
// loader is wired.
func (cd *ConversationsDisplay) ReloadCurrentMessages() {
	if cd.currentWidget == nil || cd.OnLoadMessages == nil {
		return
	}
	cd.currentWidget.SetMessages(cd.OnLoadMessages(cd.currentWidget.source))
}

// populateList fills the list based on current tab (trusted/untrusted).
func (cd *ConversationsDisplay) populateList() {
	cd.list.Clear()

	var glyphs GlyphSet
	if cd.app != nil {
		glyphs = cd.app.Glyphs
	}
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	for _, conv := range cd.conversations {
		if cd.showTrusted && conv.TrustLevel != "trusted" {
			continue
		}
		if !cd.showTrusted && conv.TrustLevel == "trusted" {
			continue
		}

		main := conversationRowMain(conv, glyphs, cd.currentConversation)
		secondary := conversationRowSecondary(conv)
		cd.list.AddItem(main, secondary, 0, nil)
	}

	if cd.list.GetItemCount() == 0 {
		emptyMsg := "No trusted conversations"
		if !cd.showTrusted {
			emptyMsg = "No untrusted conversations"
		}
		// Leave the List empty and render the message as a centered
		// placeholder (matching Python's `[urwid.Text(empty_label,
		// align='center')]` body, Conversations.py:496) rather than a
		// left-aligned list item.
		if cd.ilb != nil {
			cd.ilb.SetEmptyText(emptyMsg)
		}
	} else if cd.ilb != nil {
		cd.ilb.SetEmptyText("")
	}
}

// trustSymbol returns the trust-level glyph for a conversation row, mirroring
// Python's conversation_list_widget symbol selection (Conversations.py:1697-
// 1716): cross for untrusted, "?" for unknown, check for trusted, warning for
// warning (and the same for any unrecognized level).
func trustSymbol(trustLevel string, glyphs GlyphSet) string {
	switch trustLevel {
	case "untrusted":
		return glyphs["cross"]
	case "unknown":
		return "?"
	case "trusted":
		return glyphs["check"]
	case "warning":
		return glyphs["warning"]
	default:
		return glyphs["warning"]
	}
}

// conversationRowMain builds the first line of a conversation list row,
// mirroring Python's conversation_list_widget (Conversations.py:1687-1755).
// The line is: [pin] symbol [name] [<hash>] [badge] where <hash> is appended
// for non-trusted peers, and the badge is " ⚠ (N)" for failed or " ✉ (N)" for
// unread (failed takes precedence). The badge is suppressed for the
// conversation currently displayed in the right pane (currentConversation).
func conversationRowMain(conv ConversationInfo, glyphs GlyphSet, currentConversation string) string {
	head := trustSymbol(conv.TrustLevel, glyphs)
	if conv.Pinned {
		pin, ok := glyphs["pin"]
		if !ok || pin == "" {
			pin = "*"
		}
		head = pin + " " + head
	}
	if conv.DisplayName != "" {
		head += " " + conv.DisplayName
	}
	if conv.TrustLevel != "trusted" {
		head += " <" + conv.SourceHash + ">"
	}
	if conv.FailedCount > 0 && conv.SourceHash != currentConversation {
		head += " " + glyphs["warning"] + " (" + strconv.Itoa(conv.FailedCount) + ")"
	} else if conv.UnreadCount > 0 && conv.SourceHash != currentConversation {
		head += " " + glyphs["unread"] + " (" + strconv.Itoa(conv.UnreadCount) + ")"
	}
	return head
}

// conversationRowSecondary builds the second line of a conversation list row:
// "  "+relative_time(last_activity), mirroring Python (Conversations.py:1751).
// It returns "" when there is no last activity (last_activity <= 0), matching
// Python's `if last_activity > 0` guard.
func conversationRowSecondary(conv ConversationInfo) string {
	if conv.LastTime.IsZero() {
		return ""
	}
	return "  " + relativeTime(conv.LastTime)
}

// Widget returns the tview primitive for this display.
func (cd *ConversationsDisplay) Widget() tview.Primitive {
	return cd.widget
}

// showDetail displays the conversation detail in the right panel.
func (cd *ConversationsDisplay) showDetail(idx int) {
	if idx < 0 || idx >= len(cd.conversations) {
		return
	}

	conv := cd.conversations[idx]
	// Mark this conversation as the currently-displayed one so its
	// unread/failed badge is suppressed in the list (Python parity,
	// Conversations.py:1743-1749).
	prev := cd.currentConversation
	cd.currentConversation = conv.SourceHash
	if prev != cd.currentConversation {
		cd.populateList()
	}
	cd.detail.SetText(fmt.Sprintf(
		"[::b]%s[-]\n\nTrust: %s\nMessages: %d\nLast: %s\n\n[gray]Select a message to read[-]",
		conv.DisplayName,
		conv.TrustLevel,
		conv.MessageCount,
		relativeTime(conv.LastTime),
	))
}

// relativeTime formats a timestamp as a relative string.
// Delegates to RelativeTime for consistent behavior.
func relativeTime(t time.Time) string {
	return RelativeTime(t)
}

// SortMode represents the conversation list sort order.
type SortMode int

const (
	// SortRecent sorts conversations by most recent activity first.
	SortRecent SortMode = iota
	// SortName sorts conversations alphabetically by display name.
	SortName
)

// ToggleSortMode returns the alternate sort mode.
func ToggleSortMode(mode SortMode) SortMode {
	if mode == SortRecent {
		return SortName
	}
	return SortRecent
}

// SortConversations sorts conversations by the given mode, pinning
// conversations with SortRank != nil to the top. Pinned conversations
// are sorted among themselves by the same mode.
func SortConversations(convs []ConversationInfo, mode SortMode) {
	if len(convs) <= 1 {
		return
	}

	sort.SliceStable(convs, func(i, j int) bool {
		if convs[i].Pinned != convs[j].Pinned {
			return convs[i].Pinned
		}
		switch mode {
		case SortName:
			li, lj := strings.ToLower(convs[i].DisplayName), strings.ToLower(convs[j].DisplayName)
			if li != lj {
				return li < lj
			}
			return convs[i].SourceHash < convs[j].SourceHash
		default:
			if !convs[i].LastTime.Equal(convs[j].LastTime) {
				return convs[i].LastTime.After(convs[j].LastTime)
			}
			return convs[i].SourceHash < convs[j].SourceHash
		}
	})
}

// FilterConversations returns conversations matching the given trust level.
func FilterConversations(convs []ConversationInfo, trustLevel string) []ConversationInfo {
	var result []ConversationInfo
	for _, c := range convs {
		if c.TrustLevel == trustLevel {
			result = append(result, c)
		}
	}
	return result
}

// FilterConversationsWithBlocked returns conversations for a trust tab,
// optionally including blocked conversations on the untrusted tab.
func FilterConversationsWithBlocked(convs []ConversationInfo, trustLevel string, showBlocked bool) []ConversationInfo {
	var result []ConversationInfo
	for _, c := range convs {
		if trustLevel == "trusted" && c.TrustLevel == "trusted" {
			result = append(result, c)
		} else if trustLevel == "untrusted" {
			if c.TrustLevel == "untrusted" {
				result = append(result, c)
			} else if showBlocked && c.TrustLevel == "blocked" {
				result = append(result, c)
			}
		}
	}
	return result
}

// GetSelectedIndex returns the currently selected conversation index.
func (cd *ConversationsDisplay) GetSelectedIndex() int {
	return cd.list.GetCurrentItem()
}

// SetConversations replaces the conversation list and refreshes.
func (cd *ConversationsDisplay) SetConversations(convs []ConversationInfo) {
	cd.conversations = convs
	cd.populateList()
	cd.refreshTabBar()
}

// ToggleFullscreen toggles the conversation list pane between its normal fixed
// width and zero (detail pane fills the whole width), matching Python's
// toggle_fullscreen (Conversations.py:1276-1282): when going fullscreen the
// current list width is saved and the pane collapses to width 0; toggling
// again restores it. OnToggleFullscreen fires after the flip.
func (cd *ConversationsDisplay) ToggleFullscreen() {
	cd.fullscreen = !cd.fullscreen
	if cd.content != nil && cd.leftPanel != nil {
		if cd.fullscreen {
			cd.content.ResizeItem(cd.leftPanel, 0, 0)
		} else {
			cd.content.ResizeItem(cd.leftPanel, cd.listWidth, 0)
		}
	}
	if cd.OnToggleFullscreen != nil {
		cd.OnToggleFullscreen()
	}
}

// Fullscreen reports whether the list pane is currently hidden (fullscreen
// detail).
func (cd *ConversationsDisplay) Fullscreen() bool { return cd.fullscreen }

// SetShowTrusted switches the conversation list between the Trusted tab
// (showTrusted=true) and the Untrusted tab (showTrusted=false) and repopulates
// the list, mirroring Python's ConversationsDisplay._set_filter
// (Conversations.py:606-618). Used by the New Conversation dialog to reveal a
// freshly-created untrusted/unknown entry (Python switches to LIST_FILTER_UNTRUSTED
// when the new entry is not trusted, Conversations.py:1066-1068).
func (cd *ConversationsDisplay) SetShowTrusted(show bool) {
	cd.showTrusted = show
	cd.populateList()
	cd.refreshTabBar()
	cd.applyPileLayout()
}

// ListWidth returns the normal (non-fullscreen) fixed width of the list pane.
func (cd *ConversationsDisplay) ListWidth() int { return cd.listWidth }

// tabBarText builds the combined Trusted/Untrusted tab label text, matching
// Python's _label (Conversations.py:461-465) and the two-button tab_bar
// (Conversations.py:392-398). There are no digit prefixes (the original has
// none). When a tab has alert (unread or failed) conversations, an envelope
// glyph and the alert count follow the total, e.g. "Trusted (3) ✉ 2".
// unreadGlyph is the glyphs["unread"] string for the active glyph set. The two
// labels are joined by two spaces (the dividechars=1 Columns gap plus one
// space from each button's trailing padding).
func tabBarText(convs []ConversationInfo, unreadGlyph string) string {
	trusted, untrusted := tabButtonLabels(convs, unreadGlyph)
	return trusted + "  " + untrusted
}

// tabButtonLabels returns the per-button labels for the Trusted and Untrusted
// tab buttons, matching Python's _label (Conversations.py:458-465).
func tabButtonLabels(convs []ConversationInfo, unreadGlyph string) (trusted, untrusted string) {
	trustedCount, untrustedCount := 0, 0
	trustedAlert, untrustedAlert := 0, 0
	for _, c := range convs {
		alert := c.Unread || c.Failed
		if c.TrustLevel == "trusted" {
			trustedCount++
			if alert {
				trustedAlert++
			}
		} else {
			untrustedCount++
			if alert {
				untrustedAlert++
			}
		}
	}
	label := func(name string, total, unread int) string {
		if unread > 0 {
			return fmt.Sprintf("%s (%d) %s %d", name, total, unreadGlyph, unread)
		}
		return fmt.Sprintf("%s (%d)", name, total)
	}
	return label("Trusted", trustedCount, trustedAlert), label("Untrusted", untrustedCount, untrustedAlert)
}

// refreshTabBar recomputes the two tab-button labels from the current
// conversations and updates the tab buttons, matching Python's update_listbox
// which calls tab_trusted.set_label / tab_untrusted.set_label
// (Conversations.py:463-464).
func (cd *ConversationsDisplay) refreshTabBar() {
	if cd.tabTrusted == nil || cd.tabUntrusted == nil {
		return
	}
	glyph := "✉"
	if cd.app != nil {
		if g, ok := cd.app.Glyphs["unread"]; ok && g != "" {
			glyph = g
		}
	}
	trusted, untrusted := tabButtonLabels(cd.conversations, glyph)
	cd.tabTrusted.SetLabel(trusted)
	cd.tabUntrusted.SetLabel(untrusted)
}

// syncStatusLine returns the left-pane sync footer text, matching Python's
// _sync_status_line (Conversations.py:517-545): " Last sync: <when>" (with a
// leading space), where <when> is "never" when no sync has been recorded.
// The propagation-node label suffix is omitted until RNS wiring supplies it.
func (cd *ConversationsDisplay) syncStatusLine() string {
	return " Last sync: never"
}

// applyPileLayout rebuilds the left-pane item stack, matching Python's
// _apply_pile_layout (Conversations.py:313-330): the tab bar on top, the
// "Show blocked" checkbox only in the Untrusted filter, the list (weight 1)
// in the middle, and the sync footer at the bottom.
func (cd *ConversationsDisplay) applyPileLayout() {
	if cd.leftPanel == nil {
		return
	}
	cd.leftPanel.Clear()
	cd.leftPanel.AddItem(cd.tabBar, 1, 0, false)
	if !cd.showTrusted && cd.showBlockedCheckbox != nil {
		cd.leftPanel.AddItem(cd.showBlockedCheckbox, 1, 0, false)
	}
	cd.leftPanel.AddItem(cd.ilb, 0, 1, true)
	cd.leftPanel.AddItem(cd.syncStatus, 1, 0, false)
}

// ToggleSort toggles between sort-by-time and sort-by-name.
func (cd *ConversationsDisplay) ToggleSort() {
	cd.populateList()
}

// PeerInfoEntry holds the data for a peer info dialog.
// Matches Python's directory entry fields used in
// edit_selected_in_directory() at Conversations.py:821.
type PeerInfoEntry struct {
	SourceHash        string
	DisplayName       string
	TrustLevel        string
	PreferredDelivery string
	Pinned            bool
	Notes             string
}

// TrustLevelValue returns the trust level string normalized to one of
// TrustTrusted, TrustUntrusted, or TrustUnknown.
func (e PeerInfoEntry) TrustLevelValue() string {
	switch e.TrustLevel {
	case TrustTrusted:
		return TrustTrusted
	case TrustUntrusted, "blocked":
		return TrustUntrusted
	default:
		return TrustUnknown
	}
}

// EditSelectedInDirectory opens a Peer Info dialog for the currently
// selected conversation, returning the peer's directory entry data.
// Matches Python's edit_selected_in_directory() at Conversations.py:821.
func (cd *ConversationsDisplay) EditSelectedInDirectory() PeerInfoEntry {
	conv, ok := cd.GetSelectedConversation()
	if !ok {
		return PeerInfoEntry{}
	}
	return PeerInfoEntry{
		SourceHash:  conv.SourceHash,
		DisplayName: conv.DisplayName,
		TrustLevel:  conv.TrustLevel,
	}
}

// DialogOpen reports whether a dialog is currently open.
func (cd *ConversationsDisplay) DialogOpen() bool {
	return cd.dialogOpen
}

// OpenIngestURIDialog opens the ingest LXMF URI dialog.
// Matches Python's ingest_lxm_uri() at Conversations.py:1118.
func (cd *ConversationsDisplay) OpenIngestURIDialog() {
	cd.dialogOpen = true
	cd.ingestURIValue = ""
}

// ConfirmIngestURI confirms the URI to ingest and closes the dialog.
func (cd *ConversationsDisplay) ConfirmIngestURI(uri string) {
	cd.ingestURIValue = uri
	cd.dialogOpen = false
}

// IngestURIDialogValue returns the URI entered in the dialog.
func (cd *ConversationsDisplay) IngestURIDialogValue() string {
	return cd.ingestURIValue
}

// DismissIngestURIDialog closes the ingest URI dialog without action.
func (cd *ConversationsDisplay) DismissIngestURIDialog() {
	cd.dialogOpen = false
	cd.ingestURIValue = ""
}

// ShowBlocked reports whether blocked peers are shown in the
// untrusted tab. Matches Python's _on_show_blocked_change()
// at Conversations.py:306.
func (cd *ConversationsDisplay) ShowBlocked() bool {
	return cd.showBlocked
}

// SetShowBlocked toggles showing blocked peers and refreshes
// the conversation list. Matches Python's _on_show_blocked_change()
// at Conversations.py:306.
func (cd *ConversationsDisplay) SetShowBlocked(show bool) {
	cd.showBlocked = show
	cd.populateList()
}

// BlockedRowLabel formats the display label for a blocked peer row.
// Matches Python's _blocked_row_widget() at Conversations.py:332.
func BlockedRowLabel(displayName, sourceHash string) string {
	if displayName == "" {
		displayName = sourceHash
	}
	return fmt.Sprintf("× [blocked] %s", displayName)
}

// OpenSyncDialog opens the LXMF sync dialog with propagation
// node selector and progress bar.
// Matches Python's sync_conversations() at Conversations.py:1359.
func (cd *ConversationsDisplay) OpenSyncDialog() {
	cd.dialogOpen = true
}

// RequestSync requests an LXMF sync with an optional message limit.
// A limit of 0 means download all messages.
func (cd *ConversationsDisplay) RequestSync(limit int) {
	if cd.OnSyncRequested != nil {
		cd.OnSyncRequested(limit)
	}
}

// DismissSyncDialog closes the sync dialog.
func (cd *ConversationsDisplay) DismissSyncDialog() {
	cd.dialogOpen = false
}

// IngestURIDialog shows a dialog to ingest an LXM URI.
// Matches Python's ingest_lxm_uri() at Conversations.py:1118.
func (cd *ConversationsDisplay) IngestURIDialog(onSubmit func(uri string)) {
	cd.dialogOpen = true
	cd.app.Dialogs.ShowInputDialog("Ingest LXM URI", "URI : ", "",
		func(uri string) {
			cd.dialogOpen = false
			if onSubmit != nil {
				onSubmit(uri)
			}
		},
		func() {
			cd.dialogOpen = false
		},
	)
}

// ShowIngestResult shows the result of an LXM URI ingest operation.
// Matches Python's ingest_lxm_uri result dialogs (Conversations.py:1143-1237).
func (cd *ConversationsDisplay) ShowIngestResult(result IngestResult) {
	cd.dialogOpen = true
	msg := IngestResultText(result)

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
			cd.dialogOpen = false
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg), 3, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Ingest message URI", layout, 50, 6, func() {
		cd.dialogOpen = false
	})
}

// IngestResultText returns the verbatim dialog text Python nomadnet shows for
// each ingest outcome (Conversations.py:1143-1237). The error text preserves
// Python's "Could ingest" typo.
func IngestResultText(result IngestResult) string {
	switch result {
	case IngestSuccess:
		return "Message was decoded, decrypted successfully, and added to your conversation list."
	case IngestDuplicate:
		return "The decoded message has already been processed by the LXMF Router, and will not be ingested again."
	case IngestPropagated:
		return "The decoded message was not addressed to this LXMF address, but has been added to the propagation node queues, and will be distributed on the propagation network."
	case IngestDiscarded:
		return "The decoded message was not addressed to this LXMF address, and has been discarded."
	case IngestError:
		return "Could ingest LXM from URI data. Check your input."
	}
	return ""
}

// IngestResult represents the result of an LXM URI ingest operation.
type IngestResult int

const (
	// IngestSuccess means the message was decoded and added.
	IngestSuccess IngestResult = iota
	// IngestDuplicate means the message was already processed.
	IngestDuplicate
	// IngestError means the URI contained no decodable messages.
	IngestError
	// IngestPropagated means the message was not local but stored to the
	// propagation node queue.
	IngestPropagated
	// IngestDiscarded means the message was not local and this node is not
	// hosting a propagation node, so the message was discarded.
	IngestDiscarded
)

// PaperMessageDialog shows the paper message output options.
// Matches Python's paper_message() at Conversations.py:2505-2570.
func (cd *ConversationsDisplay) PaperMessageDialog(
	onPrintQR func(),
	onSaveQR func(),
	onSaveURI func(),
) {
	cd.dialogOpen = true

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Print QR").SetSelectedFunc(func() {
			cd.dialogOpen = false
			if onPrintQR != nil {
				onPrintQR()
			}
		}), 0, 1, false).
		AddItem(tview.NewButton("Save QR").SetSelectedFunc(func() {
			cd.dialogOpen = false
			if onSaveQR != nil {
				onSaveQR()
			}
		}), 0, 1, false).
		AddItem(tview.NewButton("Save URI").SetSelectedFunc(func() {
			cd.dialogOpen = false
			if onSaveURI != nil {
				onSaveURI()
			}
		}), 0, 1, false).
		AddItem(tview.NewButton("Cancel").SetSelectedFunc(func() {
			cd.dialogOpen = false
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText("Select the desired paper message output method."), 2, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Create Paper Message", layout, 60, 5, func() {
		cd.dialogOpen = false
	})
}

// PaperMessageFailed shows a failure message for paper message operations.
// Matches Python's paper_message_failed() at Conversations.py:2580-2600.
func (cd *ConversationsDisplay) PaperMessageFailed() {
	cd.dialogOpen = true

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
			cd.dialogOpen = false
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText("Could not output paper message,\ncheck your settings. See the log\nfile for any error messages."), 4, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("!", layout, 40, 6, func() {
		cd.dialogOpen = false
	})
}

// PaperMessageSaved shows the saved-path confirmation for a paper message,
// matching Python's paper_message_saved() at Conversations.py:2451-2472.
func (cd *ConversationsDisplay) PaperMessageSaved(path string) {
	cd.dialogOpen = true

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
			cd.dialogOpen = false
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText("The paper message was saved to:\n\n"+path+"\n"), 4, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Paper Message", layout, 60, 6, func() {
		cd.dialogOpen = false
	})
}

// AttachFileDialog shows a file browser dialog for selecting files.
// Matches Python's attach_file() at Conversations.py:2438.
func (cd *ConversationsDisplay) AttachFileDialog(directory string, onSelect func(path string)) {
	cd.dialogOpen = true
	cd.app.Dialogs.ShowInputDialog("Attach File", "Path:", directory,
		func(path string) {
			cd.dialogOpen = false
			if onSelect != nil {
				onSelect(path)
			}
		},
		func() {
			cd.dialogOpen = false
		},
	)
}

// SaveAttachmentsDialog shows a dialog with checkboxes for each attachment in
// the conversation, allowing the user to select which to copy to the download
// directory. Matches Python's save_focused_attachments() / do_save
// (Conversations.py:2324-2410): the dialog stays open after "Copy to Downloads"
// so the in-dialog status text reports the result; "Close" dismisses it.
func (cd *ConversationsDisplay) SaveAttachmentsDialog(sourceHash string, refs []AttachmentRef) {
	cd.dialogOpen = true

	list := tview.NewList()
	list.SetHighlightFullLine(true)
	ApplyListFocusStyle(list, cd.app.Theme)
	for _, r := range refs {
		list.AddItem(r.Name, "", 0, nil)
	}

	selected := make(map[int]bool)
	list.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		selected[i] = !selected[i]
		if selected[i] {
			list.SetItemText(i, "[x] "+mainText, secondaryText)
		} else {
			list.SetItemText(i, mainText, secondaryText)
		}
	})

	statusText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd))

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Copy to Downloads").SetSelectedFunc(func() {
			var chosen []AttachmentRef
			for i := range refs {
				if selected[i] {
					chosen = append(chosen, refs[i])
				}
			}
			if cd.OnSaveAttachments == nil {
				return
			}
			saved, failed := cd.OnSaveAttachments(sourceHash, chosen)
			g := cd.app.Glyphs
			var lines []string
			if len(saved) > 0 {
				lines = append(lines, fmt.Sprintf("%s Copied %d file(s) to %s:", g["check"], len(saved), saveDirOf(saved)))
				for _, p := range saved {
					lines = append(lines, "  "+filepath.Base(p))
				}
				if failed > 0 {
					lines = append(lines, fmt.Sprintf("%s %d failed", g["cross"], failed))
				}
			} else if failed > 0 {
				lines = append(lines, fmt.Sprintf("%s Failed: %d file(s)", g["cross"], failed))
			} else {
				lines = append(lines, "No files selected")
			}
			statusText.SetText(strings.Join(lines, "\n"))
		}), 0, 1, true).
		AddItem(tview.NewButton("Close").SetSelectedFunc(func() {
			cd.dialogOpen = false
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(statusText, 2, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Attachments", layout, 50, 14, func() {
		cd.dialogOpen = false
	})
}

// saveDirOf returns the common directory of the saved paths (for the status
// line), or the first path's directory when they differ.
func saveDirOf(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return filepath.Dir(paths[0])
}

// PeerInfoDialogHooks supplies the wiring-layer behavior for the Peer Info
// dialog's action buttons and the known-section query, mirroring Python's
// edit_selected_in_directory (Conversations.py:957-1000) ping/block/qr/query
// actions. Any nil hook disables its control (the button is still drawn for
// layout parity but is a no-op).
type PeerInfoDialogHooks struct {
	// IsKnown reports whether the peer's identity is known on the network
	// (Python directory.is_known). When false the dialog shows the "Query
	// network for keys" section; when true it shows only a divider.
	IsKnown func(sourceHash string) bool
	// OnQueryKeys queries the network for the peer's identity (Python
	// Conversation.query_for_peer) and dismisses the dialog.
	OnQueryKeys func(sourceHash string)
	// OnPing pings the peer and reports the outcome via setStatus, which
	// updates the dialog's centered action-status line (Python
	// _ping_peer_from_dialog).
	OnPing func(sourceHash string, setStatus func(string))
	// OnBlock blocks the peer (Python _block_peer_from_dialog).
	OnBlock func(sourceHash string)
	// OnLXMFQR shows the LXMF QR dialog for the peer (Python show_qr_dialog).
	OnLXMFQR func(sourceHash string, title string)
}

// ShowPeerInfoDialog shows the Peer Info dialog with editable fields for name,
// copy, trust level, delivery mode, pin, and notes, plus the known-section and
// Ping/Block/LXMF action row. Matches Python's edit_selected_in_directory()
// at Conversations.py:821-1020. onSave fires with the edited entry on Save.
func (cd *ConversationsDisplay) ShowPeerInfoDialog(entry PeerInfoEntry, hooks PeerInfoDialogHooks, onSave func(PeerInfoEntry)) {
	cd.dialogOpen = true
	g := cd.app.Glyphs
	divider := "-"
	if g != nil {
		divider = g["divider1"]
	}

	// Address (read-only): urwid.Text("Addr : "+hash), Python selected_id_widget.
	addrText := tview.NewTextView()
	addrText.SetDynamicColors(true)
	addrText.SetText("Addr : " + entry.SourceHash)

	// Name (editable): ReadlineEdit "Name : ".
	eName := NewReadlineEdit(cd.app.killRing, "Name : ", "")
	eName.SetText(entry.DisplayName)
	eName.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	eName.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Copy (editable): ReadlineEdit "Copy : ", pre-filled with the hash.
	eCopy := NewReadlineEdit(cd.app.killRing, "Copy : ", "")
	eCopy.SetText(entry.SourceHash)
	eCopy.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	eCopy.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Trust radio group (Untrusted/Unknown/Trusted). Defaults: Unknown selected,
	// matching Python (unknown_selected=True).
	utrust := entry.TrustLevelValue()
	untrustedSel := false
	unknownSel := true
	trustedSel := false
	switch utrust {
	case TrustUntrusted:
		untrustedSel, unknownSel, trustedSel = true, false, false
	case TrustTrusted:
		untrustedSel, unknownSel, trustedSel = false, false, true
	}
	trustGroup := &DialogRadioGroup{}
	rUntrusted := NewRadioButton(trustGroup, "Untrusted", untrustedSel, true)
	rUnknown := NewRadioButton(trustGroup, "Unknown", unknownSel, false)
	rTrusted := NewRadioButton(trustGroup, "Trusted", trustedSel, true)

	// Delivery radio group (Deliver directly/Use propagation nodes). Default:
	// direct, matching Python (direct_selected=True).
	propagatedSel := entry.PreferredDelivery == "propagated"
	methodGroup := &DialogRadioGroup{}
	rDirect := NewRadioButton(methodGroup, "Deliver directly", !propagatedSel, true)
	rPropagated := NewRadioButton(methodGroup, "Use propagation nodes", propagatedSel, false)

	// Pin checkbox ("Pin to top").
	cbPin := tview.NewCheckbox().SetLabel("Pin to top")
	cbPin.SetChecked(entry.Pinned)

	// Notes (ReadlineEdit "Notes: ").
	eNotes := NewReadlineEdit(cd.app.killRing, "Notes: ", "")
	eNotes.SetText(entry.Notes)
	eNotes.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	eNotes.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Known-section: divider if the peer identity is known, else the "Query
	// network for keys" section (Python Conversations.py:957-983).
	known := true
	if hooks.IsKnown != nil {
		known = hooks.IsKnown(entry.SourceHash)
	}
	var knownSection tview.Primitive
	if known {
		knownSection = newDividerRow(divider)
	} else {
		queryBtn := NewUrwidButton("Query network for keys")
		queryBtn.SetSelectedFunc(func() {
			if hooks.OnQueryKeys != nil {
				hooks.OnQueryKeys(entry.SourceHash)
			}
		})
		infoText := tview.NewTextView()
		infoText.SetDynamicColors(true)
		infoText.SetTextAlign(tview.AlignCenter)
		infoText.SetWrap(true)
		infoText.SetText("The identity of this peer is not known, and you cannot currently send messages to it. You can query the network to obtain the identity.")
		knownSection = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(newDividerRow(divider), 1, 0, false).
			AddItem(infoText, 3, 0, false).
			AddItem(queryBtn, 1, 0, false).
			AddItem(newDividerRow(divider), 1, 0, false)
	}

	// Action status line + Ping/Block/LXMF buttons (Python actions_row).
	actionStatus := tview.NewTextView()
	actionStatus.SetDynamicColors(true)
	actionStatus.SetTextAlign(tview.AlignCenter)
	setStatus := func(s string) { actionStatus.SetText(s) }

	pingBtn := NewUrwidButton("Ping")
	pingBtn.SetSelectedFunc(func() {
		if hooks.OnPing != nil {
			hooks.OnPing(entry.SourceHash, setStatus)
		}
	})
	blockBtn := NewUrwidButton("Block")
	blockBtn.SetSelectedFunc(func() {
		if hooks.OnBlock != nil {
			hooks.OnBlock(entry.SourceHash)
		}
	})
	qrBtn := NewUrwidButton("LXMF")
	qrBtn.SetSelectedFunc(func() {
		if hooks.OnLXMFQR != nil {
			title := entry.DisplayName
			if title == "" {
				title = entry.SourceHash
			}
			hooks.OnLXMFQR(entry.SourceHash, title)
		}
	})
	blank := func() tview.Primitive { return tview.NewTextView() }
	actionsRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(pingBtn, 0, 1, true).
		AddItem(blank(), 1, 0, false).
		AddItem(blockBtn, 0, 1, false).
		AddItem(blank(), 1, 0, false).
		AddItem(qrBtn, 0, 1, false)

	dismiss := func() { cd.app.Dialogs.DismissTop() }

	// Save builds the edited PeerInfoEntry and fires onSave, mirroring Python's
	// confirmed() (Conversations.py:901-929).
	saveBtn := NewUrwidButton("Save")
	saveBtn.SetSelectedFunc(func() {
		result := PeerInfoEntry{
			SourceHash:        entry.SourceHash,
			DisplayName:       eName.GetText(),
			PreferredDelivery: "direct",
			Pinned:            cbPin.IsChecked(),
			Notes:             eNotes.GetText(),
		}
		if rPropagated.Checked() {
			result.PreferredDelivery = "propagated"
		}
		switch {
		case rUnknown.Checked():
			result.TrustLevel = TrustUnknown
		case rTrusted.Checked():
			result.TrustLevel = TrustTrusted
		default:
			result.TrustLevel = TrustUntrusted
		}
		if onSave != nil {
			onSave(result)
		}
	})

	backBtn := NewUrwidButton("Back")
	backBtn.SetSelectedFunc(dismiss)

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(saveBtn, 0, 1, true).
		AddItem(blank(), 1, 0, false).
		AddItem(backBtn, 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(addrText, 1, 0, false).
		AddItem(eName, 1, 0, true).
		AddItem(eCopy, 1, 0, false).
		AddItem(newDividerRow(divider), 1, 0, false).
		AddItem(rUntrusted, 1, 0, false).
		AddItem(rUnknown, 1, 0, false).
		AddItem(rTrusted, 1, 0, false).
		AddItem(newDividerRow(divider), 1, 0, false).
		AddItem(rDirect, 1, 0, false).
		AddItem(rPropagated, 1, 0, false).
		AddItem(newDividerRow(divider), 1, 0, false).
		AddItem(cbPin, 1, 0, false).
		AddItem(eNotes, 1, 0, false).
		AddItem(knownSection, 0, 1, false).
		AddItem(actionsRow, 1, 0, false).
		AddItem(actionStatus, 1, 0, false).
		AddItem(newDividerRow(divider), 1, 0, false).
		AddItem(buttons, 1, 0, false)

	items := []tview.Primitive{eName, eCopy, rUntrusted, rUnknown, rTrusted, rDirect, rPropagated, cbPin, eNotes, pingBtn, blockBtn, qrBtn, saveBtn, backBtn}
	cd.app.Dialogs.ShowDialog("Peer Info", layout, 50, 24, func() {
		cd.dialogOpen = false
	})
	wireDialogNav(cd.app, dismiss, items)
}

// ShowNewConversationDialog opens the "New Conversation" dialog
// (Conversations.py:1024-1120): Addr and Name ReadlineEdit fields, a
// Untrusted/Unknown/Trusted radio group, and flat Create/Back buttons. Create
// calls onCreate(addrHex, name, trust); on success the dialog dismisses, on
// failure it re-shows with the centered "Could not start conversation. Check
// your input." error text (the nomadnet "error_text" style, dark red). Back or
// Esc dismisses without action.
//
// Per urwid's RadioButton construction quirk the dialog opens with BOTH
// "Untrusted" and "Unknown" showing "(X)" (the first radio defaults to checked
// via "first True", and an explicitly-checked radio does not uncheck its
// siblings during construction). Python's confirmed() checks r_unknown first,
// so a fresh dialog with no toggling creates the entry with the Unknown trust
// level — reproduced verbatim (bug-for-bug).
//
// Layout parity: the bordered dialog is 48 columns wide and 10 rows tall (8
// content rows), matching the original whose overlay sits in the 52-column
// left pane with left=right=2 padding (Conversations.py:1108-1113). The Go
// DialogManager centers the overlay on the WHOLE screen rather than the left
// pane, a known position parity gap; the width/height/content match exactly.
func (cd *ConversationsDisplay) ShowNewConversationDialog(onCreate func(addrHex, name, trust string) bool) {
	cd.showNewConversationDialog("", "", false, onCreate)
}

// showNewConversationDialog is the recursive builder for the New Conversation
// dialog. addr/name pre-fill the fields (used when re-showing after a failed
// Create so the typed input is preserved); showError appends the error row.
func (cd *ConversationsDisplay) showNewConversationDialog(addr, name string, showError bool, onCreate func(addrHex, name, trust string) bool) {
	cd.dialogOpen = true

	eID := NewReadlineEdit(cd.app.killRing, "Addr : ", "")
	eID.SetText(addr)
	eName := NewReadlineEdit(cd.app.killRing, "Name : ", "")
	eName.SetText(name)

	group := &DialogRadioGroup{}
	rUntrusted := NewRadioButton(group, "Untrusted", false, true)
	rUnknown := NewRadioButton(group, "Unknown", true, false)
	rTrusted := NewRadioButton(group, "Trusted", false, true)

	createBtn := NewUrwidButton("Create")
	backBtn := NewUrwidButton("Back")

	// Blank 1-row spacer, matching the urwid.Text("") separators in the pile.
	blank := func() tview.Primitive { return tview.NewTextView() }

	// Button row: Create 21 / gap 5 / Back 20 = 46 (the inner content width),
	// matching the original Columns weights 0.45/0.1/0.45 at width 46.
	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(createBtn, 21, 0, true).
		AddItem(blank(), 5, 0, false).
		AddItem(backBtn, 20, 0, false)

	pile := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(eID, 1, 0, true).
		AddItem(eName, 1, 0, false).
		AddItem(blank(), 1, 0, false).
		AddItem(rUntrusted, 1, 0, false).
		AddItem(rUnknown, 1, 0, false).
		AddItem(rTrusted, 1, 0, false).
		AddItem(blank(), 1, 0, false).
		AddItem(buttons, 1, 0, false)

	// Inner content width 46 → bordered dialog 48 wide (DialogLineBox draws
	// its border one cell outside the box). 8 content rows → 10 rows tall.
	width, height := 46, 8
	if showError {
		errText := tview.NewTextView().SetDynamicColors(true)
		errText.SetTextAlign(tview.AlignCenter)
		errText.SetWrap(true)
		errText.SetWordWrap(true)
		errText.SetText("[red]Could not start conversation. Check your input.[-]")
		// The error text is 47 chars and wraps to 2 lines at width 46
		// (urwid renders it PACK, 2 centered lines); reserve 2 rows so both
		// wrapped lines are visible, plus the leading blank separator.
		pile.AddItem(blank(), 1, 0, false).
			AddItem(errText, 2, 0, false)
		height = 11
	}

	dismiss := func() { cd.app.Dialogs.DismissTop() }

	createBtn.SetSelectedFunc(func() {
		addrHex := strings.TrimSpace(eID.GetText())
		displayName := eName.GetText()
		trust := "untrusted"
		if rUnknown.Checked() {
			trust = "unknown"
		} else if rTrusted.Checked() {
			trust = "trusted"
		}
		if onCreate(addrHex, displayName, trust) {
			dismiss()
			return
		}
		// Re-show with the preserved inputs and the error row. The dialog is
		// fixed-height, so the error row cannot be added in place; rebuilding
		// preserves the typed Addr/Name. Python appends the error without
		// rebuilding (so focus stays on Create there) — a minor parity gap not
		// visible in static captures (the tmux capture cannot see the cursor).
		cd.app.Dialogs.DismissTop()
		cd.showNewConversationDialog(addrHex, displayName, true, onCreate)
	})
	backBtn.SetSelectedFunc(dismiss)

	items := []tview.Primitive{eID, eName, rUntrusted, rUnknown, rTrusted, createBtn, backBtn}
	cd.app.Dialogs.ShowDialog("New Conversation", pile, width, height, func() {
		cd.dialogOpen = false
	})
	// Wire urwid-Pile-style Tab/Up/Down/Esc traversal and focus the Addr field
	// (the dialog's first focusable widget, matching the original).
	wireDialogNav(cd.app, dismiss, items)
}

// SyncMode represents the conversation sync download mode.
type SyncMode int

const (
	// SyncAll downloads all available messages.
	SyncAll SyncMode = iota
	// SyncLimited downloads up to a specified limit.
	SyncLimited
)

// SyncDialogResult holds the result of the sync dialog.
type SyncDialogResult struct {
	Mode   SyncMode
	Limit  int
	Action string // "sync", "cancel", or "dismiss"
}

// SyncDialogHooks supplies the live sync state the dialog polls while open,
// mirroring Python's update_sync_dialog (Conversations.py:1566-1575) reading
// app.get_sync_progress / get_sync_status. Progress is a 0..1 fraction; Status
// is the human-readable transfer state; ShowPercent reports whether the
// percent should be appended to the status line (true only while actively
// receiving, Python sync_status_show_percent).
type SyncDialogHooks struct {
	Progress    func() float64
	Status      func() string
	ShowPercent func() bool
}

// ShowSyncDialog shows the sync configuration dialog with propagation node
// selection, download limit, and a live-refreshing status/progress line.
// Matches Python's sync_conversations() (Conversations.py:1359-1500) plus
// update_sync_dialog (Conversations.py:1566-1575): the status line + the
// Sync Now/Cancel Sync button toggle from the live hooks each refresh tick
// (200ms) while the dialog is open.
func (cd *ConversationsDisplay) ShowSyncDialog(
	currentPN string,
	pnOptions []string,
	hooks SyncDialogHooks,
	onSync func(result SyncDialogResult),
) {
	cd.stopSyncRefresh()
	cd.dialogOpen = true
	cd.syncHooks = hooks
	mode := SyncAll

	// Mode selection via list
	modeList := tview.NewList()
	modeList.SetHighlightFullLine(true)
	ApplyListFocusStyle(modeList, cd.app.Theme)
	modeList.AddItem("Download all", "", 0, nil)
	modeList.AddItem("Limit to:", "", 0, nil)
	modeList.SetSelectedFunc(func(i int, _ string, _ string, _ rune) {
		switch i {
		case 0:
			mode = SyncAll
		case 1:
			mode = SyncLimited
		}
	})

	// Limit input
	limitInput := tview.NewInputField()
	limitInput.SetLabel("Messages: ")
	limitInput.SetText("5")
	limitInput.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	limitInput.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Live status/progress line (refreshed by updateSyncProgress).
	cd.syncStatusText = tview.NewTextView()
	cd.syncStatusText.SetDynamicColors(true)
	cd.syncStatusText.SetTextColor(tcell.NewHexColor(0xdddddd))
	cd.syncProgressBox = tview.NewTextView()
	cd.syncProgressBox.SetDynamicColors(true)
	cd.syncProgressBox.SetTextColor(tcell.NewHexColor(0xdddddd))

	// Propagation node display
	pnText := tview.NewTextView()
	pnText.SetDynamicColors(true)
	pnText.SetTextColor(tcell.NewHexColor(0xdddddd))
	if currentPN != "" {
		pnText.SetText("Node: " + currentPN)
	} else {
		pnText.SetText("[gray]No propagation node selected[-]")
	}

	// The Sync Now / Cancel Sync button label toggles with transfer state
	// (Python swaps real_sync_button / hidden_sync_button, Conversations.py:1393-1396).
	cd.syncSyncBtn = tview.NewButton("Sync Now")
	cd.syncSyncBtn.SetSelectedFunc(func() {
		if cd.isSyncActive() {
			// Currently a transfer is in progress → this is "Cancel Sync".
			if onSync != nil {
				onSync(SyncDialogResult{Action: "cancel"})
			}
			return
		}
		result := SyncDialogResult{Mode: mode, Action: "sync"}
		if mode == SyncLimited {
			_, _ = fmt.Sscanf(limitInput.GetText(), "%d", &result.Limit)
		}
		if onSync != nil {
			onSync(result)
		}
	})

	closeBtn := tview.NewButton("Close")
	closeBtn.SetSelectedFunc(func() {
		cd.dialogOpen = false
		cd.stopSyncRefresh()
		if onSync != nil {
			onSync(SyncDialogResult{Action: "dismiss"})
		}
	})

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(cd.syncSyncBtn, 0, 1, true).
		AddItem(tview.NewTextView().SetText("  "), 1, 0, false).
		AddItem(closeBtn, 0, 1, false)

	// Layout
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pnText, 1, 0, false).
		AddItem(cd.syncStatusText, 1, 0, false).
		AddItem(cd.syncProgressBox, 1, 0, false).
		AddItem(tview.NewTextView().SetText("Download mode:"), 1, 0, false).
		AddItem(modeList, 2, 0, false).
		AddItem(limitInput, 1, 0, false).
		AddItem(tview.NewTextView().SetText(""), 1, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.updateSyncProgress()
	cd.app.Dialogs.ShowDialog("Sync", layout, 50, 11, func() {
		cd.dialogOpen = false
		cd.stopSyncRefresh()
		if onSync != nil {
			onSync(SyncDialogResult{Action: "dismiss"})
		}
	})
}

// isSyncActive reports whether a transfer is currently in progress, mirroring
// Python's check that the status is not Idle and not yet PR_COMPLETE
// (Conversations.py:1393,1570).
func (cd *ConversationsDisplay) isSyncActive() bool {
	if cd.syncHooks.Status == nil {
		return false
	}
	status := cd.syncHooks.Status()
	return status != "Idle" && !strings.HasPrefix(status, "Done") && !strings.HasPrefix(status, "Downloaded")
}

// updateSyncProgress refreshes the status line, progress bar, and Sync Now /
// Cancel Sync button label from the live hooks, mirroring Python's
// update_sync_dialog (Conversations.py:1566-1575). Synchronous (no event-loop
// marshalling) so it is unit-testable; the refresh goroutine wraps it in
// QueueUpdateDraw in production.
func (cd *ConversationsDisplay) updateSyncProgress() {
	if cd.syncStatusText == nil {
		return
	}
	var status string
	if cd.syncHooks.Status != nil {
		status = cd.syncHooks.Status()
	}
	var prog float64
	if cd.syncHooks.Progress != nil {
		prog = cd.syncHooks.Progress()
	}
	showPercent := false
	if cd.syncHooks.ShowPercent != nil {
		showPercent = cd.syncHooks.ShowPercent()
	}

	if showPercent {
		cd.syncStatusText.SetText(fmt.Sprintf("%s (%.0f%%)", status, prog*100))
	} else {
		cd.syncStatusText.SetText(status)
	}
	// The progress bar always shows the numeric progress so the bar is visible
	// even when the status line suppresses the percent (e.g. while connecting).
	cd.syncProgressBox.SetText(fmt.Sprintf("[%.0f%%]", prog*100))

	if cd.syncSyncBtn != nil {
		if cd.isSyncActive() {
			cd.syncSyncBtn.SetLabel("Cancel Sync")
		} else {
			cd.syncSyncBtn.SetLabel("Sync Now")
		}
	}
}

// SetSyncShowPercentHook replaces the ShowPercent hook (test helper to flip
// the percent display without rebuilding the dialog).
func (cd *ConversationsDisplay) SetSyncShowPercentHook(f func() bool) {
	cd.syncMutex.Lock()
	cd.syncHooks.ShowPercent = f
	cd.syncMutex.Unlock()
}

// startSyncRefresh launches the 200ms progress-refresh goroutine, mirroring
// Python's set_alarm_in(0.2, update_sync_dialog) loop
// (Conversations.py:1575). marshal=true queues updates onto the running
// application event loop (production); false runs synchronously (tests, where
// QueueUpdateDraw would block). Idempotent.
func (cd *ConversationsDisplay) StartSyncRefresh(marshal bool) {
	cd.syncMutex.Lock()
	if cd.syncStop != nil {
		cd.syncMutex.Unlock()
		return
	}
	cd.syncStop = make(chan struct{})
	stop := cd.syncStop
	cd.syncMutex.Unlock()

	cd.syncWG.Go(func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if marshal && cd.app != nil {
					cd.app.QueueUpdateDraw(cd.updateSyncProgress)
				} else {
					cd.updateSyncProgress()
				}
			}
		}
	})
}

// stopSyncRefresh stops the progress-refresh goroutine. Idempotent.
func (cd *ConversationsDisplay) stopSyncRefresh() {
	cd.syncMutex.Lock()
	stop := cd.syncStop
	cd.syncStop = nil
	cd.syncMutex.Unlock()
	if stop != nil {
		close(stop)
		cd.syncWG.Wait()
	}
}
