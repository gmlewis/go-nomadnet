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

// newSplitDialogCD opens a connected hub's room and returns the display with a
// room composer ready for a draft.
func newSplitDialogCD(t *testing.T, limit int) *ChannelsDisplay {
	t.Helper()
	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	cd.SetHubs([]HubView{
		fakeHub{name: "RNS Community", status: hubStatusConnected, joined: []string{"general"}, maxMsgBodyBytes: limit},
	})
	cd.ShowRoom(0, "general", nil)
	return cd
}

// dialogLeftTexts returns the dialog's urwid-left-text rows in layout order,
// which is the order Python's Pile stacks them.
func dialogLeftTexts(t *testing.T, cd *ChannelsDisplay) []*urwidLeftText {
	t.Helper()
	var rows []*urwidLeftText
	for _, item := range dialogItems(t, cd) {
		if lt, ok := item.(*urwidLeftText); ok {
			rows = append(rows, lt)
		}
	}
	return rows
}

// TestChannelsCtrlDOverLimitOpensSplitDialog is the regression test for the
// 2026-09-14 glenn-kamrui report: C-d on a draft over the per-message limit
// must NOT be a silent no-op. Python's RoomWidget.send_message hands the draft
// to _open_split_dialog (Channels.py:879-889), which raises the "Message Too
// Long" dialog carrying the byte counts, the split plan, the part-1 preview and
// Send Split / Cancel. Before this fix the gate's OnSplitDialog callback was
// unwired, so the key did nothing at all: no send, no dialog, no notice.
func TestChannelsCtrlDOverLimitOpensSplitDialog(t *testing.T) {
	t.Parallel()

	cd := newSplitDialogCD(t, defaultMaxMsgBytes)

	var sent []string
	cd.OnSendMessage = func(text string) { sent = append(sent, text) }
	cd.roomWidget.editor.SetText(rrcCommunityDraft)

	cd.roomWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	if cd.dialogOverlay == nil {
		t.Fatal("C-d on an over-limit draft must open the split dialog, not silently drop the message")
	}
	dialog := cd.dialogOverlay.Dialog()
	if title := dialog.GetTitle(); title != "Message Too Long" {
		t.Errorf("dialog title = %q, want %q", title, "Message Too Long")
	}
	if len(sent) != 0 {
		t.Errorf("sent %v messages, want none while the split dialog is open", len(sent))
	}
	if got := cd.roomWidget.editor.GetText(); got != rrcCommunityDraft {
		t.Error("the draft must be kept while the split dialog is open")
	}

	// Python's Pile: blank, message/limit, blank, split plan, preview label,
	// preview, blank, error row, button row — 10 rows total.
	items := dialogItems(t, cd)
	if len(items) != 10 {
		t.Fatalf("dialog rows = %v, want 10 (Python's Pile)", len(items))
	}
	info := ComputeSplitDialog(rrcCommunityDraft, defaultMaxMsgBytes)
	rows := dialogLeftTexts(t, cd)
	var texts []string
	for _, r := range rows {
		texts = append(texts, r.text)
	}
	want := []string{
		"",
		"  Message is 362 bytes.",
		"  Hub limit  : 350 bytes per message.",
		"",
		"  Split into 2 messages.",
		"  Preview of part 1:",
		"    " + info.Preview,
		"",
		"",
	}
	if strings.Join(texts, "\n") != strings.Join(want, "\n") {
		t.Errorf("dialog rows =\n%v\nwant\n%v", texts, want)
	}

	// The preview row carries Python's AttrMap(..., "irc_system") style.
	preview := rows[len(rows)-1-2]
	if wantColor := GetThemeColors(cd.app.Theme)["irc_system"]; preview.color != wantColor {
		t.Errorf("preview row color = %v, want the irc_system %v", preview.color, wantColor)
	}

	// Python's Pile focuses its first focusable widget: the Send Split button.
	sendBtn := dialogButton(t, cd, "Send Split")
	_ = dialogButton(t, cd, "Cancel")
	if cd.app.GetFocus() != tview.Primitive(sendBtn) {
		t.Error("Send Split must be the dialog's initial focus (Python's Pile default)")
	}
}

// TestChannelsSplitDialogSendSplitSendsPartsAndClearsDraft pins Python's
// send_split (Channels.py:907-916): every part is transmitted as its own room
// message in order, the composer is cleared, and the dialog closes.
func TestChannelsSplitDialogSendSplitSendsPartsAndClearsDraft(t *testing.T) {
	t.Parallel()

	cd := newSplitDialogCD(t, defaultMaxMsgBytes)

	var sent []string
	cd.OnSendMessage = func(text string) { sent = append(sent, text) }
	cd.roomWidget.editor.SetText(rrcCommunityDraft)
	cd.roomWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	sendBtn := dialogButton(t, cd, "Send Split")
	sendBtn.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})

	want := SplitMessage(rrcCommunityDraft, defaultMaxMsgBytes)
	if len(sent) != len(want) {
		t.Fatalf("sent %v messages, want %v part(s)", len(sent), len(want))
	}
	for i, part := range want {
		if sent[i] != part {
			t.Errorf("part %v = %q, want %q", i, sent[i], part)
		}
	}
	if got := cd.roomWidget.editor.GetText(); got != "" {
		t.Errorf("composer after Send Split = %q, want empty", got)
	}
	if cd.dialogOverlay != nil {
		t.Error("Send Split must close the dialog")
	}
}

// TestChannelsSplitDialogCancelKeepsDraft pins Python's cancel branch
// (Channels.py:904-905: cancel() → display.close_dialog()): the draft survives
// so the user can edit it down instead of losing what they typed.
func TestChannelsSplitDialogCancelKeepsDraft(t *testing.T) {
	t.Parallel()

	cd := newSplitDialogCD(t, defaultMaxMsgBytes)

	var sent []string
	cd.OnSendMessage = func(text string) { sent = append(sent, text) }
	cd.roomWidget.editor.SetText(rrcCommunityDraft)
	cd.roomWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	cancelBtn := dialogButton(t, cd, "Cancel")
	cancelBtn.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {})

	if len(sent) != 0 {
		t.Errorf("Cancel sent %v messages, want none", len(sent))
	}
	if got := cd.roomWidget.editor.GetText(); got != rrcCommunityDraft {
		t.Error("Cancel must keep the draft")
	}
	if cd.dialogOverlay != nil {
		t.Error("Cancel must close the dialog")
	}
}

// TestChannelsSplitDialogUnsplittableRecordsLocalError pins Python's
// `if not parts:` branch (Channels.py:893-896): when the limit is too small to
// split at all, no dialog opens and the reason is recorded as a local error row
// on the open room.
func TestChannelsSplitDialogUnsplittableRecordsLocalError(t *testing.T) {
	t.Parallel()

	cd := newSplitDialogCD(t, defaultMaxMsgBytes)

	var kind, room, text string
	cd.OnLocalMessage = func(k, r, msg string) error { kind, room, text = k, r, msg; return nil }

	cd.ShowSplitDialog("hello world", 5)

	if cd.dialogOverlay != nil {
		t.Error("an unsplittable message must not open a dialog")
	}
	if kind != "error" {
		t.Errorf("local row kind = %q, want %q", kind, "error")
	}
	if room != "general" {
		t.Errorf("local row room = %q, want %q", room, "general")
	}
	want := "Message is 11 bytes but per-message limit is too small to split."
	if text != want {
		t.Errorf("local row text = %q, want %q", text, want)
	}
}

// TestShowRoomWiresHubMessageLimitIntoComposer pins the live hub-limit read the
// composer's over-limit gate depends on. Python reads
// `self.hub.max_msg_body_bytes or 350` at SEND time (Channels.py:879), so the
// composer must follow the hub rather than a value snapshotted when the room
// view was built: the WELCOME carrying the limit routinely lands after it.
func TestShowRoomWiresHubMessageLimitIntoComposer(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewChannelsDisplay(app, nil)
	hub := &fakeHubView{name: "Hub A", status: hubStatusConnected, rooms: []string{"general"}, maxMsgBodyBytes: 350}
	cd.SetHubs([]HubView{hub})
	cd.ShowRoom(0, "general", nil)

	if got := cd.roomWidget.MaxMessageBytes(); got != 350 {
		t.Fatalf("composer limit = %v, want the advertised 350", got)
	}

	// A WELCOME arriving after the room view was built raises the hub's limit;
	// the composer must pick it up without a rebuild.
	hub.maxMsgBodyBytes = 700
	if got := cd.roomWidget.MaxMessageBytes(); got != 700 {
		t.Errorf("composer limit = %v, want the hub's live 700", got)
	}

	var sent []string
	cd.OnSendMessage = func(text string) { sent = append(sent, text) }
	cd.roomWidget.editor.SetText(rrcCommunityDraft)
	cd.roomWidget.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	if len(sent) != 1 || sent[0] != rrcCommunityDraft {
		t.Fatalf("sent %v messages, want the 362-byte draft sent whole under the hub's 700-byte limit", len(sent))
	}
	if cd.dialogOverlay != nil {
		t.Error("a draft the hub's limit allows must not open the split dialog")
	}
}
