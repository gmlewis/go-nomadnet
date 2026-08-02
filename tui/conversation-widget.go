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
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConversationWidget displays a single conversation's messages,
// peer info header, trust banner, and compose editor.
// Matches Python's ConversationWidget at Conversations.py:1874.
type ConversationWidget struct {
	app    *App
	source string // source hash hex

	// Peer info data (injected by the caller; tui must not import the app
	// package). DisplayName "" → falls back to <full hash> (RNS.prettyhexrep).
	// StampCost nil → omit the "Stamp: N" segment. Hops nil → "unknown".
	TrustLevel  string
	DisplayName string
	StampCost   *int
	Hops        *int

	// OwnHash is this app's LXMF destination hash (app.lxmf_destination.hash),
	// injected by the wiring layer so LXMessageHeader can tell outbound from
	// inbound. TimeFormat is the configured strftime format (default
	// "%Y-%m-%d %H:%M:%S", Python app.time_format).
	OwnHash    []byte
	TimeFormat string

	// Layout
	frame          *tview.Flex
	headerFlex     *tview.Flex
	messageList    *tview.TextView
	peerInfoBar    *tview.TextView
	trustBanner    *tview.Flex
	editor         *ReadlineEdit
	titleEditor    *ReadlineEdit
	fullEditorArea *tview.Flex
	footerArea     *tview.Flex
	// attachmentIndicator is the pending-attachments footer line (Python
	// _build_footer, Conversations.py:2167-2175). Lazily created by buildFooter.
	attachmentIndicator *tview.TextView
	widget              tview.Primitive

	fullEditorActive     bool
	sortByTimestamp      bool
	trustBannerDismissed bool

	// Callbacks
	OnClose                  func()
	OnPurgeFailed            func()
	OnClearHistory           func()
	OnSend                   func(content, title string, attachments []string)
	OnAttach                 func()
	OnToggleFullscreen       func()
	OnPaperMessage           func(action, content, title string) (path string, ok bool)
	OnPaperMessageSaved      func(path string)
	OnPaperMessageFailed     func()
	OnPaperMessageRequested  func()
	OnAttachFiles            func(paths []string)
	OnSaveAttachments        func(refs []AttachmentRef)
	OnSaveFocusedAttachments func(refs []AttachmentRef)
	// Trust banner button callbacks (Python _on_trust_click/_on_block_click/
	// _on_ignore_click, Conversations.py:1989-2030).
	OnTrust  func()
	OnBlock  func()
	OnIgnore func()

	// Dialog state
	dialogOpen bool

	// pendingAttachments is the list of staged file paths to attach to the
	// next sent message (Python ConversationWidget.pending_attachments,
	// Conversations.py:1891,2167). Populated by ConfirmAttachFile (the file
	// browser selection); consumed and cleared by sendMessage.
	pendingAttachments []string

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

	// LXMF wire fields used by LXMessageHeader for Python-parity rendering
	// (Conversations.py:2596-2670). When State/SourceHash are zero, the
	// legacy IsSent/IsDelivered/IsFailed bools drive a fallback derivation.
	State                int
	Method               int
	SourceHash           []byte
	Hash                 []byte // LXMF message hash — locates extracted attachments for C-s save
	TransportEncrypted   bool
	SignatureValidated   bool
	SignatureDescription string
	AttachmentTypes      []string
	AttachmentNames      []string
}

// NewConversationWidget creates a conversation view for the given source hash.
// Matches Python's ConversationWidget.__init__().
func NewConversationWidget(app *App, sourceHash string) *ConversationWidget {
	cw := &ConversationWidget{
		app:    app,
		source: sourceHash,
	}

	// Peer info bar — style "msg_header_sent" (TextUI.py:35/88): true-color
	// fg #111, bg #ddd.
	cw.peerInfoBar = tview.NewTextView()
	cw.peerInfoBar.SetDynamicColors(true)
	cw.peerInfoBar.SetTextColor(tcell.NewHexColor(0x111111))
	cw.peerInfoBar.SetBackgroundColor(tcell.NewHexColor(0xdddddd))
	cw.updatePeerInfo()

	// Trust banner — style "msg_warning_untrusted" (TextUI.py:39/92): true-color
	// fg #111, bg dark red (#800000). Hidden by default; refreshTrustBanner
	// reveals it for non-trusted peers (Python _refresh_trust_banner,
	// Conversations.py:1962).
	cw.trustBanner = tview.NewFlex().SetDirection(tview.FlexColumn)
	cw.trustBanner.SetBackgroundColor(tcell.NewHexColor(0x800000))

	// Header: peer info (1 row) + optional trust banner (0 rows when hidden).
	header := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.peerInfoBar, 1, 0, false).
		AddItem(cw.trustBanner, 0, 0, false)
	cw.headerFlex = header
	cw.refreshTrustBanner()

	// Message list
	cw.messageList = tview.NewTextView()
	cw.messageList.SetDynamicColors(true)
	cw.messageList.SetScrollable(true)
	cw.messageList.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	cw.messageList.SetBackgroundColor(tcell.ColorDefault)
	cw.messageList.SetTextAlign(tview.AlignLeft)

	// Minimal editor (content only)
	cw.editor = NewReadlineEdit(app.killRing, "", "Type a message... (Ctrl-D to send)")
	cw.editor.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cw.editor.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Title editor (hidden by default)
	cw.titleEditor = NewReadlineEdit(app.killRing, "Title: ", "")
	cw.titleEditor.SetFieldBackgroundColor(tcell.NewHexColor(0x222222))
	cw.titleEditor.SetFieldTextColor(tcell.NewHexColor(0xdddddd))

	// Full editor (title + content)
	cw.fullEditorArea = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cw.titleEditor, 1, 0, false).
		AddItem(cw.editor, 0, 1, true)

	// Footer area switches between minimal and full editor, optionally
	// prepending the pending-attachments indicator (Python _build_footer,
	// Conversations.py:2160-2177). Populated by buildFooter after the frame
	// exists (it resizes the frame's footer slot).
	cw.footerArea = tview.NewFlex().SetDirection(tview.FlexRow)

	// Main frame: header | messages | editor
	cw.frame = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 2, 0, false).
		AddItem(cw.messageList, 0, 1, false).
		AddItem(cw.footerArea, 1, 0, true)
	cw.frame.SetBorder(true)
	cw.buildFooter()

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
	cw.pendingAttachments = nil
	cw.buildFooter()
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
	case tcell.KeyCtrlF:
		// C-f → attach_file, same as C-a. Python's MessageEdit.keypress
		// (Conversations.py:1813) binds ctrl f to attach_file; the frame
		// keypress binds ctrl a (Conversations.py:2237). Both reach the same
		// action, so bind both here.
		if cw.OnAttach != nil {
			cw.OnAttach()
		}
		return nil
	case tcell.KeyCtrlS:
		cw.saveFocusedAttachments()
		return nil
	case tcell.KeyCtrlP:
		// C-p → paper_message (Python MessageEdit.keypress,
		// Conversations.py:1811). Opens the paper-message dialog.
		cw.PaperMessageDialog()
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
	cw.buildFooter()
}

// buildFooter rebuilds the footer area, mirroring Python's _build_footer
// (Conversations.py:2160-2177): a pending-attachments indicator line
// ("{file-glyph} N file(s): {basenames}") sits above the editor when
// pendingAttachments is non-empty; otherwise only the editor shows. The frame's
// footer slot is resized to the total row count.
func (cw *ConversationWidget) buildFooter() {
	cw.footerArea.Clear()
	if cw.attachmentIndicator != nil {
		cw.attachmentIndicator.Clear()
	}
	rows := 0
	if len(cw.pendingAttachments) > 0 {
		if cw.attachmentIndicator == nil {
			cw.attachmentIndicator = tview.NewTextView()
		}
		cw.attachmentIndicator.Clear()
		g := cw.glyphs()
		names := make([]string, len(cw.pendingAttachments))
		for i, p := range cw.pendingAttachments {
			names[i] = filepath.Base(p)
		}
		fmt.Fprintf(cw.attachmentIndicator, "%s %d file(s): %s", g["file"], len(cw.pendingAttachments), strings.Join(names, ", "))
		cw.footerArea.AddItem(cw.attachmentIndicator, 1, 0, false)
		rows++
	}
	if cw.fullEditorActive {
		cw.footerArea.AddItem(cw.fullEditorArea, 2, 0, true)
		rows += 2
	} else {
		cw.footerArea.AddItem(cw.editor, 1, 0, true)
		rows++
	}
	if cw.frame != nil {
		cw.frame.ResizeItem(cw.footerArea, rows, 0)
	}
}

// footerIndicatorText returns the current pending-attachments indicator text
// (empty when no attachments are staged), for testing/parity verification.
func (cw *ConversationWidget) footerIndicatorText() string {
	if cw.attachmentIndicator == nil {
		return ""
	}
	return cw.attachmentIndicator.GetText(true)
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
	// Hand off the staged attachments (file paths) to the wiring layer so
	// it can build the LXMF FIELD_FILE_ATTACHMENTS field, then clear the
	// staging list (Python send_message + clear_editor,
	// Conversations.py:2412-2436,2294-2298).
	attachments := cw.pendingAttachments
	cw.pendingAttachments = nil
	if cw.OnSend != nil {
		cw.OnSend(content, title, attachments)
	}
	cw.ClearEditor()
}

// updatePeerInfo refreshes the peer info header bar, mirroring Python's
// _update_peer_info (Conversations.py:2084-2120): " {name} | {right} " where
// name is the display name or <full hash> (RNS.prettyhexrep), and right joins
// "Stamp: N" (when a stamp cost is known) and "{speed}{hops}" by two spaces.
// hops is "N hop"/"N hops"/"unknown". RNS-dependent fields (stamp cost, hop
// count, app-data-derived name) are injected by the caller via StampCost/Hops/
// DisplayName; nil values yield the same fallbacks Python uses when RNS data
// is unavailable.
func (cw *ConversationWidget) updatePeerInfo() {
	if cw.source == "" {
		cw.peerInfoBar.SetText(" No conversation selected")
		return
	}
	name := cw.DisplayName
	if name == "" {
		name = "<" + cw.source + ">" // RNS.prettyhexrep
	}

	g := cw.app.Glyphs
	if g == nil {
		g = glyphsUnicode
	}
	speed := g["speed"]

	var hopsStr string
	switch {
	case cw.Hops == nil:
		hopsStr = "unknown"
	case *cw.Hops == 1:
		hopsStr = "1 hop"
	default:
		hopsStr = fmt.Sprintf("%d hops", *cw.Hops)
	}

	var rightParts []string
	if cw.StampCost != nil {
		rightParts = append(rightParts, fmt.Sprintf("Stamp: %d", *cw.StampCost))
	}
	rightParts = append(rightParts, speed+hopsStr)

	cw.peerInfoBar.SetText(" " + name + " | " + strings.Join(rightParts, "  ") + " ")
}

// hasVisibleTrustBanner reports whether the trust banner should show,
// mirroring Python's has_visible_trust_banner (Conversations.py:1953-1960):
// false when dismissed or when the peer is trusted.
func (cw *ConversationWidget) hasVisibleTrustBanner() bool {
	if cw.trustBannerDismissed {
		return false
	}
	return cw.TrustLevel != "trusted"
}

// refreshTrustBanner shows or hides the trust banner in the header pile,
// mirroring Python's _refresh_trust_banner (Conversations.py:1962-1973).
func (cw *ConversationWidget) refreshTrustBanner() {
	if cw.hasVisibleTrustBanner() {
		cw.buildTrustBanner()
		cw.headerFlex.ResizeItem(cw.trustBanner, 1, 0)
	} else {
		cw.trustBanner.Clear()
		cw.headerFlex.ResizeItem(cw.trustBanner, 0, 0)
	}
}

// buildTrustBanner populates the trust banner row, mirroring Python's
// _build_trust_banner (Conversations.py:1975-1987): a warning message on the
// left followed by Trust / Block / Do nothing buttons, on the dark-red
// "msg_warning_untrusted" background.
func (cw *ConversationWidget) buildTrustBanner() {
	cw.trustBanner.Clear()
	g := cw.app.Glyphs
	if g == nil {
		g = glyphsUnicode
	}
	fg := tcell.NewHexColor(0x111111)
	bg := tcell.NewHexColor(0x800000)

	msg := tview.NewTextView()
	msg.SetDynamicColors(true)
	msg.SetText(" " + g["warning"] + " This peer isn't trusted yet.")
	msg.SetTextColor(fg)
	msg.SetBackgroundColor(bg)

	button := func(label string, fn func()) *tview.Button {
		b := tview.NewButton(label).SetSelectedFunc(fn)
		b.SetBackgroundColor(bg)
		b.SetLabelColor(fg)
		b.SetLabelColorActivated(tcell.NewHexColor(0x800000))
		b.SetBackgroundColorActivated(tcell.NewHexColor(0x111111))
		return b
	}
	btnTrust := button("Trust", cw.trustClick)
	btnBlock := button("Block", cw.blockClick)
	btnNothing := button("Do nothing", cw.ignoreClick)
	spacer := func() *tview.TextView {
		s := tview.NewTextView()
		s.SetBackgroundColor(bg)
		return s
	}

	cw.trustBanner.SetDirection(tview.FlexColumn).
		AddItem(msg, 0, 1, false).
		AddItem(btnTrust, 8, 0, true).
		AddItem(spacer(), 1, 0, false).
		AddItem(btnBlock, 8, 0, false).
		AddItem(spacer(), 1, 0, false).
		AddItem(btnNothing, 13, 0, false).
		AddItem(spacer(), 1, 0, false)
	cw.trustBanner.SetBackgroundColor(bg)
}

// SetTrustLevel sets the peer's trust level and refreshes the trust banner
// (so trusting/blocking/dismissing updates the header immediately).
func (cw *ConversationWidget) SetTrustLevel(level string) {
	cw.TrustLevel = level
	cw.refreshTrustBanner()
}

// trustClick fires the OnTrust callback (Python _on_trust_click,
// Conversations.py:1989).
func (cw *ConversationWidget) trustClick() {
	if cw.OnTrust != nil {
		cw.OnTrust()
	}
}

// blockClick fires the OnBlock callback (Python _on_block_click).
func (cw *ConversationWidget) blockClick() {
	if cw.OnBlock != nil {
		cw.OnBlock()
	}
}

// ignoreClick dismisses the trust banner and fires OnIgnore (Python
// _on_ignore_click).
func (cw *ConversationWidget) ignoreClick() {
	cw.trustBannerDismissed = true
	cw.refreshTrustBanner()
	if cw.OnIgnore != nil {
		cw.OnIgnore()
	}
}

// renderMessages renders all messages into the message list.
// renderMessages renders the message list into the messageList TextView. Each
// message header (title string + style) is built by LXMessageHeader for
// Python LXMessageWidget parity (Conversations.py:2576-2670); the message body
// is the indented content (Python indents every line two columns).
func (cw *ConversationWidget) renderMessages() {
	var sb strings.Builder
	for _, msg := range cw.messages {
		in := cw.headerInputs(msg)
		title, style := LXMessageHeader(in)
		// Header style colors are applied via tview tags mapped from the urwid
		// style name; the title text is rendered on its own line(s).
		sb.WriteString(styleHeader(title, style))
		sb.WriteString("\n")

		// Body: indent every content line two columns (Python LXMessageWidget
		// "  "+line for non-markdown content).
		for _, line := range strings.Split(msg.Content, "\n") {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		if msg.HasAttach && len(msg.AttachmentNames) == 0 {
			sb.WriteString(fmt.Sprintf("  [gray]%s %d attachment(s)[-]\n", cw.glyphs()["file"], msg.AttachCount))
		}
		sb.WriteString("\n")
	}

	if len(cw.messages) == 0 {
		sb.WriteString("[gray]No messages yet. Type below to send.[-]\n")
	}

	cw.messageList.SetText(sb.String())
}

// headerInputs builds MessageHeaderInputs for a message, deriving the LXMF
// wire fields from the legacy Is* bools when State/SourceHash are unset so
// older callers still render a sensible header.
func (cw *ConversationWidget) headerInputs(msg ConversationMessage) MessageHeaderInputs {
	in := MessageHeaderInputs{
		Timestamp:            msg.Timestamp,
		Now:                  time.Now(),
		State:                msg.State,
		Method:               msg.Method,
		SourceHash:           msg.SourceHash,
		OwnHash:              cw.OwnHash,
		TransportEncrypted:   msg.TransportEncrypted,
		Title:                msg.Title,
		SignatureValidated:   msg.SignatureValidated,
		SignatureDescription: msg.SignatureDescription,
		AttachmentTypes:      msg.AttachmentTypes,
		AttachmentNames:      msg.AttachmentNames,
		TimeFormat:           cw.timeFormat(),
		Glyphs:               cw.glyphs(),
	}

	// Legacy fallback when the LXMF wire fields are unset.
	if in.State == 0 && in.SourceHash == nil && in.Method == 0 && !in.TransportEncrypted {
		switch {
		case msg.IsSent:
			own := in.OwnHash
			if own == nil {
				own = []byte{0x00}
			}
			in.OwnHash = own
			in.SourceHash = own
			in.TransportEncrypted = true
			switch {
			case msg.IsFailed:
				in.State = lxmfStateFailed
			case msg.IsDelivered:
				in.State = lxmfStateDelivered
			default:
				in.State = lxmfStateSent
				in.Method = lxmfMethodPropagated
			}
		case msg.IsFailed:
			in.State = lxmfStateFailed // failed_no_source
		default:
			in.SourceHash = []byte{0xff} // inbound, distinct from any own hash
			in.SignatureValidated = true
		}
	}
	return in
}

// styleHeader renders a header title with the urwid style name's tview color
// mapping. The LXMessageWidget header styles map to background colors; here we
// apply a foreground tag derived from the style so the title is visible on the
// default background. The title may contain a "\n  " continuation (unvalidated
// inbound signatures), which is preserved.
func styleHeader(title, style string) string {
	fg := headerStyleForeground(style)
	if fg == "" {
		return title + "\n"
	}
	return "[" + fg + "]" + title + "[-]\n"
}

// headerStyleForeground maps an LXMessageWidget urwid header style name to a
// tview foreground color tag. The Python styles carry bg colors (e.g.
// msg_header_sent bg #ddd); the port renders the header text with a
// representative foreground so it is legible on the default background.
func headerStyleForeground(style string) string {
	switch style {
	case "msg_header_failed":
		return "red"
	case "msg_header_delivered":
		return "green"
	case "msg_header_propagated":
		return "yellow"
	case "msg_header_sent":
		return "#66cc55"
	case "msg_header_ok":
		return "#33ccdd"
	case "msg_header_caution":
		return "yellow"
	default:
		return ""
	}
}

// glyphs returns the glyph set for this widget, falling back to the unicode
// set when no app/glyphs are available.
func (cw *ConversationWidget) glyphs() GlyphSet {
	if cw.app != nil && cw.app.Glyphs != nil {
		return cw.app.Glyphs
	}
	return glyphsUnicode
}

// timeFormat returns the configured strftime format, defaulting to the Python
// app.time_format default.
func (cw *ConversationWidget) timeFormat() string {
	if cw.TimeFormat != "" {
		return cw.TimeFormat
	}
	return "%Y-%m-%d %H:%M:%S"
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
	cw.app.Dialogs.ShowConfirmDialog("Clear conversation history?", func() {
		cw.ConfirmClearHistory()
	}, func() {
		cw.DismissClearHistoryDialog()
	})
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
// Matches Python's paper_message() at Conversations.py:2505. The widget marks
// its dialog state open and delegates the actual dialog rendering to the
// wiring layer via OnPaperMessageRequested (the display's PaperMessageDialog
// shows the real button overlay).
func (cw *ConversationWidget) PaperMessageDialog() {
	cw.dialogOpen = true
	if cw.OnPaperMessageRequested != nil {
		cw.OnPaperMessageRequested()
	}
}

// paperAction runs a paper-message output action, mirroring Python's
// print_paper_message_qr / save_paper_message_qr / save_paper_message_uri
// (Conversations.py:2474-2503): read the editor content+title, short-circuit on
// empty content, fire OnPaperMessage(action, content, title) which returns the
// saved path and ok, then on success clear the editor (and for the save modes
// fire OnPaperMessageSaved with the path) or on failure fire
// OnPaperMessageFailed and leave the editor intact.
func (cw *ConversationWidget) paperAction(action string, saved bool) {
	cw.dialogOpen = false
	content := cw.editor.GetText()
	if content == "" {
		return
	}
	title := cw.titleEditor.GetText()
	if cw.OnPaperMessage == nil {
		return
	}
	path, ok := cw.OnPaperMessage(action, content, title)
	if !ok {
		if cw.OnPaperMessageFailed != nil {
			cw.OnPaperMessageFailed()
		}
		return
	}
	cw.ClearEditor()
	if saved && cw.OnPaperMessageSaved != nil {
		cw.OnPaperMessageSaved(path)
	}
}

// PaperMessagePrintQR fires OnPaperMessage with "PrintQR" and closes the dialog.
func (cw *ConversationWidget) PaperMessagePrintQR() { cw.paperAction("PrintQR", false) }

// PaperMessageSaveQR fires OnPaperMessage with "SaveQR" and closes the dialog.
func (cw *ConversationWidget) PaperMessageSaveQR() { cw.paperAction("SaveQR", true) }

// PaperMessageSaveURI fires OnPaperMessage with "SaveURI" and closes the dialog.
func (cw *ConversationWidget) PaperMessageSaveURI() { cw.paperAction("SaveURI", true) }

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
	// Stage the selected file paths for the next send (Python appends to
	// pending_attachments in the file-browser selection handler,
	// Conversations.py:3057-3060).
	cw.pendingAttachments = append(cw.pendingAttachments, paths...)
	cw.buildFooter()
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
// Matches Python's _collect_attachment_refs() at Conversations.py:2300: each
// ref carries the attachment's display name, field type, the owning message's
// LXMF hash (to locate the extracted attachment directory), and the field index
// within that message (to select the extracted file_ N).
type AttachmentRef struct {
	Name        string
	Type        string
	MessageHash []byte
	FieldIndex  int
}

// collectAttachmentRefs gathers every attachment across all messages, ordered
// by sort_timestamp descending (newest message first). Matches Python's
// _collect_attachment_refs() at Conversations.py:2300-2322: it iterates the
// conversation's messages sorted by sort_timestamp reverse and emits one ref
// per attachment (file or otherwise) on every message that has attachments.
func (cw *ConversationWidget) collectAttachmentRefs() []AttachmentRef {
	sorted := make([]ConversationMessage, len(cw.messages))
	copy(sorted, cw.messages)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})

	var refs []AttachmentRef
	for _, msg := range sorted {
		if !msg.HasAttach || len(msg.AttachmentNames) == 0 {
			continue
		}
		for i, name := range msg.AttachmentNames {
			atype := "file"
			if i < len(msg.AttachmentTypes) && msg.AttachmentTypes[i] != "" {
				atype = msg.AttachmentTypes[i]
			}
			refs = append(refs, AttachmentRef{
				Name:        name,
				Type:        atype,
				MessageHash: msg.Hash,
				FieldIndex:  i,
			})
		}
	}
	return refs
}

// saveFocusedAttachments opens the save-attachments flow for the current
// conversation. Matches Python's save_focused_attachments() at
// Conversations.py:2324: it sets dialog_active, collects the attachment refs,
// and hands them to the delegate (wiring layer) to render the save dialog.
// Distinct from C-a/attach_file.
func (cw *ConversationWidget) saveFocusedAttachments() {
	cw.dialogOpen = true
	refs := cw.collectAttachmentRefs()
	if cw.OnSaveFocusedAttachments != nil {
		cw.OnSaveFocusedAttachments(refs)
	}
}

// SaveAttachmentsDialog shows a dialog with checkboxes for each
// attachment in the conversation, allowing the user to select
// which to save. Matches Python's save_focused_attachments()
// at Conversations.py:2324.
func (cw *ConversationWidget) SaveAttachmentsDialog(attachments []AttachmentRef) {
	cw.dialogOpen = true
}

// ConfirmSaveAttachments saves the selected attachments, fires
// the OnSaveAttachments callback with the selected refs, and closes the dialog.
func (cw *ConversationWidget) ConfirmSaveAttachments(refs []AttachmentRef) {
	cw.dialogOpen = false
	if cw.OnSaveAttachments != nil {
		cw.OnSaveAttachments(refs)
	}
}

// DismissSaveAttachmentsDialog closes the save attachments
// dialog without saving.
func (cw *ConversationWidget) DismissSaveAttachmentsDialog() {
	cw.dialogOpen = false
}
