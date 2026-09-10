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
)

// TestSlashHelpLocalRoomMessages pins Python's /help UX (Channels.py:1003-1007
// and _local_message at 938-946): each SLASH_HELP line becomes a SYSTEM row in
// the current room buffer with a live timestamp (the IRC "[HH:MM:SS] →" look),
// not a dialog overlay. The hub must also receive the rows so a later
// SetMessages/refresh does not wipe them ("help goes away quickly").
func TestSlashHelpLocalRoomMessages(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	rw := NewRoomWidget(app, "hub", "general")

	var recorded []string
	rw.OnLocalMessage = func(kind, text string) error {
		if kind != "system" {
			t.Errorf("OnLocalMessage kind = %q, want system", kind)
		}
		recorded = append(recorded, text)
		return nil
	}

	rw.handleSlashCommand("/help")

	wantLines := strings.Split(SlashHelpText(), "\n")
	msgs := rw.ChatMessages()
	if len(msgs) != len(wantLines) {
		t.Fatalf("chat rows = %v, want %v /help system lines", len(msgs), len(wantLines))
	}
	if len(recorded) != len(wantLines) {
		t.Fatalf("OnLocalMessage calls = %v, want %v", len(recorded), len(wantLines))
	}
	for i, want := range wantLines {
		if msgs[i].Text != want {
			t.Errorf("row %v text = %q, want %q", i, msgs[i].Text, want)
		}
		if !msgs[i].IsSystem || msgs[i].IsError {
			t.Errorf("row %v IsSystem=%v IsError=%v, want system notice", i, msgs[i].IsSystem, msgs[i].IsError)
		}
		if msgs[i].TsMs == 0 {
			t.Errorf("row %v TsMs = 0 — local notices must be timestamped like Python", i)
		}
		if recorded[i] != want {
			t.Errorf("hub record %v = %q, want %q", i, recorded[i], want)
		}
	}

	// A hub refresh replaces the widget buffer; rows that were recorded on
	// the hub (as rrcRoomMessages does) must still be present.
	refreshed := make([]ChannelMessage, len(recorded))
	for i, text := range recorded {
		refreshed[i] = ChannelMessage{Text: text, IsSystem: true, TsMs: msgs[i].TsMs}
	}
	rw.SetMessages(refreshed)
	after := rw.ChatMessages()
	if len(after) != len(wantLines) {
		t.Fatalf("after SetMessages rows = %v, want %v (help must survive refresh)", len(after), len(wantLines))
	}
	if after[0].Text != wantLines[0] {
		t.Errorf("after refresh first row = %q, want %q", after[0].Text, wantLines[0])
	}
}
