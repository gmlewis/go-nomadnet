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
		t.Errorf("SetMessages: got %v messages, want 3", len(cw.messages))
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
	cw.OnSend = func(content, title string, _ []string) {
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
	cw.OnSend = func(content, title string, _ []string) { sent = true }
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
				t.Errorf("key %v was not consumed", tt.name)
			}
			if tt.want == "" {
				if len(fired) != 0 {
					t.Errorf("key %v should not fire callbacks, fired %v", tt.name, fired)
				}
			} else {
				if len(fired) != 1 || fired[0] != tt.want {
					t.Errorf("key %v fired %v, want [%v]", tt.name, fired, tt.want)
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
			t.Errorf("rendered text missing %q\ngot: %v", want, text)
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

// TestConversationWidgetPaperMessageDialogActions pins the paper-message
// action handlers parity (Python print_paper_message_qr / save_paper_message_qr
// / save_paper_message_uri, Conversations.py:2474-2503): each handler reads the
// editor content+title, fires OnPaperMessage(action, content, title) returning
// (path, ok), and on success clears the editor (SaveQR/SaveURI also fire
// OnPaperMessageSaved with the path; PrintQR does not), while failure fires
// OnPaperMessageFailed and leaves the editor intact.
func TestConversationWidgetPaperMessageDialogActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action func(cw *ConversationWidget)
		saved  bool // whether OnPaperMessageSaved should fire (save modes only)
	}{
		{"PrintQR", func(cw *ConversationWidget) { cw.PaperMessagePrintQR() }, false},
		{"SaveQR", func(cw *ConversationWidget) { cw.PaperMessageSaveQR() }, true},
		{"SaveURI", func(cw *ConversationWidget) { cw.PaperMessageSaveURI() }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			cw := NewConversationWidget(app, "aabb1122")
			cw.editor.SetText("paper body")
			cw.titleEditor.SetText("paper title")

			var gotAction, gotContent, gotTitle string
			cw.OnPaperMessage = func(action, content, title string) (string, bool) {
				gotAction, gotContent, gotTitle = action, content, title
				return "/dl/saved.lxm", true
			}
			var savedPath string
			cw.OnPaperMessageSaved = func(path string) { savedPath = path }
			var failedFired bool
			cw.OnPaperMessageFailed = func() { failedFired = true }

			cw.PaperMessageDialog()
			tt.action(cw)

			if cw.DialogOpen() {
				t.Error("action should close dialog")
			}
			if gotAction != tt.name {
				t.Errorf("OnPaperMessage action = %q, want %q", gotAction, tt.name)
			}
			if gotContent != "paper body" {
				t.Errorf("OnPaperMessage content = %q, want %q", gotContent, "paper body")
			}
			if gotTitle != "paper title" {
				t.Errorf("OnPaperMessage title = %q, want %q", gotTitle, "paper title")
			}
			if failedFired {
				t.Error("success should not fire OnPaperMessageFailed")
			}
			if cw.editor.GetText() != "" {
				t.Error("success should clear the editor")
			}
			if tt.saved {
				if savedPath != "/dl/saved.lxm" {
					t.Errorf("OnPaperMessageSaved = %q, want %q", savedPath, "/dl/saved.lxm")
				}
			} else if savedPath != "" {
				t.Error("PrintQR success should not fire OnPaperMessageSaved")
			}
		})
	}
}

// TestConversationWidgetPaperMessageFailure pins the failure branch: an !ok
// result fires OnPaperMessageFailed and leaves the editor content intact
// (Python paper_message_failed, Conversations.py:2480-2481,2491-2492,2502-2503).
func TestConversationWidgetPaperMessageFailure(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.editor.SetText("keep me")

	cw.OnPaperMessage = func(action, content, title string) (string, bool) {
		return "", false
	}
	var failedFired bool
	var savedFired bool
	cw.OnPaperMessageFailed = func() { failedFired = true }
	cw.OnPaperMessageSaved = func(path string) { savedFired = true }

	cw.PaperMessageSaveQR()

	if !failedFired {
		t.Error("failure should fire OnPaperMessageFailed")
	}
	if savedFired {
		t.Error("failure should not fire OnPaperMessageSaved")
	}
	if cw.editor.GetText() != "keep me" {
		t.Error("failure should leave the editor content intact")
	}
}

// TestConversationWidgetPaperMessageEmptyContent pins the Python guard
// `if not content == ""`: an empty editor short-circuits the action without
// firing OnPaperMessage (Conversations.py:2477,2486,2497).
func TestConversationWidgetPaperMessageEmptyContent(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	called := false
	cw.OnPaperMessage = func(action, content, title string) (string, bool) {
		called = true
		return "", true
	}
	cw.PaperMessagePrintQR()
	if called {
		t.Error("empty editor should short-circuit without firing OnPaperMessage")
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

// TestConversationWidgetCtrlSavesAttachments pins the C-s keypress parity:
// Python's ConversationWidget.keypress (Conversations.py:2235-2236) maps
// "ctrl s" to save_focused_attachments(), which is DISTINCT from "ctrl a"
// (attach_file). The Go handler must fire the save-attachments entry callback
// (collecting attachment refs from messages, sorted by sort_timestamp desc —
// Python _collect_attachment_refs at Conversations.py:2300), and must NOT fire
// OnAttach.
// TestConversationWidgetCtrlPCtrlFParity pins that C-p and C-f are handled at
// the frame level (matching Python's MessageEdit.keypress at
// Conversations.py:1809-1814, where ctrl p → paper_message and ctrl f →
// attach_file). Without the frame capture these fall through to the
// ReadlineEdit's readline bindings (C-p=prev-history, C-f=forward-char) and
// never reach the conversation actions.
func TestConversationWidgetCtrlPCtrlFParity(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	var paperFired bool
	cw.OnPaperMessage = func(action, content, title string) (string, bool) {
		paperFired = true
		return "", true
	}

	var attachFired bool
	cw.OnAttach = func() { attachFired = true }

	// C-p opens the paper-message dialog (paper_message).
	if cw.handleInput(tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModNone)) != nil {
		t.Error("C-p was not consumed")
	}
	if !cw.DialogOpen() {
		t.Error("C-p should open the paper message dialog")
	}
	cw.DismissPaperMessageDialog()

	// C-f triggers attach_file (same as C-a in Python's frame keypress).
	if cw.handleInput(tcell.NewEventKey(tcell.KeyCtrlF, 0, tcell.ModNone)) != nil {
		t.Error("C-f was not consumed")
	}
	if !attachFired {
		t.Error("C-f should fire OnAttach (attach_file)")
	}
	// C-p must not have fired the paper callback just from opening the dialog
	// (the callback fires on a chosen action, not on dialog open).
	if paperFired {
		t.Error("opening the paper dialog should not fire OnPaperMessage")
	}
}

func TestConversationWidgetCtrlSavesAttachments(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	base := time.Unix(1700000000, 0).UTC()
	cw.SetMessages([]ConversationMessage{
		{Content: "old", Timestamp: base, IsSent: true, HasAttach: true,
			AttachmentTypes: []string{"file"}, AttachmentNames: []string{"old.txt"}},
		{Content: "no attach here", Timestamp: base.Add(time.Minute), IsSent: false},
		{Content: "newest", Timestamp: base.Add(2 * time.Minute), IsSent: true, HasAttach: true,
			AttachmentTypes: []string{"file", "image"}, AttachmentNames: []string{"new.pdf", "pic.png"}},
	})

	attachFired := false
	cw.OnAttach = func() { attachFired = true }

	var gotRefs []AttachmentRef
	cw.OnSaveFocusedAttachments = func(refs []AttachmentRef) { gotRefs = refs }

	result := cw.handleInput(tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModNone))
	if result != nil {
		t.Errorf("C-s was not consumed (returned %v)", result)
	}
	if attachFired {
		t.Error("C-s must NOT fire OnAttach (that is C-a's behavior)")
	}
	if !cw.DialogOpen() {
		t.Error("C-s should mark the dialog open (save_focused_attachments sets dialog_active)")
	}
	// Sorted by timestamp desc: newest message's two attachments first, then
	// old. FieldIndex is the per-message attachment index (0..).
	want := []AttachmentRef{
		{Name: "new.pdf", Type: "file", FieldIndex: 0},
		{Name: "pic.png", Type: "image", FieldIndex: 1},
		{Name: "old.txt", Type: "file", FieldIndex: 0},
	}
	if len(gotRefs) != len(want) {
		t.Fatalf("got %v refs %v, want %v %v", len(gotRefs), gotRefs, len(want), want)
	}
	for i, w := range want {
		if gotRefs[i].Name != w.Name || gotRefs[i].Type != w.Type || gotRefs[i].FieldIndex != w.FieldIndex {
			t.Errorf("ref[%v] = %+v, want %+v", i, gotRefs[i], w)
		}
	}
}

// TestConversationWidgetCtrlSNoAttachments still fires the entry callback with
// an empty ref list (Python save_focused_attachments shows a "No attachments"
// dialog rather than silently no-op'ing).
func TestConversationWidgetCtrlSNoAttachments(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")
	cw.SetMessages([]ConversationMessage{
		{Content: "nothing attached", Timestamp: time.Now(), IsSent: true},
	})

	called := false
	cw.OnSaveFocusedAttachments = func(refs []AttachmentRef) {
		called = true
		if len(refs) != 0 {
			t.Errorf("empty conversation refs = %v, want empty", refs)
		}
	}

	if cw.handleInput(tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModNone)) != nil {
		t.Error("C-s was not consumed")
	}
	if !called {
		t.Error("C-s with no attachments should still fire OnSaveFocusedAttachments (empty)")
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

	var saved []AttachmentRef
	cw.OnSaveAttachments = func(refs []AttachmentRef) { saved = refs }

	cw.ConfirmSaveAttachments([]AttachmentRef{attachments[0]})
	if cw.DialogOpen() {
		t.Error("ConfirmSaveAttachments should close dialog")
	}
	if len(saved) != 1 || saved[0].Name != "photo.jpg" {
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

// TestConversationWidgetFooterAttachmentIndicator pins the pending-attachments
// footer indicator parity: Python's _build_footer (Conversations.py:2160-2177)
// shows "{file-glyph} N file(s): {basenames joined by ', '}" above the editor
// when pending_attachments is non-empty, and just the editor otherwise.
func TestConversationWidgetFooterAttachmentIndicator(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "aabb1122")

	// No pending attachments → no indicator row.
	cw.buildFooter()
	if cw.footerIndicatorText() != "" {
		t.Errorf("empty pending: indicator = %q, want empty", cw.footerIndicatorText())
	}

	// Stage two attachments → indicator lists basenames, comma-separated.
	cw.pendingAttachments = []string{"/tmp/notes.txt", "/tmp/pic.png"}
	cw.buildFooter()
	g := cw.glyphs()
	want := g["file"] + " 2 file(s): notes.txt, pic.png"
	if got := cw.footerIndicatorText(); got != want {
		t.Errorf("indicator = %q, want %q", got, want)
	}

	// ClearEditor clears pending attachments and rebuilds the footer.
	cw.ClearEditor()
	if cw.footerIndicatorText() != "" {
		t.Errorf("after ClearEditor: indicator = %q, want empty", cw.footerIndicatorText())
	}

	// ConfirmAttachFile stages paths and rebuilds the footer.
	cw.ConfirmAttachFile([]string{"/tmp/a.bin"})
	if got := cw.footerIndicatorText(); got != g["file"]+" 1 file(s): a.bin" {
		t.Errorf("after ConfirmAttachFile: indicator = %q, want %q", got, g["file"]+" 1 file(s): a.bin")
	}
}
