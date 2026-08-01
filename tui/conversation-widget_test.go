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
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestNewConversationWidget(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	if cw == nil {
		t.Fatal("NewConversationWidget returned nil")
	}
	if cw.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestConversationWidgetEmptySource(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "")
	if cw == nil {
		t.Fatal("NewConversationWidget returned nil")
	}
}

func TestConversationWidgetSetMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	now := time.Now()
	msgs := []ConversationMessage{
		{Content: "Hello", Timestamp: now, IsSent: true},
		{Content: "Hi there", Timestamp: now.Add(time.Minute), IsSent: false},
		{Content: "How are you?", Timestamp: now.Add(2 * time.Minute), IsSent: true, IsFailed: true},
	}

	cw.SetMessages(msgs)
	if len(cw.messages) != 3 {
		t.Errorf("SetMessages: got %d messages, want 3", len(cw.messages))
	}
}

func TestConversationWidgetClearEditor(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.ClearEditor()
	if cw.editor.GetText() != "" {
		t.Error("ClearEditor should clear content editor")
	}
}

func TestConversationWidgetSendMessage(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	var sentContent string
	cw.OnSend = func(content, title string) {
		sentContent = content
	}

	cw.editor.SetText("Hello world")
	cw.sendMessage()

	if sentContent != "Hello world" {
		t.Errorf("sendMessage content = %q, want %q", sentContent, "Hello world")
	}
	if cw.editor.GetText() != "" {
		t.Error("sendMessage should clear editor")
	}
}

func TestConversationWidgetSendMessageEmpty(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	sent := false
	cw.OnSend = func(content, title string) { sent = true }
	cw.sendMessage()

	if sent {
		t.Error("Empty message should not be sent")
	}
}

func TestConversationWidgetKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	var fired []string
	cw.OnClose = func() { fired = append(fired, "close") }
	cw.OnPurgeFailed = func() { fired = append(fired, "purge") }
	cw.OnClearHistory = func() { fired = append(fired, "clear") }
	cw.OnAttach = func() { fired = append(fired, "attach") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-w", tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone), "close"},
		{"ctrl-u", tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone), "purge"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "clear"},
		{"ctrl-t", tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModNone), ""},
		{"ctrl-o", tcell.NewEventKey(tcell.KeyCtrlO, 0, tcell.ModNone), ""},
		{"ctrl-a", tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone), "attach"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := cw.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %s was not consumed", tt.name)
			}
			if tt.want == "" {
				if len(fired) != 0 {
					t.Errorf("key %s should not fire callbacks, fired %v", tt.name, fired)
				}
			} else {
				if len(fired) != 1 || fired[0] != tt.want {
					t.Errorf("key %s fired %v, want [%s]", tt.name, fired, tt.want)
				}
			}
		})
	}
}

// TestPeerInfoPythonParity checks the peer-info header bar text against golden
// values captured from Python's _update_peer_info (Conversations.py:2084-2120).
func TestPeerInfoPythonParity(t *testing.T) {
	t.Parallel()

	fullHash := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	stamp3 := 3
	hops2 := 2
	hops1 := 1

	tests := []struct {
		name string
		cw   *ConversationWidget
		want string
	}{
		{
			"unknown peer",
			func() *ConversationWidget {
				cw := NewConversationWidget(newTestApp(), fullHash)
				return cw
			}(),
			" <" + fullHash + "> | ◷ unknown ",
		},
		{
			"named 2 hops stamp 3",
			func() *ConversationWidget {
				cw := NewConversationWidget(newTestApp(), fullHash)
				cw.DisplayName = "Alice"
				cw.StampCost = &stamp3
				cw.Hops = &hops2
				cw.updatePeerInfo()
				return cw
			}(),
			" Alice | Stamp: 3  ◷ 2 hops ",
		},
		{
			"named 1 hop",
			func() *ConversationWidget {
				cw := NewConversationWidget(newTestApp(), fullHash)
				cw.DisplayName = "Alice"
				cw.StampCost = &stamp3
				cw.Hops = &hops1
				cw.updatePeerInfo()
				return cw
			}(),
			" Alice | Stamp: 3  ◷ 1 hop ",
		},
		{
			"named no stamp unknown hops",
			func() *ConversationWidget {
				cw := NewConversationWidget(newTestApp(), fullHash)
				cw.DisplayName = "Bob"
				cw.updatePeerInfo()
				return cw
			}(),
			" Bob | ◷ unknown ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.cw.peerInfoBar.GetText(false)
			if got != tt.want {
				t.Errorf("peerInfo = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTrustBannerVisibility checks the trust banner shows for non-trusted
// peers and is hidden for trusted / after dismissal (Python
// has_visible_trust_banner, Conversations.py:1953-1960).
func TestTrustBannerVisibility(t *testing.T) {
	t.Parallel()

	app := newTestApp()

	// Untrusted → banner visible.
	cw := NewConversationWidget(app, "aabb1122")
	cw.TrustLevel = "untrusted"
	cw.refreshTrustBanner()
	if !cw.hasVisibleTrustBanner() {
		t.Error("untrusted: banner should be visible")
	}

	// Trusted → banner hidden.
	cw.SetTrustLevel("trusted")
	if cw.hasVisibleTrustBanner() {
		t.Error("trusted: banner should be hidden")
	}

	// Unknown → visible.
	cw.SetTrustLevel("unknown")
	if !cw.hasVisibleTrustBanner() {
		t.Error("unknown: banner should be visible")
	}

	// Warning → visible.
	cw.SetTrustLevel("warning")
	if !cw.hasVisibleTrustBanner() {
		t.Error("warning: banner should be visible")
	}
}

// TestTrustBannerButtonCallbacks verifies the Trust/Block/Do nothing buttons
// fire their callbacks and "Do nothing" dismisses the banner.
func TestTrustBannerButtonCallbacks(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.TrustLevel = "unknown"
	cw.refreshTrustBanner()

	var fired []string
	cw.OnTrust = func() { fired = append(fired, "trust") }
	cw.OnBlock = func() { fired = append(fired, "block") }
	cw.OnIgnore = func() { fired = append(fired, "ignore") }

	cw.trustClick()
	cw.blockClick()
	if len(fired) != 2 || fired[0] != "trust" || fired[1] != "block" {
		t.Errorf("trust/block fired %v, want [trust block]", fired)
	}

	cw.ignoreClick()
	if !cw.trustBannerDismissed {
		t.Error("ignoreClick should dismiss the banner")
	}
	if cw.hasVisibleTrustBanner() {
		t.Error("after ignore, banner should be hidden")
	}
}

func TestConversationWidgetToggleEditor(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	if cw.fullEditorActive {
		t.Error("Editor should start in minimal mode")
	}

	cw.toggleEditor()
	if !cw.fullEditorActive {
		t.Error("toggleEditor should activate full editor")
	}

	cw.toggleEditor()
	if cw.fullEditorActive {
		t.Error("toggleEditor again should return to minimal editor")
	}
}

func TestConversationWidgetRenderMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	cw.SetMessages(nil)
	text := cw.messageList.GetText(false)
	if text == "" {
		t.Error("Empty message list should show placeholder")
	}

	now := time.Now()
	cw.SetMessages([]ConversationMessage{
		{Content: "Hello", Timestamp: now, IsSent: true},
		{Content: "Hi", Timestamp: now.Add(time.Minute), IsSent: false, HasAttach: true, AttachCount: 2},
	})
	text = cw.messageList.GetText(false)
	if text == "" {
		t.Error("Message list should have content after SetMessages")
	}
}

// TestConversationWidgetRenderHeaderParity checks renderMessages emits the
// LXMessageWidget header (prefix glyph + strftime timestamp + encryption glyph)
// for a fully-specified LXMF message, matching the Python parity format.
func TestConversationWidgetRenderHeaderParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.OwnHash = bytes.Repeat([]byte{0x11}, 32)
	cw.TimeFormat = "%Y-%m-%d %H:%M:%S"

	ts := time.Unix(1699996400, 0).UTC()
	cw.SetMessages([]ConversationMessage{{
		Content:            "Hello world",
		Timestamp:          ts,
		State:              lxmfStateSent,
		Method:             lxmfMethodPropagated,
		SourceHash:         cw.OwnHash,
		TransportEncrypted: true,
		Title:              "My Subject",
	}})

	text := cw.messageList.GetText(false)
	// Prefix "↑ → " (sent + arrow_r) and the deterministic strftime timestamp +
	// encryption glyph must appear; relative_time is now-dependent so not asserted.
	for _, want := range []string{"↑ → ", "2023-11-14 21:13:20 ⚿", "| My Subject", "  Hello world"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered text missing %q\ngot: %s", want, text)
		}
	}
}

func TestConversationWidgetClearHistoryDialogConfirm(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	now := time.Now()
	cw.SetMessages([]ConversationMessage{
		{Content: "Hello", Timestamp: now, IsSent: true},
	})

	var historyCleared bool
	cw.OnClearHistory = func() { historyCleared = true }

	cw.ClearHistoryDialog()
	if !cw.DialogOpen() {
		t.Error("ClearHistoryDialog should set dialog open")
	}

	cw.ConfirmClearHistory()
	if historyCleared != true {
		t.Error("ConfirmClearHistory should call OnClearHistory")
	}
	if cw.DialogOpen() {
		t.Error("ConfirmClearHistory should close dialog")
	}
}

func TestConversationWidgetClearHistoryDialogDismiss(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	cw.ClearHistoryDialog()
	if !cw.DialogOpen() {
		t.Error("ClearHistoryDialog should set dialog open")
	}

	cw.DismissClearHistoryDialog()
	if cw.DialogOpen() {
		t.Error("DismissClearHistoryDialog should close dialog")
	}
}

func TestConversationWidgetPaperMessageDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	cw.PaperMessageDialog()
	if !cw.DialogOpen() {
		t.Error("PaperMessageDialog should set dialog open")
	}
}

func TestConversationWidgetPaperMessageDialogActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action func(cw *ConversationWidget)
	}{
		{"PrintQR", func(cw *ConversationWidget) { cw.PaperMessagePrintQR() }},
		{"SaveQR", func(cw *ConversationWidget) { cw.PaperMessageSaveQR() }},
		{"SaveURI", func(cw *ConversationWidget) { cw.PaperMessageSaveURI() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			cw := NewConversationWidget(app, "aabb1122")

			var actionFired string
			cw.OnPaperMessage = func(action string) { actionFired = action }

			cw.PaperMessageDialog()
			tt.action(cw)

			if cw.DialogOpen() {
				t.Error("action should close dialog")
			}
			if actionFired != tt.name {
				t.Errorf("OnPaperMessage = %q, want %q", actionFired, tt.name)
			}
		})
	}
}

func TestConversationWidgetPaperMessageDialogCancel(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	cw.PaperMessageDialog()
	cw.DismissPaperMessageDialog()

	if cw.DialogOpen() {
		t.Error("DismissPaperMessageDialog should close dialog")
	}
}

func TestConversationWidgetSaveAttachmentsNoAttachments(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	cw.SaveAttachmentsDialog([]AttachmentRef{})
	if !cw.DialogOpen() {
		t.Error("SaveAttachmentsDialog should set dialog open")
	}
}

func TestConversationWidgetSaveAttachmentsWithItems(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	attachments := []AttachmentRef{
		{Name: "photo.jpg", Type: "file"},
		{Name: "doc.pdf", Type: "file"},
	}

	cw.SaveAttachmentsDialog(attachments)
	if !cw.DialogOpen() {
		t.Error("SaveAttachmentsDialog should set dialog open")
	}

	var saved []string
	cw.OnSaveAttachments = func(names []string) { saved = names }

	cw.ConfirmSaveAttachments([]string{"photo.jpg"})
	if cw.DialogOpen() {
		t.Error("ConfirmSaveAttachments should close dialog")
	}
	if len(saved) != 1 || saved[0] != "photo.jpg" {
		t.Errorf("OnSaveAttachments = %v, want [photo.jpg]", saved)
	}
}

func TestConversationWidgetDismissSaveAttachments(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	cw.SaveAttachmentsDialog([]AttachmentRef{{Name: "file.txt", Type: "file"}})
	cw.DismissSaveAttachmentsDialog()

	if cw.DialogOpen() {
		t.Error("DismissSaveAttachmentsDialog should close dialog")
	}
}
