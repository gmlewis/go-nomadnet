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
}

// ConversationsDisplay shows the conversation list and message view.
type ConversationsDisplay struct {
	app           *tview.Application
	widget        tview.Primitive
	list          *tview.List
	detail        *tview.TextView
	conversations []ConversationInfo
	selected      int
	onSelect      func(idx int)
}

// NewConversationsDisplay creates a new conversations display.
func NewConversationsDisplay(app *tview.Application, convs []ConversationInfo) *ConversationsDisplay {
	cd := &ConversationsDisplay{
		app:           app,
		conversations: convs,
		selected:      -1,
	}

	// Title
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Conversations[-]")

	// Conversation list
	cd.list = tview.NewList().
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	for i, conv := range convs {
		prefix := "  "
		if conv.Unread {
			prefix = "[!] "
		}
		trustIcon := "○"
		if conv.TrustLevel == "trusted" {
			trustIcon = "●"
		}
		text := fmt.Sprintf("%s%s %s", prefix, trustIcon, conv.DisplayName)
		secondary := fmt.Sprintf("%s — %s", relativeTime(conv.LastTime), conv.LastMessage)
		idx := i
		cd.list.AddItem(text, secondary, 0, func() {
			cd.selected = idx
			cd.showDetail(idx)
		})
	}

	// Detail view (right side)
	cd.detail = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetTextAlign(tview.AlignLeft)

	// Layout: list on left, detail on right
	content := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(cd.list, 0, 1, true).
		AddItem(cd.detail, 0, 2, false)

	cd.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(content, 0, 1, true)

	return cd
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
