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

// rrcCommunityDraft is the 362-byte draft captured live from the glenn-kamrui
// session on 2026-09-14 while the RNS Community hub's #general room was open.
// It is 12 bytes over the 350-byte per-message limit the composer used to
// hardcode, which is exactly why C-d appeared to do nothing: the over-limit
// gate returned without a wired split dialog, so the draft was never sent and
// nothing was reported. Kept verbatim as the regression fixture.
const rrcCommunityDraft = "@qbit - I don't understand - are you talking about 24-bit ASCII art like nomadnet can render? If so,\n" +
	"I'm seeing it just fine through tmux. I've got this in my ~/.tmux.conf:\n" +
	"# Enable true-color (24-bit) passthrough for terminals that support it.\n" +
	"# Without this, tmux downsamples true-color escape sequences to 256 colors.\n" +
	"set -ga terminal-overrides \",*256col*:Tc\""

// TestRRCommunityDraftIsOverTheDefaultLimit pins the fixture's size: the draft
// must stay just over the 350-byte default, which is the condition that tripped
// the bug. (That a 350-byte body still fits one RNS link envelope — the reason
// the limit exists — is pinned on the wire encoder in the rrc package.)
func TestRRCommunityDraftIsOverTheDefaultLimit(t *testing.T) {
	t.Parallel()

	size := len([]byte(rrcCommunityDraft))
	if size != 362 {
		t.Fatalf("draft = %v bytes, want 362", size)
	}
	if size <= defaultMaxMsgBytes {
		t.Errorf("draft (%v bytes) must exceed the %v-byte default limit", size, defaultMaxMsgBytes)
	}
}

// TestRoomWidgetOverLimitUsesLiveHubLimit pins Python's live limit read
// (Channels.py:879: `limit = self.hub.max_msg_body_bytes or 350`): a hub that
// advertises more room than the 350-byte default must accept the draft, so the
// over-limit gate cannot be pinned to a hardcoded 350.
func TestRoomWidgetOverLimitUsesLiveHubLimit(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	rw.hubMaxMsgBytesFn = func() int { return 400 }

	var sent []string
	rw.OnSendMessage = func(text string) { sent = append(sent, text) }
	rw.editor.SetText(rrcCommunityDraft)

	rw.sendMessage()

	if len(sent) != 1 || sent[0] != rrcCommunityDraft {
		t.Fatalf("sent = %v messages, want the 362-byte draft sent whole", len(sent))
	}
	if rw.editor.GetText() != "" {
		t.Error("a sent draft must clear the composer")
	}
	if got := rw.MaxMessageBytes(); got != 400 {
		t.Errorf("MaxMessageBytes = %v, want the hub's advertised 400", got)
	}
}

// TestRoomWidgetOverLimitFallsBackToConfiguredLimit pins the `or 350` half of
// Python's expression: a hub that never advertised a limit (zero) keeps the
// statically configured value in force, so the gate stays at 350.
func TestRoomWidgetOverLimitFallsBackToConfiguredLimit(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	rw.hubMaxMsgBytesFn = func() int { return 0 }

	if got := rw.MaxMessageBytes(); got != defaultMaxMsgBytes {
		t.Errorf("MaxMessageBytes = %v, want the %v default", got, defaultMaxMsgBytes)
	}
}

// TestRoomWidgetOverLimitDialogGetsLiveLimit pins the limit handed to the
// split dialog: Python passes the limit it read at send time
// (Channels.py:881 → _open_split_dialog(text, limit)), so the dialog must
// report the hub's advertised value, not the composer's default.
func TestRoomWidgetOverLimitDialogGetsLiveLimit(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	rw.hubMaxMsgBytesFn = func() int { return 120 }

	var gotText string
	var gotLimit int
	rw.OnSplitDialog = func(text string, limit int) { gotText, gotLimit = text, limit }
	rw.editor.SetText(rrcCommunityDraft)

	rw.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	if gotText != rrcCommunityDraft {
		t.Errorf("dialog text = %q, want the draft", gotText)
	}
	if gotLimit != 120 {
		t.Errorf("dialog limit = %v, want the hub's advertised 120", gotLimit)
	}
	if rw.editor.GetText() != rrcCommunityDraft {
		t.Error("the draft must be kept while the split dialog is open")
	}
}

// TestRoomWidgetOverLimitWithoutDialogSaysWhyAndKeepsDraft pins the safety net
// around the over-limit gate: when no split dialog is wired the key is never
// silently dead — the draft is kept AND the reason is recorded as a local error
// row, so the user learns the message was too long instead of watching C-d do
// nothing.
func TestRoomWidgetOverLimitWithoutDialogSaysWhyAndKeepsDraft(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")

	var sent []string
	rw.OnSendMessage = func(text string) { sent = append(sent, text) }
	var kind, notice string
	rw.OnLocalMessage = func(k, text string) error { kind, notice = k, text; return nil }
	rw.editor.SetText(rrcCommunityDraft)

	rw.handleInput(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone))

	if len(sent) != 0 {
		t.Fatalf("sent %v messages, want none over the limit", len(sent))
	}
	if rw.editor.GetText() != rrcCommunityDraft {
		t.Error("an over-limit draft must be kept")
	}
	if kind != "error" {
		t.Errorf("local notice kind = %q, want %q", kind, "error")
	}
	for _, want := range []string{"362 bytes", "350 bytes"} {
		if !strings.Contains(notice, want) {
			t.Errorf("local notice %q must state %q", notice, want)
		}
	}
}

// TestRoomWidgetSendSplitParts pins Python _open_split_dialog's send_split
// (Channels.py:907-916): every part goes out as its own room message, in order,
// and the composer is cleared afterwards. The parts already fit the limit, so
// they must not be re-gated.
func TestRoomWidgetSendSplitParts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub1", "general")
	// A live limit that would trip the gate if the parts were re-checked as a
	// whole: only per-part sizes matter here.
	rw.hubMaxMsgBytesFn = func() int { return 20 }

	var sent []string
	rw.OnSendMessage = func(text string) { sent = append(sent, text) }
	rw.editor.SetText(rrcCommunityDraft)

	parts := SplitMessage(rrcCommunityDraft, 350)
	if len(parts) < 2 {
		t.Fatalf("SplitMessage returned %v parts, want at least 2", len(parts))
	}
	rw.SendSplitParts(parts)

	if len(sent) != len(parts) {
		t.Fatalf("sent %v messages, want %v (one per part)", len(sent), len(parts))
	}
	for i, part := range parts {
		if sent[i] != part {
			t.Errorf("part %v = %q, want %q", i, sent[i], part)
		}
	}
	if rw.editor.GetText() != "" {
		t.Error("SendSplitParts must clear the composer")
	}
}
