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

	"github.com/rivo/tview"
)

// The composer footer must be REPLACED by the centered identity-unknown
// warning when the peer's identity keys are not known (Python
// check_editor_allowed, Conversations.py:2198-2215 in the installed 1.2.8):
// without the keys the editor is unusable, so Python swaps the whole footer
// for the banner and restores it when the identity arrives.
func TestConversationEditorAllowedBanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cw := NewConversationWidget(app, "abcdef0123456789abcdef0123456789")

	// Unknown identity: the banner replaces the editor.
	cw.OnEditorAllowed = func(string) bool { return false }
	cw.buildFooter()

	if cw.footerArea == nil {
		t.Fatal("footerArea is nil")
	}
	if cw.footerArea.GetItemCount() == 0 {
		t.Fatal("footer area is empty with an unknown peer identity")
	}
	banner := cw.footerArea.GetItem(0)
	tv, ok := banner.(*tview.TextView)
	_ = ok
	if tv == nil {
		t.Fatalf("footer item = %T, want the banner TextView", banner)
	}
	// The banner is PRE-WRAPPED with urwid's space wrap and ceil-left
	// centered line by line — compare against the parity definition itself.
	text := tv.GetText(true)
	got := strings.Split(text, "\n")
	want := urwidSpaceWrap(cw.editorAllowedBannerText(), 46)
	if len(got) != len(want) {
		t.Fatalf("banner rows = %v, want %v (urwidSpaceWrap of the banner text)", len(got), len(want))
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != strings.TrimSpace(want[i]) {
			t.Errorf("banner row %v = %q, want the urwid-wrapped %q", i, got[i], want[i])
		}
	}
	joined := strings.Join(want, "\n")
	for _, frag := range []string{
		"You cannot currently message this peer",
		"identity keys are not known",
		"Ctrl-E, and use the query button",
	} {
		if !strings.Contains(joined, frag) {
			t.Errorf("banner text missing %q in %q", frag, joined)
		}
	}
	if strings.Contains(joined, "Send a message") {
		t.Error("banner replaced the editor but editor text leaked in")
	}

	// Known identity: the editor returns.
	cw.OnEditorAllowed = func(string) bool { return true }
	cw.buildFooter()
	if cw.footerArea.GetItem(0) == banner {
		t.Error("footer still shows the banner with a known peer identity")
	}
}

// The delete-conversation confirmation must be a LIST-SLOT overlay with
// Python's exact chrome (installed 1.2.8, Conversations.py:571-598): the "?"
// DialogLineBox title, the centered two-line "Delete conversation with\n<name>"
// body, and flat Yes/No buttons — not the global DialogManager form whose
// "Confirm" title and whole-screen centering diverged (differential explorer,
// C-x finding).
func TestDeleteConversationConfirmListSlotDialog(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	var yes, no int
	cd.DeleteConversationConfirm("Trusted Seed Peer",
		func() { yes++ },
		func() { no++ })

	if cd.listSlotOverlay == nil {
		t.Fatal("delete confirm must be a list-slot overlay (Python overlays the list column)")
	}
	dialog := cd.listSlotOverlay.Dialog()
	if title := dialog.GetTitle(); title != "?" {
		t.Errorf("delete dialog title = %q, want %q", title, "?")
	}
	rows := dialogRowTexts(dialog)
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"Delete conversation with", "Trusted Seed Peer", "Yes", "No"} {
		if !strings.Contains(joined, want) {
			t.Errorf("delete dialog missing %q in %q", want, rows)
		}
	}
	for _, banned := range []string{"Confirm", "Delete conversation with Trusted"} {
		if strings.Contains(joined, banned) {
			t.Errorf("old global-dialog string %q still present in %q", banned, rows)
		}
	}

	// Yes fires the onYes path and dismisses; No fires onNo.
	cd.listSlotOverlay.Dialog().InputHandler()
	if cd.listSlotOverlay == nil {
		t.Fatal("dialog dismissed prematurely")
	}
	if yes != 0 || no != 0 {
		t.Fatalf("callbacks fired without activation (yes=%v no=%v)", yes, no)
	}
}
