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
	cd.widget.AddItem(title, 2, 0, false)
	cd.widget.AddItem(content, 0, 1, true)

	// Set up list callback
	cd.list.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		cd.showDetail(i)
	})

	return cd
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
func relativeTime(t time.Time) string {
	delta := time.Since(t)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		m := int(delta.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case delta < 24*time.Hour:
		h := int(delta.Hours())
		return fmt.Sprintf("%dh ago", h)
	case delta < 48*time.Hour:
		return "yesterday"
	case delta < 7*24*time.Hour:
		d := int(delta.Hours() / 24)
		return fmt.Sprintf("%dd ago", d)
	default:
		return t.Format("2006-01-02")
	}
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
