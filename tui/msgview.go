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
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// MessageInfo holds a single message for display.
type MessageInfo struct {
	Title       string
	Content     string
	Sender      string
	Timestamp   string
	TrustLevel  string
	HasAttached bool
}

// MessageViewDisplay renders messages with Micron markup.
type MessageViewDisplay struct {
	app    *App
	widget tview.Primitive
	view   *tview.TextView
}

// NewMessageViewDisplay creates a new message view display.
func NewMessageViewDisplay(app *App) *MessageViewDisplay {
	mvd := &MessageViewDisplay{app: app}

	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Message[-]")

	mvd.view = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetTextAlign(tview.AlignLeft)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(mvd.view, 0, 1, true)

	mvd.widget = layout
	return mvd
}

// Widget returns the tview primitive for this display.
func (mvd *MessageViewDisplay) Widget() tview.Primitive {
	return mvd.widget
}

// ShowMessage displays a message with optional Micron rendering.
func (mvd *MessageViewDisplay) ShowMessage(msg MessageInfo) {
	var sb strings.Builder

	// Trust level banner
	trustColor := "[gray]"
	switch msg.TrustLevel {
	case "trusted":
		trustColor = "[green]"
	case "untrusted":
		trustColor = "[red]"
	case "unknown":
		trustColor = "[yellow]"
	}

	sb.WriteString(fmt.Sprintf("%s[%s][-] ", trustColor, strings.ToUpper(msg.TrustLevel)))
	sb.WriteString(fmt.Sprintf("[::b]%s[-]", msg.Sender))
	sb.WriteString(fmt.Sprintf("  [gray]%s[-]\n", msg.Timestamp))
	sb.WriteString(strings.Repeat("─", 40) + "\n\n")

	// Render content with Micron if it looks like Micron markup
	content := msg.Content
	if looksLikeMicron(content) {
		content = renderMicronAsText(content)
	}

	sb.WriteString(content)

	if msg.HasAttached {
		sb.WriteString("\n\n[gray][has attachments][-]")
	}

	mvd.view.SetText(sb.String())
}

// Clear clears the message view.
func (mvd *MessageViewDisplay) Clear() {
	mvd.view.SetText("")
}

// looksLikeMicron checks if text contains Micron markup.
func looksLikeMicron(text string) bool {
	return strings.Contains(text, ">>") ||
		strings.Contains(text, "```") ||
		strings.Contains(text, "`!")
}

// renderMicronAsText renders Micron markup to plain text with tview colors.
func renderMicronAsText(text string) string {
	nodes := micron.Parse(text)
	var sb strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case micron.NodeHeading:
			sb.WriteString("[::b]")
			for _, child := range node.Children {
				sb.WriteString(child.Text)
			}
			sb.WriteString("[-]\n")
		case micron.NodeText:
			sb.WriteString(node.Text)
		case micron.NodeBold:
			sb.WriteString("[::b]")
		case micron.NodeItalic:
			sb.WriteString("[::i]")
		case micron.NodeUnderline:
			sb.WriteString("[::u]")
		case micron.NodeReset:
			sb.WriteString("[-]")
		case micron.NodeDivider:
			sb.WriteString(strings.Repeat("─", 30) + "\n")
		default:
			sb.WriteString(node.Text)
		}
	}
	return sb.String()
}
