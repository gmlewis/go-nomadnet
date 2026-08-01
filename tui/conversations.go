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
	"sort"
	"strconv"
	"strings"
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
// Matches Python's ConversationsDisplay with Trusted/Untrusted tabs.
type ConversationsDisplay struct {
	app                 *App
	widget              *tview.Flex
	content             *tview.Flex
	leftPanel           *tview.Flex
	list                *tview.List
	detail              *tview.TextView
	tabBar              *tview.TextView
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
}

// NewConversationsDisplay creates a new conversations display.
func NewConversationsDisplay(app *App, convs []ConversationInfo) *ConversationsDisplay {
	cd := &ConversationsDisplay{
		app:           app,
		conversations: convs,
		selected:      -1,
		showTrusted:   true,
	}

	// Title
	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetDynamicColors(true)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("[::b]Conversations[-]")

	// Tab bar: Trusted (N) | Untrusted (N) — no digit prefixes (the original
	// has none); unread counts get an envelope glyph (Python _label,
	// Conversations.py:461-465).
	tabBar := tview.NewTextView()
	tabBar.SetTextAlign(tview.AlignCenter)
	tabBar.SetDynamicColors(true)
	tabBar.SetTextColor(tcell.NewHexColor(0xdddddd))
	cd.tabBar = tabBar
	cd.refreshTabBar()

	// Conversation list
	cd.list = tview.NewList()
	cd.list.SetHighlightFullLine(true)
	ApplyListFocusStyle(cd.list, cd.app.Theme)

	cd.populateList()

	// Detail view (right side)
	cd.detail = tview.NewTextView()
	cd.detail.SetDynamicColors(true)
	cd.detail.SetScrollable(true)
	cd.detail.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	cd.detail.SetTextAlign(tview.AlignLeft)
	cd.detail.SetText("[gray]Select a conversation to view[-]")

	// Layout: tab bar + list on left, detail on right
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow)
	leftPanel.AddItem(cd.tabBar, 1, 0, false)
	leftPanel.AddItem(cd.list, 0, 1, true)
	cd.leftPanel = leftPanel
	cd.listWidth = 52

	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(leftPanel, cd.listWidth, 0, true)
	content.AddItem(cd.detail, 0, 1, false)
	cd.content = content

	cd.widget = tview.NewFlex().SetDirection(tview.FlexRow)
	cd.widget.SetBorder(true)
	cd.widget.AddItem(title, 2, 0, false)
	cd.widget.AddItem(content, 0, 1, true)

	// Set up list callback
	cd.list.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		cd.showDetail(i)
	})

	// Set up keyboard shortcuts matching Python's ConversationsArea.keypress()
	cd.widget.SetInputCapture(cd.handleInput)

	return cd
}

// SetShortcutFocus sets which of the three Conversations shortcut bars
// GetShortcutText returns, matching the focus-path dispatch in Python
// Conversations.py:1765-1779: "list" when the list pane has focus, "editor"
// when the message editor (frame footer) has focus, "body" otherwise.
func (cd *ConversationsDisplay) SetShortcutFocus(region string) {
	cd.shortcutFocus = region
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
	cw.updatePeerInfo()
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
		// Restore the detail panel
		cd.widget.RemoveItem(cd.widget.GetItem(1))
		cd.widget.AddItem(cd.detail, 0, 2, false)
	}
	cd.widget.RemoveItem(cd.detail)
	cd.widget.AddItem(cw.Widget(), 0, 1, true)
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
		cd.list.AddItem("[gray]"+emptyMsg+"[-]", "", 0, nil)
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

// ListWidth returns the normal (non-fullscreen) fixed width of the list pane.
func (cd *ConversationsDisplay) ListWidth() int { return cd.listWidth }

// tabBarText builds the Trusted/Untrusted tab label text, matching Python's
// _label (Conversations.py:461-465) and the two-button tab_bar
// (Conversations.py:392-398). There are no digit prefixes (the original has
// none). When a tab has alert (unread or failed) conversations, an envelope
// glyph and the alert count follow the total, e.g. "Trusted (3) ✉ 2".
// unreadGlyph is the glyphs["unread"] string for the active glyph set.
func tabBarText(convs []ConversationInfo, unreadGlyph string) string {
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
	return label("Trusted", trustedCount, trustedAlert) + "  " + label("Untrusted", untrustedCount, untrustedAlert)
}

// refreshTabBar recomputes the tab label text from the current conversations
// and writes it to the tab bar widget.
func (cd *ConversationsDisplay) refreshTabBar() {
	if cd.tabBar == nil {
		return
	}
	glyph := "✉"
	if cd.app != nil {
		if g, ok := cd.app.Glyphs["unread"]; ok && g != "" {
			glyph = g
		}
	}
	cd.tabBar.SetText(tabBarText(cd.conversations, glyph))
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
func (cd *ConversationsDisplay) ShowIngestResult(result IngestResult) {
	cd.dialogOpen = true
	var msg string
	switch result {
	case IngestSuccess:
		msg = "Message was decoded, decrypted successfully, and added to your conversation list."
	case IngestDuplicate:
		msg = "The decoded message has already been processed by the LXMF Router, and will not be ingested again."
	case IngestError:
		msg = "The URI contained no decodable messages"
	}

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

// IngestResult represents the result of an LXM URI ingest operation.
type IngestResult int

const (
	// IngestSuccess means the message was decoded and added.
	IngestSuccess IngestResult = iota
	// IngestDuplicate means the message was already processed.
	IngestDuplicate
	// IngestError means the URI contained no decodable messages.
	IngestError
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

// SaveAttachmentsDialog shows a dialog for saving attachments.
// Matches Python's save_focused_attachments() at Conversations.py:2324.
func (cd *ConversationsDisplay) SaveAttachmentsDialog(attachments []string, onSave func(selected []string)) {
	cd.dialogOpen = true
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	ApplyListFocusStyle(list, cd.app.Theme)

	for _, att := range attachments {
		list.AddItem(att, "", 0, nil)
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

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Copy to Downloads").SetSelectedFunc(func() {
			cd.dialogOpen = false
			var chosen []string
			for i, att := range attachments {
				if selected[i] {
					chosen = append(chosen, att)
				}
			}
			if onSave != nil {
				onSave(chosen)
			}
		}), 0, 1, true).
		AddItem(tview.NewButton("Close").SetSelectedFunc(func() {
			cd.dialogOpen = false
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Attachments", layout, 50, 12, func() {
		cd.dialogOpen = false
	})
}

// ShowPeerInfoDialog shows the Peer Info dialog with editable fields
// for name, trust level, delivery mode, pin, and notes.
// Matches Python's edit_selected_in_directory() at Conversations.py:821-1020.
func (cd *ConversationsDisplay) ShowPeerInfoDialog(entry PeerInfoEntry, onSave func(PeerInfoEntry)) {
	cd.dialogOpen = true

	// Name field
	nameInput := tview.NewInputField()
	nameInput.SetLabel("Name : ")
	nameInput.SetText(entry.DisplayName)
	nameInput.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	nameInput.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Address (read-only)
	addrText := tview.NewTextView()
	addrText.SetDynamicColors(true)
	addrText.SetTextColor(tcell.NewHexColor(0xdddddd))
	addrText.SetText("Addr : " + entry.SourceHash)

	// Trust level selection via list
	trustList := tview.NewList()
	trustList.SetHighlightFullLine(true)
	ApplyListFocusStyle(trustList, cd.app.Theme)
	trustLevels := []string{TrustUntrusted, TrustUnknown, TrustTrusted}
	trustList.AddItem(TrustUntrusted, "", 0, nil)
	trustList.AddItem(TrustUnknown, "", 0, nil)
	trustList.AddItem(TrustTrusted, "", 0, nil)

	// Select current trust level
	selectedTrust := entry.TrustLevelValue()
	for i, tl := range trustLevels {
		if tl == selectedTrust {
			trustList.SetCurrentItem(i)
			break
		}
	}

	// Delivery mode via list
	deliveryList := tview.NewList()
	deliveryList.SetHighlightFullLine(true)
	ApplyListFocusStyle(deliveryList, cd.app.Theme)
	deliveryList.AddItem("Deliver directly", "", 0, nil)
	deliveryList.AddItem("Use propagation nodes", "", 0, nil)
	if entry.PreferredDelivery == "propagated" {
		deliveryList.SetCurrentItem(1)
	}

	// Notes field
	notesInput := tview.NewInputField()
	notesInput.SetLabel("Notes: ")
	notesInput.SetText(entry.Notes)
	notesInput.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	notesInput.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Save/Back buttons
	saveBtn := tview.NewButton("Save")
	saveBtn.SetSelectedFunc(func() {
		cd.dialogOpen = false
		result := PeerInfoEntry{
			SourceHash:  entry.SourceHash,
			DisplayName: nameInput.GetText(),
			TrustLevel:  trustLevels[trustList.GetCurrentItem()],
			Pinned:      entry.Pinned,
			Notes:       notesInput.GetText(),
		}
		if deliveryList.GetCurrentItem() == 1 {
			result.PreferredDelivery = "propagated"
		} else {
			result.PreferredDelivery = "direct"
		}
		if onSave != nil {
			onSave(result)
		}
	})

	backBtn := tview.NewButton("Back")
	backBtn.SetSelectedFunc(func() {
		cd.dialogOpen = false
	})

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(saveBtn, 0, 1, true).
		AddItem(backBtn, 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(addrText, 1, 0, false).
		AddItem(nameInput, 1, 0, true).
		AddItem(tview.NewTextView().SetText("Trust Level:"), 1, 0, false).
		AddItem(trustList, 3, 0, false).
		AddItem(tview.NewTextView().SetText("Delivery:"), 1, 0, false).
		AddItem(deliveryList, 2, 0, false).
		AddItem(notesInput, 1, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Peer Info", layout, 50, 14, func() {
		cd.dialogOpen = false
	})
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

// ShowSyncDialog shows the sync configuration dialog with propagation
// node selection and download limit options.
// Matches Python's sync_conversations() at Conversations.py:1359-1500.
func (cd *ConversationsDisplay) ShowSyncDialog(
	currentPN string,
	pnOptions []string,
	progress float64,
	onSync func(result SyncDialogResult),
) {
	cd.dialogOpen = true
	mode := SyncAll

	// Mode selection via list
	modeList := tview.NewList()
	modeList.SetHighlightFullLine(true)
	ApplyListFocusStyle(modeList, cd.app.Theme)
	modeList.AddItem("Download all", "", 0, nil)
	modeList.AddItem("Limit to:", "", 0, nil)

	// Limit input
	limitInput := tview.NewInputField()
	limitInput.SetLabel("Messages: ")
	limitInput.SetText("5")
	limitInput.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	limitInput.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Progress bar (simplified as text)
	progressBar := tview.NewTextView()
	progressBar.SetDynamicColors(true)
	progressBar.SetTextColor(tcell.NewHexColor(0xdddddd))
	progressBar.SetText(fmt.Sprintf("Progress: %.0f%%", progress*100))

	// Propagation node display
	pnText := tview.NewTextView()
	pnText.SetDynamicColors(true)
	pnText.SetTextColor(tcell.NewHexColor(0xdddddd))
	if currentPN != "" {
		pnText.SetText("Node: " + currentPN)
	} else {
		pnText.SetText("[gray]No propagation node selected[-]")
	}

	// Sync Now / Close buttons
	syncBtn := tview.NewButton("Sync Now")
	syncBtn.SetSelectedFunc(func() {
		cd.dialogOpen = false
		if onSync != nil {
			result := SyncDialogResult{Mode: mode, Action: "sync"}
			if mode == SyncLimited {
				_, _ = fmt.Sscanf(limitInput.GetText(), "%d", &result.Limit)
			}
			onSync(result)
		}
	})

	cancelBtn := tview.NewButton("Cancel Sync")
	cancelBtn.SetSelectedFunc(func() {
		cd.dialogOpen = false
		if onSync != nil {
			onSync(SyncDialogResult{Action: "cancel"})
		}
	})

	closeBtn := tview.NewButton("Close")
	closeBtn.SetSelectedFunc(func() {
		cd.dialogOpen = false
		if onSync != nil {
			onSync(SyncDialogResult{Action: "dismiss"})
		}
	})

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(syncBtn, 0, 1, true).
		AddItem(cancelBtn, 0, 1, false).
		AddItem(closeBtn, 0, 1, false)

	// Layout
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pnText, 1, 0, false).
		AddItem(progressBar, 1, 0, false).
		AddItem(tview.NewTextView().SetText("Download mode:"), 1, 0, false).
		AddItem(modeList, 2, 0, false).
		AddItem(limitInput, 1, 0, false).
		AddItem(tview.NewTextView().SetText(""), 1, 0, false).
		AddItem(buttons, 1, 0, false)

	cd.app.Dialogs.ShowDialog("Sync", layout, 50, 10, func() {
		cd.dialogOpen = false
		if onSync != nil {
			onSync(SyncDialogResult{Action: "dismiss"})
		}
	})
}
