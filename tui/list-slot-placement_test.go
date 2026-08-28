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
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// newC2App mounts a ConversationsDisplay with an OPEN conversation so the
// two-pane layout is [list | conversation] before a dialog opens.
func newC2App(t *testing.T) (*App, *ConversationsDisplay) {
	t.Helper()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	const hash = "c2000000000000000000000000000000000000"
	cd := NewConversationsDisplay(app, []ConversationInfo{
		{SourceHash: hash, DisplayName: "C2 Peer", TrustLevel: "trusted"},
	})
	cd.OnLoadMessages = func(string) []ConversationMessage {
		return []ConversationMessage{{
			Content:   "hi",
			Timestamp: time.Unix(1700000000, 0),
			State:     lxmfStateSent, SourceHash: []byte{1},
		}}
	}
	app.Main.SetDisplay("conversations", cd.Widget())
	app.SetRoot()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(func() { screen.Fini() })
	screen.SetSize(135, 32)
	app.SetScreen(screen)
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.SelectPage("conversations")
	cd.DisplayConversation(hash)
	app.Main.Root().Draw(screen)
	return app, cd
}

// TestC2DialogReplacesListColumnInPlace pins C2: opening a list-slot dialog
// REPLACES the list column at index 0 (Python
// columns_widget.contents[0] = (overlay, options), Conversations.py:1024-1029)
// so the layout stays [dialog | conversation] — the open conversation remains
// visible on the right. The previous Go build appended the overlay to the END
// of the content Flex, throwing the layout to [conversation | dialog], and
// CloseListSlotDialog appended leftPanel at the end again, leaving the panes
// SWAPPED until the next conversation open self-healed them.
func TestC2DialogReplacesListColumnInPlace(t *testing.T) {
	t.Parallel()

	_, cd := newC2App(t)

	// Sanity: before any dialog, content[0] is the left list panel.
	if got := cd.content.GetItem(0); got != tview.Primitive(cd.leftPanel) {
		t.Fatalf("pre-dialog content[0] = %T, want the left list panel", got)
	}

	dialog := NewDialogLineBox("Test", tview.NewTextView().SetText("dialog body"), nil)
	cd.ShowListSlotDialog(dialog, 0, 44, 5)

	if got := cd.content.GetItem(0); got != tview.Primitive(cd.listSlotOverlay) {
		t.Errorf("dialog-open content[0] = %T, want the slot overlay (in-place list-column replacement)", got)
	}
	if got := cd.content.GetItem(1); got != cd.currentWidget.Widget() {
		t.Errorf("dialog-open content[1] = %T, want the OPEN conversation widget (must stay visible on the right)", got)
	}
	if n := cd.content.GetItemCount(); n != 2 {
		t.Errorf("content items while dialog open = %v, want 2", n)
	}

	cd.CloseListSlotDialog()

	if got := cd.content.GetItem(0); got != tview.Primitive(cd.leftPanel) {
		t.Errorf("after close content[0] = %T, want the left list panel (panes must not swap)", got)
	}
	if got := cd.content.GetItem(1); got != cd.currentWidget.Widget() {
		t.Errorf("after close content[1] = %T, want the conversation widget still on the right", got)
	}
}

// TestC2DialogCloseWithNoConversation pins the close path when the right pane
// is the empty detail placeholder (no conversation open): the restored layout
// must be [list | detail].
func TestC2DialogCloseWithNoConversation(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	cd := NewConversationsDisplay(app, nil)
	app.Main.SetDisplay("conversations", cd.Widget())
	app.SetRoot()

	dialog := NewDialogLineBox("Test", tview.NewTextView().SetText("x"), nil)
	cd.ShowListSlotDialog(dialog, 0, 44, 5)
	cd.CloseListSlotDialog()

	if got := cd.content.GetItem(0); got != tview.Primitive(cd.leftPanel) {
		t.Errorf("after close content[0] = %T, want the left list panel", got)
	}
	if got := cd.content.GetItem(1); got != tview.Primitive(cd.detail) {
		t.Errorf("after close content[1] = %T, want the empty detail placeholder", got)
	}
}
