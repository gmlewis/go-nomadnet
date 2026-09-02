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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dialogMessageText joins the dialog content's text-widget rows so tests can
// assert rendered dialog messages.
func dialogMessageText(t *testing.T, dialog *DialogLineBox) string {
	t.Helper()
	flex, ok := dialog.Content().(*tview.Flex)
	if !ok {
		t.Fatalf("dialog content is %T, want *tview.Flex", dialog.Content())
	}
	var sb strings.Builder
	for i := 0; i < flex.GetItemCount(); i++ {
		switch v := flex.GetItem(i).(type) {
		case *urwidCenterText:
			sb.WriteString(v.text)
			sb.WriteString("\n")
		case *urwidLeftText:
			sb.WriteString(v.text)
			sb.WriteString("\n")
		case *centeredText:
			sb.WriteString(v.GetText())
			sb.WriteString("\n")
		case *tview.TextView:
			sb.WriteString(v.GetText(true))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// detailDialogButton finds the named button in the open detail-slot dialog.
func detailDialogButton(t *testing.T, cd *ConversationsDisplay, label string) *UrwidButton {
	t.Helper()
	dialog := cd.detailSlotOverlay.Dialog()
	flex, ok := dialog.Content().(*tview.Flex)
	if !ok {
		t.Fatalf("dialog content is %T, want *tview.Flex", dialog.Content())
	}
	for i := 0; i < flex.GetItemCount(); i++ {
		if row, ok := flex.GetItem(i).(*urwidColumns); ok {
			for _, child := range row.children {
				if btn, ok := child.(*UrwidButton); ok && btn.Label() == label {
					return btn
				}
			}
		}
	}
	t.Fatalf("dialog has no %q button", label)
	return nil
}

// TestConversationsDisplayCtrlUPurgeFailed pins the Ctrl-U path: activating
// Ctrl-U inside the open conversation fires the display-level OnPurgeFailed
// with the open conversation's source hash (Python
// ConversationWidget.keypress "ctrl u" → conversation.purge_failed +
// conversation_changed, Conversations.py:2226-2228).
func TestConversationsDisplayCtrlUPurgeFailed(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, []ConversationInfo{
		{SourceHash: "aabb1122", DisplayName: "Peer", TrustLevel: "unknown"},
	})

	var purged []string
	cd.OnPurgeFailed = func(sourceHash string) { purged = append(purged, sourceHash) }
	cd.DisplayConversation("aabb1122")

	if cd.currentWidget == nil {
		t.Fatal("no conversation widget displayed")
	}
	// Python's "ctrl u" purge lives on the frame keypress, which only sees the
	// key when the BODY (message list) is focused — with the composer focused
	// the editor consumes Ctrl-U as readline's unix-line-discard. Focus the
	// message list like the user flow does.
	cd.app.SetFocus(cd.currentWidget.messageList)
	cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone))

	if len(purged) != 1 || purged[0] != "aabb1122" {
		t.Errorf("OnPurgeFailed = %v, want [aabb1122]", purged)
	}
}

// TestConversationsDisplayCtrlXClearHistoryConfirm pins the Ctrl-X path
// (Python ConversationWidget.keypress "ctrl x" → clear_history_dialog,
// Conversations.py:2232-2233 + 2122-2159): the "?" confirm dialog overlays the
// conversation body at Python's fixed width 34 with the centered
// "Clear conversation history" message; Yes fires the display-level
// OnClearHistory with the open conversation's source hash and closes the
// overlay; No (dismiss) fires nothing.
func TestConversationsDisplayCtrlXClearHistoryConfirm(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, []ConversationInfo{
		{SourceHash: "aabb1122", DisplayName: "Peer", TrustLevel: "unknown"},
	})

	var cleared []string
	cd.OnClearHistory = func(sourceHash string) { cleared = append(cleared, sourceHash) }
	cd.DisplayConversation("aabb1122")

	if cd.currentWidget == nil {
		t.Fatal("no conversation widget displayed")
	}
	// Ctrl-X reaches the frame keypress with the body focused (the composer
	// consumes its own keys).
	cd.app.SetFocus(cd.currentWidget.messageList)
	cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone))

	if cd.detailSlotOverlay == nil {
		t.Fatal("Ctrl-X did not open the clear-history confirm overlay")
	}
	dialog := cd.detailSlotOverlay.Dialog()
	if title := dialog.GetTitle(); title != "?" {
		t.Errorf("dialog title = %q, want %q", title, "?")
	}
	msg := dialogMessageText(t, dialog)
	if !strings.Contains(msg, "Clear conversation history") {
		t.Errorf("dialog message %q missing %q", msg, "Clear conversation history")
	}

	// Yes fires the clear callback and closes the overlay.
	yes := detailDialogButton(t, cd, "Yes")
	yes.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if len(cleared) != 1 || cleared[0] != "aabb1122" {
		t.Errorf("OnClearHistory = %v, want [aabb1122]", cleared)
	}
	if cd.detailSlotOverlay != nil {
		t.Error("Yes should close the confirm overlay")
	}

	// No (dismiss) fires nothing.
	cd.currentWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone))
	no := detailDialogButton(t, cd, "No")
	no.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})
	if len(cleared) != 1 {
		t.Errorf("No fired OnClearHistory: cleared = %v", cleared)
	}
	if cd.detailSlotOverlay != nil {
		t.Error("No should close the confirm overlay")
	}
}

// TestConversationsDisplaySyncRequestedAndTimeFormat pins the display-level
// hooks the wiring layer supplies: RequestSync forwards to OnSyncRequested
// with the limit, and the OnTimeFormat provider feeds the open conversation
// widget's strftime format (Python app.time_format).
func TestConversationsDisplaySyncRequestedAndTimeFormat(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var syncLimit = -1
	cd.OnSyncRequested = func(limit int) { syncLimit = limit }
	cd.RequestSync(42)
	if syncLimit != 42 {
		t.Errorf("RequestSync(42) → OnSyncRequested limit = %v, want 42", syncLimit)
	}

	cd.OnTimeFormat = func() string { return "%H:%M" }
	cd.DisplayConversation("aabb1122")
	if cd.currentWidget == nil {
		t.Fatal("no conversation widget displayed")
	}
	if got := cd.currentWidget.timeFormat(); got != "%H:%M" {
		t.Errorf("widget timeFormat = %q, want %%H:%%M from OnTimeFormat", got)
	}
}
