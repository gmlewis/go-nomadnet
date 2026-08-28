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
)

// TestC6SyncDialogLayout pins C6: the Message Sync dialog matches Python's
// sync_conversations layout/strings/row order (Conversations.py:1359-1541):
//
//	title "Message Sync"
//	"Ⓝ (no default)" glyph header row (CENTERED)  ← Conversations.py:1512-1532
//	┄┄┄ divider
//	centered sync status row (the SyncProgressBar label, SyncProgressBar.get_text)
//	┄┄┄ divider
//	[ Sync Now | | Close ]
//	blank
//	(X) Download all
//	( ) Limit to  5
//
// The previous Go build used the title "Sync" and invented rows ("Download
// mode:", "Messages: ", "[0%]") that Python does not have.
func TestC6SyncDialogLayout(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)

	var result SyncDialogResult
	// Python reaches the general layout ("Ⓝ (no default)" + radios) only when
	// pn_options is non-empty (node_selector is not None,
	// Conversations.py:1496-1512); with no options AND no default node the
	// no-trusted-nodes variant applies.
	cd.ShowSyncDialog("", []string{"parity-node"}, SyncDialogHooks{
		Progress:    func() float64 { return 0 },
		Status:      func() string { return "Idle" },
		ShowPercent: func() bool { return false },
	}, func(r SyncDialogResult) { result = r })

	if !cd.dialogOpen {
		t.Fatal("sync dialog should be open")
	}

	// Gather every row text of the dialog body (the DialogLineBox content).
	rows := dialogRowTexts(cd.listSlotOverlay.Dialog())
	joined := strings.Join(rows, "\n")

	if cd.listSlotOverlay == nil {
		t.Fatal("sync dialog must be a list-slot overlay (columns_widget.contents[0])")
	}
	if title := cd.listSlotOverlay.Dialog().GetTitle(); title != "Message Sync" {
		t.Errorf("sync dialog title = %q, want %q", title, "Message Sync")
	}

	// Row 1: the glyph header — unicode set renders "Ⓝ " (trailing space) and
	// Python appends " (no default)" for an unset default node.
	if !strings.Contains(joined, "Ⓝ  (no default)") && !strings.Contains(joined, "Ⓝ (no default)") {
		t.Errorf("no propagation-node glyph header row found in %q", rows)
	}

	// The centered status row must show the plain status (no percent when
	// showPercent is false — Python SyncProgressBar.get_text).
	if !strings.Contains(joined, "Idle") {
		t.Errorf("no centered status row (Idle) found in %q", rows)
	}

	// Python's radios must be present in Python's wording.
	if !strings.Contains(joined, "Download all") {
		t.Errorf("missing the Download all radio in %q", rows)
	}
	if !strings.Contains(joined, "Limit to") {
		t.Errorf("missing the Limit to radio in %q", rows)
	}
	if !strings.Contains(joined, "5") {
		t.Errorf("missing the default limit IntEdit value 5 in %q", rows)
	}

	// Go-invented rows must be GONE.
	for _, banned := range []string{"Download mode:", "Messages:", "[0%]", "[50%]"} {
		if strings.Contains(joined, banned) {
			t.Errorf("Go-invented row %q still present in %q", banned, rows)
		}
	}

	// Buttons must carry Python's labels.
	if cd.syncSyncBtn == nil || cd.syncSyncBtn.Label() != "Sync Now" {
		t.Errorf("sync button = %v, want Sync Now", cd.syncSyncBtn)
	}
	foundClose := false
	for _, r := range rows {
		if strings.Contains(r, "Close") {
			foundClose = true
		}
	}
	if !foundClose {
		t.Errorf("no Close button row in %q", rows)
	}

	// Divider rows: Python separates header/status/buttons with divider1 ("┄")
	// rendered by urwid.Divider — custom-drawn, so assert on the RENDERED
	// dialog rather than the row texts.
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(60, 20)
	cd.listSlotOverlay.Dialog().SetRect(0, 0, 60, 20)
	cd.listSlotOverlay.Dialog().Draw(screen)
	dividers := 0
	for y := range 20 {
		for x := range 60 {
			s, _, _ := screen.Get(x, y)
			if s == "┄" {
				dividers++
			}
		}
	}
	if dividers == 0 {
		t.Error("no divider rows (┄) rendered in the sync dialog")
	}
	_ = result
}

// TestC6SyncDialogNoNodesVariant pins the no-trusted-nodes variant
// (Conversations.py:1534-1540): with no default propagation node and no
// options, the dialog shows the explainer text and a Close-only button row.
func TestC6SyncDialogNoNodesVariant(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)

	cd.ShowSyncDialog("", nil, SyncDialogHooks{}, nil)
	rows := dialogRowTexts(cd.listSlotOverlay.Dialog())
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "No trusted nodes found, cannot sync!") {
		t.Errorf("no-nodes explainer missing in %q", rows)
	}
	if strings.Contains(joined, "Sync Now") {
		t.Errorf("the no-nodes variant must not offer Sync Now (Python shows only Close): %q", rows)
	}
}

// TestC6SyncDialogLimitedMode pins the Limit-to flow: checking "Limit to" and
// pressing Sync Now reports SyncLimited with the parsed limit (Python
// sync_now: limit = ie_lim.value() when r_mlim is set, Conversations.py:1377).
func TestC6SyncDialogLimitedMode(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	cd := NewConversationsDisplay(app, nil)

	var got SyncDialogResult
	cd.ShowSyncDialog("", nil, SyncDialogHooks{}, func(r SyncDialogResult) { got = r })

	// Find the Limit-to radio among the dialog's widgets and check it.
	rLim := cd.syncLimitRadio
	if rLim == nil {
		t.Fatal("sync dialog did not expose the Limit-to radio")
	}
	rLim.SetChecked(true)
	cd.syncSyncBtn.selected()

	if got.Action != "sync" || got.Mode != SyncLimited {
		t.Errorf("sync result = %+v, want action=sync mode=SyncLimited", got)
	}
	if got.Limit != 5 {
		t.Errorf("limit = %v, want 5 (the IntEdit default)", got.Limit)
	}
}

// TestC4IngestURIDialogStrings pins C4: the Ingest URI dialog strings match
// Python's ingest_lxm_uri (Conversations.py:1118-1260): title "Ingest message
// URI", field caption "URI : ", buttons Ingest/Back.
func TestC4IngestURIDialogStrings(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)
	cd.IngestURIDialog(nil)

	if cd.listSlotOverlay == nil {
		t.Fatal("ingest dialog must be a list-slot overlay")
	}
	dialog := cd.listSlotOverlay.Dialog()
	if title := dialog.GetTitle(); title != "Ingest message URI" {
		t.Errorf("ingest dialog title = %q, want %q", title, "Ingest message URI")
	}
	rows := dialogRowTexts(dialog)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "URI : ") {
		t.Errorf("field caption \"URI : \" missing in %q", rows)
	}
	for _, want := range []string{"Ingest", "Back"} {
		if !strings.Contains(joined, want) {
			t.Errorf("button %q missing in %q", want, rows)
		}
	}
	for _, banned := range []string{"Save", "Cancel", "URI:"} {
		if strings.Contains(joined, banned) {
			t.Errorf("old Go string %q still present in %q", banned, rows)
		}
	}
}

// TestC3PeerInfoPinCheckbox pins C3: the Peer Info dialog renders "Pin to top"
// as a real CHECKBOX (Python urwid.CheckBox("Pin to top"), Conversations.py:890)
// — not plain text — and its state feeds Save (Pinned).
func TestC3PeerInfoPinCheckbox(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, nil)

	entry := PeerInfoEntry{
		SourceHash:  "c30000000000000000000000000000000000",
		DisplayName: "C3 Peer",
		TrustLevel:  "unknown",
	}
	saved := PeerInfoEntry{}
	cd.ShowPeerInfoDialog(entry, PeerInfoDialogHooks{}, func(e PeerInfoEntry) { saved = e })

	dialog := cd.listSlotOverlay.Dialog()
	rows := dialogRowTexts(dialog)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Pin to top") {
		t.Errorf("\"Pin to top\" row missing in %q", rows)
	}
	// The checkbox must be a real tview.Checkbox in the dialog tree.
	if !dialogContainsCheckbox(dialog, "Pin to top") {
		t.Error("Pin to top is not rendered as a checkbox (plain text?)")
	}

	// Toggling the checkbox and pressing Save persists Pinned.
	cb := findCheckbox(dialog, "Pin to top")
	if cb == nil {
		t.Fatal("checkbox not found")
	}
	cb.SetChecked(true)
	pressDialogButton(dialog, "Save")
	if !saved.Pinned {
		t.Errorf("saved entry Pinned = false, want true (checkbox state must feed Save)")
	}
}
