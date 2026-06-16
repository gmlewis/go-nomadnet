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
	MessageCount int
	Failed       bool
	Pinned       bool
	SortRank     *int
}

// ConversationsDisplay shows the conversation list and message view.
// Matches Python's ConversationsDisplay with Trusted/Untrusted tabs.
type ConversationsDisplay struct {
	app           *tview.Application
	widget        *tview.Flex
	list          *tview.List
	detail        *tview.TextView
	conversations []ConversationInfo
	selected      int
	showTrusted   bool // true = trusted tab, false = untrusted

	// Keyboard shortcut callbacks (Python: ConversationsArea.keypress)
	OnEditPeerInfo     func()
	OnDeleteConv       func()
	OnNewConv          func()
	OnIngestURI        func()
	OnSync             func()
	OnToggleFullscreen func()
	OnToggleSort       func()
	OnShowQR           func()
}

// NewConversationsDisplay creates a new conversations display.
func NewConversationsDisplay(app *tview.Application, convs []ConversationInfo) *ConversationsDisplay {
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

	// Tab bar: Trusted (N) | Untrusted (N)
	trustedCount := 0
	untrustedCount := 0
	for _, c := range convs {
		if c.TrustLevel == "trusted" {
			trustedCount++
		} else {
			untrustedCount++
		}
	}
	tabText := fmt.Sprintf("[yellow]1[-] Trusted (%d)  [yellow]2[-] Untrusted (%d)", trustedCount, untrustedCount)
	tabBar := tview.NewTextView()
	tabBar.SetTextAlign(tview.AlignCenter)
	tabBar.SetDynamicColors(true)
	tabBar.SetTextColor(tcell.NewHexColor(0xdddddd))
	tabBar.SetText(tabText)

	// Conversation list
	cd.list = tview.NewList()
	cd.list.SetHighlightFullLine(true)
	cd.list.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

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
	leftPanel.AddItem(tabBar, 1, 0, false)
	leftPanel.AddItem(cd.list, 0, 1, true)

	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(leftPanel, 0, 1, true)
	content.AddItem(cd.detail, 0, 2, false)

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
		if cd.OnToggleFullscreen != nil {
			cd.OnToggleFullscreen()
		}
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

	for _, conv := range cd.conversations {
		if cd.showTrusted && conv.TrustLevel != "trusted" {
			continue
		}
		if !cd.showTrusted && conv.TrustLevel == "trusted" {
			continue
		}

		prefix := "  "
		if conv.Unread {
			prefix = "[!] "
		}
		if conv.Failed {
			prefix = "[x] "
		}
		trustIcon := "○"
		if conv.TrustLevel == "trusted" {
			trustIcon = "●"
		} else if conv.TrustLevel == "untrusted" {
			trustIcon = "×"
		}
		text := fmt.Sprintf("%s%s %s", prefix, trustIcon, conv.DisplayName)
		secondary := fmt.Sprintf("%s — %s", relativeTime(conv.LastTime), conv.LastMessage)
		cd.list.AddItem(text, secondary, 0, nil)
	}

	if cd.list.GetItemCount() == 0 {
		emptyMsg := "No trusted conversations"
		if !cd.showTrusted {
			emptyMsg = "No untrusted conversations"
		}
		cd.list.AddItem("[gray]"+emptyMsg+"[-]", "", 0, nil)
	}
}

// switchTab switches between trusted and untrusted tabs.
func (cd *ConversationsDisplay) switchTab() {
	cd.showTrusted = !cd.showTrusted
	cd.populateList()
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

// sortByName sorts conversations by display name.
func sortByName(convs []ConversationInfo) {
	sort.Slice(convs, func(i, j int) bool {
		return convs[i].DisplayName < convs[j].DisplayName
	})
}

// sortByTime sorts conversations by last time (most recent first).
func sortByTime(convs []ConversationInfo) {
	sort.Slice(convs, func(i, j int) bool {
		return convs[i].LastTime.After(convs[j].LastTime)
	})
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
}

// ToggleSort toggles between sort-by-time and sort-by-name.
func (cd *ConversationsDisplay) ToggleSort() {
	cd.populateList()
}
