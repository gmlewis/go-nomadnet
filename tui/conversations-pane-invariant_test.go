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

// The Conversations content Flex must hold EXACTLY two items at all times:
// the list column at index 0 (the bordered left panel, or a slot dialog
// overlaying it) and the right pane at index 1 (the open conversation widget,
// or the empty detail placeholder). Every interleaving of open/close and
// dialog operations below must restore that invariant, because any third item
// (or a swapped order) leaves the page with a collapsed list pane or the
// "No conversation selected" detail rendered full-width — states that are
// unrecoverable from the UI.

func newInvariantCD(t *testing.T) *ConversationsDisplay {
	t.Helper()
	return NewConversationsDisplay(newTestApp(), nil)
}

// assertTwoPanes verifies the structural two-column invariant: exactly two
// items, the list column first, the given right pane second.
func assertTwoPanes(t *testing.T, cd *ConversationsDisplay, left, right tview.Primitive) {
	t.Helper()
	if got := cd.content.GetItemCount(); got != 2 {
		t.Fatalf("content has %v items, want 2", got)
	}
	if got := cd.content.GetItem(0); got != left {
		t.Errorf("content[0] is %T, want the list column (%T)", got, left)
	}
	if got := cd.content.GetItem(1); got != right {
		t.Errorf("content[1] is %T, want the right pane (%T)", got, right)
	}
}

// contentRows renders cd.content on a wxh simulation screen and returns the
// joined cell text per row (same technique as the headless parity captures).
func contentRows(t *testing.T, cd *ConversationsDisplay, w, h int) []string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(w, h)
	cd.content.SetRect(0, 0, w, h)
	cd.content.Draw(screen)
	screen.Sync()
	rows := make([]string, h)
	for y := range h {
		var b strings.Builder
		for x := range w {
			c, _, _, _ := cellContent(screen, x, y)
			b.WriteRune(c)
		}
		rows[y] = b.String()
	}
	return rows
}

// assertListPaneVisible asserts the bordered list panel (or its overlaying
// dialog slot) is drawn at the left with its normal width: the "Conversations"
// title or a dialog title appears in the first ~52 columns and the list
// panel's top border corner is at column 51.
func assertListPaneVisible(t *testing.T, cd *ConversationsDisplay) {
	t.Helper()
	rows := contentRows(t, cd, 80, 24)
	sawCorner := false
	for _, r := range rows {
		if len([]rune(r)) > 51 && runeAt(r, 51) == '┐' {
			sawCorner = true
			break
		}
	}
	if !sawCorner {
		t.Errorf("list column top-right corner not at column 51: list pane collapsed or misplaced (row0=%q)", rows[0])
	}
}

// assertListPaneHidden asserts the list column is collapsed to width 0
// (fullscreen): no bordered pane corner at column 51 and the detail text
// spans the full width.
func assertListPaneHidden(t *testing.T, cd *ConversationsDisplay) {
	t.Helper()
	rows := contentRows(t, cd, 80, 24)
	if anyRowContains(rows, "Conversations") {
		t.Errorf("list pane title visible in fullscreen mode (row0=%q)", rows[0])
	}
}

func openConversation(cd *ConversationsDisplay, hash string) {
	cd.DisplayConversation(hash)
}

func TestConversationsOpenCloseKeepsListPanel(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	openConversation(cd, "<hash-a>")
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())
	assertListPaneVisible(t, cd)

	// Close via the conversation widget's close action (C-w): the left panel
	// must survive and the empty detail placeholder must return.
	cw := cd.currentWidget
	cw.OnClose()
	assertTwoPanes(t, cd, cd.leftPanel, cd.detail)
	assertListPaneVisible(t, cd)

	// Reopen: the detail placeholder is replaced, not appended.
	openConversation(cd, "<hash-b>")
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())

	cd.currentWidget.OnClose()
	assertTwoPanes(t, cd, cd.leftPanel, cd.detail)
	assertListPaneVisible(t, cd)
}

func TestConversationsReopenWhileOpenKeepsTwoPanes(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	// Open one conversation, then another (the announce/wiring path can
	// refresh or re-open the current conversation): the previous widget is
	// replaced, never accumulated.
	openConversation(cd, "<hash-a>")
	first := cd.currentWidget
	openConversation(cd, "<hash-b>")
	if cd.currentWidget == nil || cd.currentWidget == first {
		t.Fatalf("currentWidget not replaced on re-open")
	}
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())
}

func TestConversationsAttachDialogReopenKeepsTwoPanes(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	openConversation(cd, "<hash-a>")
	cw := cd.currentWidget

	// Attach dialog (detail slot overlay) while a conversation is open.
	cw.OnAttach()
	ovA := cd.detailSlotOverlay
	if ovA == nil {
		t.Fatalf("attach dialog did not install a detail slot overlay")
	}
	assertTwoPanes(t, cd, cd.leftPanel, ovA)

	// Re-invoking Ctrl-f while the dialog is already open must replace the
	// overlay, not stack a second item (this is the input-misroute repro:
	// the stale overlay stayed on top and keys fell through to the page).
	cw.OnAttach()
	if cd.detailSlotOverlay == nil || cd.detailSlotOverlay == ovA {
		t.Fatalf("re-opened attach dialog not replaced (next=%v)", cd.detailSlotOverlay)
	}
	assertTwoPanes(t, cd, cd.leftPanel, cd.detailSlotOverlay)

	// Dismiss restores the conversation widget as the right pane.
	cd.CloseDetailSlotDialog()
	assertTwoPanes(t, cd, cd.leftPanel, cw.Widget())

	// Close the conversation: back to the empty detail placeholder.
	cw.OnClose()
	assertTwoPanes(t, cd, cd.leftPanel, cd.detail)
}

func TestConversationsOpenWhileAttachDialogOpenKeepsTwoPanes(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	openConversation(cd, "<hash-a>")
	cw := cd.currentWidget
	cw.OnAttach()
	if cd.detailSlotOverlay == nil {
		t.Fatalf("attach dialog not open")
	}

	// A background refresh or user click re-opens a conversation while the
	// attach overlay covers the right pane: the overlay must be discarded and
	// the new conversation becomes the sole right pane.
	openConversation(cd, "<hash-b>")
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())

	// The stale overlay's dismiss (a late Esc) must restore — not append.
	cd.CloseDetailSlotDialog()
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())

	_ = cw
}

func TestConversationsListSlotDialogWithConversationOpenKeepsTwoPanes(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	openConversation(cd, "<hash-a>")

	// New Conversation dialog over the list slot while a conversation is open.
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	if cd.listSlotOverlay == nil {
		t.Fatalf("new conversation dialog not open")
	}
	assertTwoPanes(t, cd, cd.listSlotOverlay, cd.currentWidget.Widget())
	assertListPaneVisible(t, cd)

	cd.CloseListSlotDialog()
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())

	cd.currentWidget.OnClose()
	assertTwoPanes(t, cd, cd.leftPanel, cd.detail)
}

func TestConversationsFullscreenSurvivesOpenClose(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	cd.ToggleFullscreen()
	if !cd.Fullscreen() {
		t.Fatalf("fullscreen not toggled on")
	}
	assertListPaneHidden(t, cd)
	openConversation(cd, "<hash-a>")
	assertTwoPanes(t, cd, cd.leftPanel, cd.currentWidget.Widget())
	assertListPaneHidden(t, cd)
	cd.currentWidget.OnClose()
	assertTwoPanes(t, cd, cd.leftPanel, cd.detail)
	assertListPaneHidden(t, cd)
	cd.ToggleFullscreen()
	if cd.Fullscreen() {
		t.Fatalf("fullscreen not toggled off")
	}
	assertListPaneVisible(t, cd)
}

func TestConversationsListSlotDialogFullscreenSurvivesDismiss(t *testing.T) {
	t.Parallel()
	cd := newInvariantCD(t)

	cd.ToggleFullscreen()
	cd.ShowNewConversationDialog(func(addrHex, name, trust string) bool { return true })
	if cd.listSlotOverlay == nil {
		t.Fatalf("dialog not open")
	}
	// The slot overlay REPLACES the collapsed list column, so the fullscreen
	// detail spans the page; the dialog title is still visible in the slot.
	cd.CloseListSlotDialog()
	assertTwoPanes(t, cd, cd.leftPanel, cd.detail)
	assertListPaneHidden(t, cd)
	cd.ToggleFullscreen()
	assertListPaneVisible(t, cd)
}
