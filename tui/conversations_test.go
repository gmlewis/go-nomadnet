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
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestNewConversationsDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	convs := []ConversationInfo{
		{DisplayName: "Alice", TrustLevel: "trusted", Unread: true, LastTime: time.Now()},
		{DisplayName: "Bob", TrustLevel: "unknown", Unread: false, LastTime: time.Now().Add(-time.Hour)},
	}

	cd := NewConversationsDisplay(app, convs)
	if cd == nil {
		t.Fatal("NewConversationsDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestConversationsDisplayWidgetType(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	_, ok := cd.Widget().(*tview.Flex)
	if !ok {
		t.Error("Widget is not a Flex")
	}
}

func TestConversationsDisplayEmpty(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, []ConversationInfo{})

	if cd == nil {
		t.Fatal("NewConversationsDisplay with empty list returned nil")
	}
}

func TestNewComposeDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewComposeDisplay(app)

	if cd == nil {
		t.Fatal("NewComposeDisplay returned nil")
	}
	if cd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestComposeDisplayGetSetText(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewComposeDisplay(app)

	cd.editor.SetText("Hello World")
	if cd.GetText() != "Hello World" {
		t.Errorf("GetText() = %q, want %q", cd.GetText(), "Hello World")
	}

	cd.title.SetText("Alice")
	if cd.GetTitle() != "Alice" {
		t.Errorf("GetTitle() = %q, want %q", cd.GetTitle(), "Alice")
	}
}

func TestComposeDisplayClear(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewComposeDisplay(app)

	cd.editor.SetText("test")
	cd.title.SetText("test")
	cd.Clear()

	if cd.GetText() != "" {
		t.Errorf("GetText() after clear = %q, want empty", cd.GetText())
	}
	if cd.GetTitle() != "" {
		t.Errorf("GetTitle() after clear = %q, want empty", cd.GetTitle())
	}
}

func TestNewMessageViewDisplay(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	mvd := NewMessageViewDisplay(app)

	if mvd == nil {
		t.Fatal("NewMessageViewDisplay returned nil")
	}
	if mvd.Widget() == nil {
		t.Error("Widget() returned nil")
	}
}

func TestMessageViewShowMessage(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	mvd := NewMessageViewDisplay(app)

	msg := MessageInfo{
		Title:      "Test",
		Content:    "Hello World",
		Sender:     "Alice",
		Timestamp:  "2024-01-01",
		TrustLevel: "trusted",
	}

	// Should not panic
	mvd.ShowMessage(msg)
}

func TestMessageViewClear(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	mvd := NewMessageViewDisplay(app)

	mvd.ShowMessage(MessageInfo{Content: "test"})
	mvd.Clear()
	// Should not panic
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input time.Time
		want  string
	}{
		{time.Now(), "just now"},
		{time.Now().Add(-30 * time.Second), "just now"},
		{time.Now().Add(-5 * time.Minute), "5m ago"},
		{time.Now().Add(-3 * time.Hour), "3h ago"},
		{time.Now().Add(-25 * time.Hour), "yesterday"},
		{time.Now().Add(-3 * 24 * time.Hour), "3d ago"},
	}

	for _, tt := range tests {
		got := relativeTime(tt.input)
		if got != tt.want {
			t.Errorf("relativeTime(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLooksLikeMicron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"plain text", false},
		{">>Heading", true},
		{"```code```", true},
		{"`!bold`", true},
		{"Hello World", false},
	}

	for _, tt := range tests {
		got := looksLikeMicron(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeMicron(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRenderMicronAsText(t *testing.T) {
	t.Parallel()

	result := renderMicronAsText("plain text")
	if result != "plain text" {
		t.Errorf("renderMicronAsText plain = %q, want %q", result, "plain text")
	}
}

func TestConversationsDisplayKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	// Track which callbacks fire
	var fired []string
	cd.OnEditPeerInfo = func() { fired = append(fired, "edit_peer") }
	cd.OnDeleteConv = func() { fired = append(fired, "delete") }
	cd.OnNewConv = func() { fired = append(fired, "new") }
	cd.OnIngestURI = func() { fired = append(fired, "ingest") }
	cd.OnSync = func() { fired = append(fired, "sync") }
	cd.OnToggleFullscreen = func() { fired = append(fired, "fullscreen") }
	cd.OnToggleSort = func() { fired = append(fired, "sort") }
	cd.OnShowQR = func() { fired = append(fired, "qr") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-e", tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "edit_peer"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "delete"},
		{"ctrl-n", tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModNone), "new"},
		{"ctrl-u", tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone), "ingest"},
		{"ctrl-r", tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone), "sync"},
		{"ctrl-g", tcell.NewEventKey(tcell.KeyCtrlG, 0, tcell.ModNone), "fullscreen"},
		{"ctrl-o", tcell.NewEventKey(tcell.KeyCtrlO, 0, tcell.ModNone), "sort"},
		{"ctrl-p", tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModNone), "qr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := cd.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %s was not consumed (returned non-nil)", tt.name)
			}
			if len(fired) != 1 || fired[0] != tt.want {
				t.Errorf("key %s fired %v, want [%s]", tt.name, fired, tt.want)
			}
		})
	}
}

func TestConversationsDisplayUnhandledKeys(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	// Unhandled keys should pass through
	event := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	result := cd.handleInput(event)
	if result != event {
		t.Error("Unhandled key should pass through")
	}
}

func TestConversationsDisplayEditSelectedInDirectory(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	convs := []ConversationInfo{
		{SourceHash: "aabb1122", DisplayName: "Alice", TrustLevel: "trusted"},
	}
	cd := NewConversationsDisplay(app, convs)

	var edited PeerInfoEntry
	cd.OnEditPeerInfo = func() { edited = cd.EditSelectedInDirectory() }

	cd.list.SetCurrentItem(0)
	cd.OnEditPeerInfo()

	if edited.SourceHash != "aabb1122" {
		t.Errorf("SourceHash = %q, want %q", edited.SourceHash, "aabb1122")
	}
	if edited.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want %q", edited.DisplayName, "Alice")
	}
}

func TestConversationsDisplayEditSelectedNoSelection(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	result := cd.EditSelectedInDirectory()
	if result.SourceHash != "" {
		t.Errorf("with no selection, SourceHash should be empty, got %q", result.SourceHash)
	}
}

func TestPeerInfoEntryTrustLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		trust    string
		expected string
	}{
		{"trusted", TrustTrusted},
		{"untrusted", TrustUntrusted},
		{"unknown", TrustUnknown},
		{"blocked", TrustUntrusted},
		{"", TrustUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.trust, func(t *testing.T) {
			t.Parallel()

			entry := PeerInfoEntry{TrustLevel: tt.trust}
			if entry.TrustLevelValue() != tt.expected {
				t.Errorf("TrustLevelValue() = %q, want %q", entry.TrustLevelValue(), tt.expected)
			}
		})
	}
}

func TestConversationsDisplayIngestLXMURI(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenIngestURIDialog()
	if !cd.DialogOpen() {
		t.Error("OpenIngestURIDialog should set dialog open")
	}

	cd.ConfirmIngestURI("lxm://abc123def")
	if cd.DialogOpen() {
		t.Error("ConfirmIngestURI should close dialog")
	}
	if cd.IngestURIDialogValue() != "lxm://abc123def" {
		t.Errorf("IngestURIDialogValue = %q, want %q", cd.IngestURIDialogValue(), "lxm://abc123def")
	}
}

func TestConversationsDisplayIngestURIDismiss(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenIngestURIDialog()
	cd.DismissIngestURIDialog()

	if cd.DialogOpen() {
		t.Error("DismissIngestURIDialog should close dialog")
	}
}

func TestConversationsDisplaySyncConversations(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenSyncDialog()
	if !cd.DialogOpen() {
		t.Error("OpenSyncDialog should set dialog open")
	}
}

func TestConversationsDisplaySyncConversationsRequestSync(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	var syncRequested bool
	var syncLimit int
	cd.OnSyncRequested = func(limit int) { syncRequested = true; syncLimit = limit }

	cd.OpenSyncDialog()
	cd.RequestSync(0)

	if syncRequested != true {
		t.Error("RequestSync should fire OnSyncRequested")
	}
	if syncLimit != 0 {
		t.Errorf("syncLimit = %d, want 0 (unlimited)", syncLimit)
	}
}

func TestConversationsDisplaySyncConversationsWithLimit(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	var syncLimit int
	cd.OnSyncRequested = func(limit int) { syncLimit = limit }

	cd.OpenSyncDialog()
	cd.RequestSync(5)

	if syncLimit != 5 {
		t.Errorf("syncLimit = %d, want 5", syncLimit)
	}
}

func TestConversationsDisplaySyncConversationsDismiss(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	cd.OpenSyncDialog()
	cd.DismissSyncDialog()

	if cd.DialogOpen() {
		t.Error("DismissSyncDialog should close dialog")
	}
}

func TestSyncStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		progress int
		running  bool
		node     string
		wantBar  string
	}{
		{"idle", 0, false, "", ""},
		{"syncing 50%", 50, true, "Node1", "[==========          ] 50%"},
		{"syncing 100%", 100, true, "", "[====================] 100%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ss := &SyncStatus{
				SyncProgress: tt.progress,
				SyncRunning:  tt.running,
				NodeLabel:    tt.node,
			}
			bar := ss.FormatSyncProgress()
			if bar != tt.wantBar {
				t.Errorf("FormatSyncProgress() = %q, want %q", bar, tt.wantBar)
			}
		})
	}
}

func TestSyncStatusLine(t *testing.T) {
	t.Parallel()

	ss := &SyncStatus{HasSynced: false}
	if got := ss.FormatStatusLine(); got != "Last sync: never" {
		t.Errorf("FormatStatusLine() = %q, want %q", got, "Last sync: never")
	}
}

func TestConversationsDisplayShowBlocked(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	if cd.ShowBlocked() {
		t.Error("ShowBlocked should default to false")
	}

	cd.SetShowBlocked(true)
	if !cd.ShowBlocked() {
		t.Error("SetShowBlocked(true) should make ShowBlocked return true")
	}

	cd.SetShowBlocked(false)
	if cd.ShowBlocked() {
		t.Error("SetShowBlocked(false) should make ShowBlocked return false")
	}
}

func TestConversationsDisplayShowBlockedFiltersList(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	convs := []ConversationInfo{
		{SourceHash: "aaaa", DisplayName: "Alice", TrustLevel: "trusted"},
		{SourceHash: "bbbb", DisplayName: "Eve", TrustLevel: "blocked"},
	}
	cd := NewConversationsDisplay(app, convs)

	cd.showTrusted = false

	cd.SetShowBlocked(false)
	filtered := FilterConversationsWithBlocked(cd.conversations, "untrusted", cd.ShowBlocked())
	if len(filtered) != 0 {
		t.Errorf("without blocked, got %d untrusted, want 0", len(filtered))
	}

	cd.SetShowBlocked(true)
	filtered = FilterConversationsWithBlocked(cd.conversations, "untrusted", cd.ShowBlocked())
	if len(filtered) != 1 {
		t.Errorf("with blocked, got %d untrusted, want 1", len(filtered))
	}
}

func TestConversationsDisplayBlockedRowLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		displayName string
		sourceHash  string
		wantPrefix  string
	}{
		{"named peer", "Eve", "aabb1122", "× [blocked] Eve"},
		{"unnamed peer", "", "aabb1122", "× [blocked]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			label := BlockedRowLabel(tt.displayName, tt.sourceHash)
			if !strings.HasPrefix(label, tt.wantPrefix) {
				t.Errorf("BlockedRowLabel(%q, %q) = %q, want prefix %q", tt.displayName, tt.sourceHash, label, tt.wantPrefix)
			}
		})
	}
}

func TestPaperMessageDialog(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	var printFired, saveQRFired, saveURIFired bool
	cd.PaperMessageDialog(
		func() { printFired = true },
		func() { saveQRFired = true },
		func() { saveURIFired = true },
	)

	if !cd.dialogOpen {
		t.Error("dialog should be open after PaperMessageDialog")
	}

	// Verify callbacks are set (dialog display requires running app)
	_ = printFired
	_ = saveQRFired
	_ = saveURIFired
}

func TestPaperMessageFailed(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	cd.PaperMessageFailed()
	if !cd.dialogOpen {
		t.Error("dialog should be open after PaperMessageFailed")
	}
}

func TestAttachFileDialog(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	var selectedPath string
	cd.AttachFileDialog("/tmp/", func(path string) {
		selectedPath = path
	})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = selectedPath
}

func TestSaveAttachmentsDialog(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	attachments := []string{"file1.pdf", "image.png", "doc.txt"}
	cd.SaveAttachmentsDialog(attachments, func(selected []string) {})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
}

func TestShowPeerInfoDialog(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	entry := PeerInfoEntry{
		SourceHash:        "aabb112233445566",
		DisplayName:       "TestPeer",
		TrustLevel:        TrustTrusted,
		PreferredDelivery: "direct",
		Pinned:            true,
		Notes:             "test notes",
	}

	var saved bool
	cd.ShowPeerInfoDialog(entry, func(e PeerInfoEntry) {
		saved = true
		if e.DisplayName != "TestPeer" {
			t.Errorf("DisplayName = %q, want TestPeer", e.DisplayName)
		}
	})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = saved
}

func TestPeerInfoEntryTrustLevelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{TrustTrusted, TrustTrusted},
		{TrustUntrusted, TrustUntrusted},
		{TrustUnknown, TrustUnknown},
		{"blocked", TrustUntrusted},
		{"other", TrustUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			e := PeerInfoEntry{TrustLevel: tt.input}
			got := e.TrustLevelValue()
			if got != tt.want {
				t.Errorf("TrustLevelValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShowSyncDialog(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
	cd := NewConversationsDisplay(app, nil)

	var result SyncDialogResult
	cd.ShowSyncDialog("aabb112233445566", nil, 0.5, func(r SyncDialogResult) {
		result = r
	})

	if !cd.dialogOpen {
		t.Error("dialog should be open")
	}
	_ = result
}

func TestSyncDialogResult(t *testing.T) {
	t.Parallel()

	result := SyncDialogResult{
		Mode:   SyncLimited,
		Limit:  10,
		Action: "sync",
	}

	if result.Mode != SyncLimited {
		t.Errorf("Mode = %v, want SyncLimited", result.Mode)
	}
	if result.Limit != 10 {
		t.Errorf("Limit = %d, want 10", result.Limit)
	}
	if result.Action != "sync" {
		t.Errorf("Action = %q, want sync", result.Action)
	}
}

func TestSyncModeConstants(t *testing.T) {
	t.Parallel()

	if SyncAll != 0 {
		t.Errorf("SyncAll = %d, want 0", SyncAll)
	}
	if SyncLimited != 1 {
		t.Errorf("SyncLimited = %d, want 1", SyncLimited)
	}
}
