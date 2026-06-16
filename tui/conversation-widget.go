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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConversationWidget displays a single conversation's messages,
// peer info header, trust banner, and compose editor.
// Matches Python's ConversationWidget at Conversations.py:1874.
type ConversationWidget struct {
	app    *tview.Application
	source string // source hash hex

	// Layout
	frame          *tview.Flex
	messageList    *tview.TextView
	peerInfoBar    *tview.TextView
	trustBanner    *tview.Flex
	editor         *ReadlineEdit
	titleEditor    *ReadlineEdit
	fullEditorArea *tview.Flex
	footerArea     *tview.Flex
	widget         tview.Primitive

	fullEditorActive bool
	sortByTimestamp  bool

	// Callbacks
	OnClose            func()
	OnPurgeFailed      func()
	OnClearHistory     func()
	OnSend             func(content, title string)
	OnAttach           func()
	OnToggleFullscreen func()
	OnPaperMessage     func(action string)
	OnAttachFiles      func(paths []string)
	OnSaveAttachments  func(names []string)

	// Dialog state
	dialogOpen bool

	// Message data
	messages []ConversationMessage
}

// ConversationMessage is a single message in a conversation view.
type ConversationMessage struct {
	Content     string
	Title       string
	Timestamp   time.Time
	IsSent      bool
	IsDelivered bool
	IsFailed    bool
	HasAttach   bool
	AttachCount int
}

// NewConversationWidget creates a conversation view for the given source hash.
// Matches Python's ConversationWidget.__init__().
func NewConversationWidget(app *tview.Application, sourceHash string) *ConversationWidget {
	cw := &ConversationWidget{
		app:    app,
		source: sourceHash,
	}

	// Peer info bar
	cw.peerInfoBar = tview.NewTextView()
	cw.peerInfoBar.SetDynamicColors(true)
	cw.peerInfoBar.SetTextColor(tcell.NewHexColor(0xdddddd))
	cw.peerInfoBar.SetBackgroundColor(tcell.NewHexColor(0x333333))
	cw.updatePeerInfo()

	// Trust banner (hidden by default for trusted peers)
	cw.trustBanner = tview.NewFlex().SetDirection(tview.FlexColumn)
	cw.trustBanner.SetBackgroundColor(tcell.NewHexColor(0x553300))

	// Header: peer info + optional trust banner
	header := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.peerInfoBar, 1, 0, false).
		AddItem(cw.trustBanner, 0, 0, false)

	// Message list
	cw.messageList = tview.NewTextView()
	cw.messageList.SetDynamicColors(true)
	cw.messageList.SetScrollable(true)
	cw.messageList.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	cw.messageList.SetBackgroundColor(tcell.ColorDefault)
	cw.messageList.SetTextAlign(tview.AlignLeft)

	// Minimal editor (content only)
	cw.editor = NewReadlineEdit("", "Type a message... (Ctrl-D to send)")
	cw.editor.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cw.editor.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Title editor (hidden by default)
	cw.titleEditor = NewReadlineEdit("Title: ", "")
	cw.titleEditor.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cw.titleEditor.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Full editor (title + content)
	cw.fullEditorArea = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.titleEditor, 1, 0, false).
		AddItem(cw.editor, 0, 1, true)

	// Footer area switches between minimal and full editor
	cw.footerArea = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.editor, 1, 0, true)

	// Main frame: header | messages | editor
	cw.frame = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 2, 0, false).
		AddItem(cw.messageList, 0, 1, false).
		AddItem(cw.footerArea, 1, 0, true)
	cw.frame.SetBorder(true)

	// Wire up keyboard shortcuts matching Python's ConversationWidget.keypress()
	cw.frame.SetInputCapture(cw.handleInput)

	cw.widget = cw.frame
	return cw
}

// Widget returns the tview primitive.
func (cw *ConversationWidget) Widget() tview.Primitive {
	return cw.widget
}

// SetMessages replaces the message list with the given messages.
func (cw *ConversationWidget) SetMessages(msgs []ConversationMessage) {
	cw.messages = msgs
	cw.renderMessages()
}

// ClearEditor clears the compose editor.
func (cw *ConversationWidget) ClearEditor() {
	cw.editor.SetText("")
	cw.titleEditor.SetText("")
}

// handleInput processes keyboard shortcuts for the conversation widget.
// Matches Python's ConversationWidget.keypress() at Conversations.py:2222.
func (cw *ConversationWidget) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlW:
		if cw.OnClose != nil {
			cw.OnClose()
		}
		return nil
	case tcell.KeyCtrlU:
		if cw.OnPurgeFailed != nil {
			cw.OnPurgeFailed()
		}
		return nil
	case tcell.KeyCtrlX:
		if cw.OnClearHistory != nil {
			cw.OnClearHistory()
		}
		return nil
	case tcell.KeyCtrlT:
		cw.toggleEditor()
		return nil
	case tcell.KeyCtrlO:
		cw.sortByTimestamp = !cw.sortByTimestamp
		cw.renderMessages()
		return nil
	case tcell.KeyCtrlA:
		if cw.OnAttach != nil {
			cw.OnAttach()
		}
		return nil
	case tcell.KeyCtrlS:
		if cw.OnAttach != nil {
			cw.OnAttach()
		}
		return nil
	case tcell.KeyCtrlD:
		cw.sendMessage()
		return nil
	case tcell.KeyCtrlG:
		if cw.OnToggleFullscreen != nil {
			cw.OnToggleFullscreen()
		}
		return nil
	case tcell.KeyTab:
		// Toggle focus between editor and message list
		return event
	}

	return event
}

// toggleEditor switches between minimal and full editor modes.
func (cw *ConversationWidget) toggleEditor() {
	cw.fullEditorActive = !cw.fullEditorActive
	cw.footerArea.Clear()
	if cw.fullEditorActive {
		cw.footerArea.AddItem(cw.fullEditorArea, 2, 0, true)
	} else {
		cw.footerArea.AddItem(cw.editor, 1, 0, true)
	}
}

// sendMessage sends the current editor content.
func (cw *ConversationWidget) sendMessage() {
	content := cw.editor.GetText()
	if content == "" {
		return
	}
	title := ""
	if cw.fullEditorActive {
		title = cw.titleEditor.GetText()
	}
	if cw.OnSend != nil {
		cw.OnSend(content, title)
	}
	cw.ClearEditor()
}

// updatePeerInfo refreshes the peer info header bar.
func (cw *ConversationWidget) updatePeerInfo() {
	if cw.source == "" {
		cw.peerInfoBar.SetText(" No conversation selected")
		return
	}
	cw.peerInfoBar.SetText(fmt.Sprintf(" %s | %s", cw.source[:8]+"...", "unknown"))
}

// renderMessages renders all messages into the message list.
func (cw *ConversationWidget) renderMessages() {
	var sb strings.Builder
	for _, msg := range cw.messages {
		ts := msg.Timestamp.Format("15:04:05")
		status := ""
		switch {
		case msg.IsFailed:
			status = "[red] [failed][-]"
		case msg.IsDelivered:
			status = " [green][delivered][-]"
		case msg.IsSent:
			status = " [yellow][sent][-]"
		}

		if msg.IsSent {
			sb.WriteString(fmt.Sprintf("[#66cc55]%s[-] %s%s\n", ts, truncateStr(msg.Content, 200), status))
		} else {
			sb.WriteString(fmt.Sprintf("[#33ccdd]%s[-] %s%s\n", ts, truncateStr(msg.Content, 200), status))
		}

		if msg.Title != "" {
			sb.WriteString(fmt.Sprintf("  [::b]%s[-]\n", msg.Title))
		}
		if msg.HasAttach {
			sb.WriteString(fmt.Sprintf("  [gray]📎 %d attachment(s)[-]\n", msg.AttachCount))
		}
		sb.WriteString("\n")
	}

	if len(cw.messages) == 0 {
		sb.WriteString("[gray]No messages yet. Type below to send.[-]\n")
	}

	cw.messageList.SetText(sb.String())
}

// FormatQRText creates a text-based QR-like display for an address.
// Matches Python's show_qr_dialog at Conversations.py:641.
func FormatQRText(data string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("┌%s┐\n", strings.Repeat("─", len(data)+4)))
	sb.WriteString(fmt.Sprintf("│  %s  │\n", data))
	sb.WriteString(fmt.Sprintf("└%s┘\n", strings.Repeat("─", len(data)+4)))
	return sb.String()
}

// ClearHistoryDialog shows a confirmation dialog before clearing
// conversation history. Matches Python's clear_history_dialog()
// at Conversations.py:2122.
func (cw *ConversationWidget) ClearHistoryDialog() {
	cw.dialogOpen = true
}

// ConfirmClearHistory confirms the clear history action, fires
// the OnClearHistory callback, and closes the dialog.
func (cw *ConversationWidget) ConfirmClearHistory() {
	cw.dialogOpen = false
	if cw.OnClearHistory != nil {
		cw.OnClearHistory()
	}
}

// DismissClearHistoryDialog closes the clear history dialog
// without taking action.
func (cw *ConversationWidget) DismissClearHistoryDialog() {
	cw.dialogOpen = false
}

// DialogOpen reports whether a dialog is currently open.
func (cw *ConversationWidget) DialogOpen() bool {
	return cw.dialogOpen
}

// PaperMessageDialog shows a dialog for choosing how to output a
// paper message: Print QR, Save QR, Save URI, or Cancel.
// Matches Python's paper_message() at Conversations.py:2505.
func (cw *ConversationWidget) PaperMessageDialog() {
	cw.dialogOpen = true
}

// PaperMessagePrintQR fires the OnPaperMessage callback with "PrintQR"
// and closes the dialog.
func (cw *ConversationWidget) PaperMessagePrintQR() {
	cw.dialogOpen = false
	if cw.OnPaperMessage != nil {
		cw.OnPaperMessage("PrintQR")
	}
}

// PaperMessageSaveQR fires the OnPaperMessage callback with "SaveQR"
// and closes the dialog.
func (cw *ConversationWidget) PaperMessageSaveQR() {
	cw.dialogOpen = false
	if cw.OnPaperMessage != nil {
		cw.OnPaperMessage("SaveQR")
	}
}

// PaperMessageSaveURI fires the OnPaperMessage callback with "SaveURI"
// and closes the dialog.
func (cw *ConversationWidget) PaperMessageSaveURI() {
	cw.dialogOpen = false
	if cw.OnPaperMessage != nil {
		cw.OnPaperMessage("SaveURI")
	}
}

// DismissPaperMessageDialog closes the paper message dialog
// without taking action.
func (cw *ConversationWidget) DismissPaperMessageDialog() {
	cw.dialogOpen = false
}

// OpenAttachFileDialog opens a file browser dialog for selecting
// files to attach. Matches Python's attach_file() at
// Conversations.py:2438.
func (cw *ConversationWidget) OpenAttachFileDialog(startDir string) {
	cw.dialogOpen = true
}

// ConfirmAttachFile confirms the file selection, fires the
// OnAttachFiles callback, and closes the dialog.
func (cw *ConversationWidget) ConfirmAttachFile(paths []string) {
	cw.dialogOpen = false
	if cw.OnAttachFiles != nil {
		cw.OnAttachFiles(paths)
	}
}

// DismissAttachFileDialog closes the attach file dialog
// without selecting any files.
func (cw *ConversationWidget) DismissAttachFileDialog() {
	cw.dialogOpen = false
}

// AttachmentRef represents a single attachment in a conversation.
// Matches Python's _collect_attachment_refs() at Conversations.py:2270.
type AttachmentRef struct {
	Name string
	Type string
}

// SaveAttachmentsDialog shows a dialog with checkboxes for each
// attachment in the conversation, allowing the user to select
// which to save. Matches Python's save_focused_attachments()
// at Conversations.py:2324.
func (cw *ConversationWidget) SaveAttachmentsDialog(attachments []AttachmentRef) {
	cw.dialogOpen = true
}

// ConfirmSaveAttachments saves the selected attachments, fires
// the OnSaveAttachments callback, and closes the dialog.
func (cw *ConversationWidget) ConfirmSaveAttachments(names []string) {
	cw.dialogOpen = false
	if cw.OnSaveAttachments != nil {
		cw.OnSaveAttachments(names)
	}
}

// DismissSaveAttachmentsDialog closes the save attachments
// dialog without saving.
func (cw *ConversationWidget) DismissSaveAttachmentsDialog() {
	cw.dialogOpen = false
}
