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
)

// TestLXMessageWidgetProgressRow pins the live transfer progress row that
// Python's LXMessageWidget inserts between the header and the body while an
// outbound RESOURCE message is still in transit (Conversations.py:2709-2727):
// "  [<bar>] <pct>%" where the bar is 20 cells — filled with U+2588 and the
// remainder U+2591 when the color mode is >= 256. The row is present only when
// the message is outbound, its state is below SENT, and the router reports a
// pending outbound entry for the same hash whose representation is RESOURCE.
func TestLXMessageWidgetProgressRow(t *testing.T) {
	t.Parallel()

	full, empty := "█", "░"
	bar := func(filled int) string {
		return strings.Repeat(full, filled) + strings.Repeat(empty, progressBarWidth-filled)
	}

	cases := []struct {
		name       string
		msg        ConversationMessage
		transfer   LiveTransfer
		found      bool
		wantRow    string
		wantRowSet bool
	}{
		{
			name:     "in transit resource at 45 percent",
			msg:      ConversationMessage{Content: "big body", State: lxmfStateSending, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer: LiveTransfer{Progress: 0.45, State: lxmfStateSending, Representation: lxmfRepresentationResource},
			found:    true,
			// int(20*0.45) == 9 filled, int(0.45*100) == 45.
			wantRow: "  [" + bar(9) + "] 45%",
		},
		{
			name:     "in transit resource at 0 percent",
			msg:      ConversationMessage{Content: "big body", State: lxmfStateOutbound, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer: LiveTransfer{Progress: 0, State: lxmfStateOutbound, Representation: lxmfRepresentationResource},
			found:    true,
			wantRow:  "  [" + bar(0) + "] 0%",
		},
		{
			name:     "transfer failed shows the cross warning",
			msg:      ConversationMessage{Content: "big body", State: lxmfStateSending, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer: LiveTransfer{Progress: 0.5, State: lxmfStateFailed, Representation: lxmfRepresentationResource},
			found:    true,
			wantRow:  "  ✕ Transfer failed",
		},
		{
			name:     "transfer rejected shows the too-large message",
			msg:      ConversationMessage{Content: "big body", State: lxmfStateSending, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer: LiveTransfer{Progress: 0.5, State: lxmfStateRejected, Representation: lxmfRepresentationResource},
			found:    true,
			wantRow:  "  ✕ Rejected: too large or not accepted",
		},
		{
			name:       "settled transfer hides the row",
			msg:        ConversationMessage{Content: "big body", State: lxmfStateSending, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer:   LiveTransfer{Progress: 1, State: lxmfStateSent, Representation: lxmfRepresentationResource},
			found:      true,
			wantRowSet: false,
		},
		{
			name:       "packet representation shows no progress row",
			msg:        ConversationMessage{Content: "small", State: lxmfStateSending, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer:   LiveTransfer{Progress: 1, State: lxmfStateSending, Representation: lxmfRepresentationPacket},
			found:      true,
			wantRowSet: false,
		},
		{
			name:       "no matching pending transfer shows no row",
			msg:        ConversationMessage{Content: "big body", State: lxmfStateSending, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			found:      false,
			wantRowSet: false,
		},
		{
			name:       "already sent conversation message shows no row",
			msg:        ConversationMessage{Content: "big body", State: lxmfStateSent, SourceHash: []byte{7}, Hash: []byte{1, 2, 3}},
			transfer:   LiveTransfer{Progress: 0.9, State: lxmfStateSending, Representation: lxmfRepresentationResource},
			found:      true,
			wantRowSet: false,
		},
		{
			name:       "inbound message shows no row",
			msg:        ConversationMessage{Content: "big body", State: lxmfStateSending, SourceHash: []byte{9}, Hash: []byte{1, 2, 3}},
			transfer:   LiveTransfer{Progress: 0.9, State: lxmfStateSending, Representation: lxmfRepresentationResource},
			found:      true,
			wantRowSet: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp()
			app.Glyphs = GetGlyphSet(GlyphUnicode)
			app.ColorMode = ColorModeTrue
			cw := NewConversationWidget(app, "01010101")
			cw.OwnHash = []byte{7}
			cw.TimeFormat = "%Y-%m-%d %H:%M:%S"

			msg := tc.msg
			msg.Timestamp = time.Unix(1700000000, 0).UTC()
			cw.OnLiveTransfer = func(hash []byte) (LiveTransfer, bool) {
				if !tc.found || !bytes.Equal(hash, msg.Hash) {
					return LiveTransfer{}, false
				}
				return tc.transfer, true
			}

			entry := cw.renderMessageEntry(msg)
			rows := strings.Split(entry.GetText(true), "\n")
			// Row 0 is the header; the progress row (when present) is row 1.
			if !tc.wantRowSet && tc.wantRow == "" {
				if len(rows) > 1 && strings.HasPrefix(rows[1], "  [") {
					t.Errorf("rows[1] = %q, want no progress row", rows[1])
				}
				return
			}
			if len(rows) < 2 {
				t.Fatalf("entry rows = %q, want a progress row at index 1", entry.GetText(true))
			}
			if rows[1] != tc.wantRow {
				t.Errorf("progress row = %q, want %q (full rows: %q)", rows[1], tc.wantRow, rows)
			}
			// The progress row must sit between the header and the body.
			if len(rows) < 3 || rows[2] != "  big body" {
				t.Errorf("body row = %q, want \"  big body\" after the progress row", rows[min(2, len(rows)-1)])
			}
		})
	}
}

// TestLXMessageWidgetProgressRowAdvances pins that the progress row advances as
// new samples arrive: re-rendering the same message with a larger transfer
// progress produces a longer filled bar and a higher percentage, mirroring
// Python's _poll_progress redraw (Conversations.py:2766-2790).
func TestLXMessageWidgetProgressRowAdvances(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.ColorMode = ColorModeTrue
	cw := NewConversationWidget(app, "02020202")
	cw.OwnHash = []byte{7}
	cw.TimeFormat = "%Y-%m-%d %H:%M:%S"

	msg := ConversationMessage{
		Content:            "big body",
		Timestamp:          time.Unix(1700000000, 0).UTC(),
		State:              lxmfStateSending,
		SourceHash:         []byte{7},
		Hash:               []byte{0x42},
		TransportEncrypted: true,
	}
	var progress float64
	cw.OnLiveTransfer = func([]byte) (LiveTransfer, bool) {
		return LiveTransfer{Progress: progress, State: lxmfStateSending, Representation: lxmfRepresentationResource}, true
	}

	progress = 0.25
	first := strings.Split(cw.renderMessageEntry(msg).GetText(true), "\n")[1]
	progress = 0.75
	second := strings.Split(cw.renderMessageEntry(msg).GetText(true), "\n")[1]

	if first != "  ["+strings.Repeat("█", 5)+strings.Repeat("░", 15)+"] 25%" {
		t.Errorf("at 25%%: row = %q, want a 5-cell filled bar", first)
	}
	if second != "  ["+strings.Repeat("█", 15)+strings.Repeat("░", 5)+"] 75%" {
		t.Errorf("at 75%%: row = %q, want a 15-cell filled bar", second)
	}
	if first == second {
		t.Error("progress row did not advance between samples")
	}
}

// TestConversationProgressPollNeeded pins the poll gate the UI ticker consults:
// true only while a matching RESOURCE transfer is live for an outbound message
// below SENT, so the fast poll runs only during a transfer.
func TestConversationProgressPollNeeded(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.ColorMode = ColorModeTrue
	cw := NewConversationWidget(app, "03030303")
	cw.OwnHash = []byte{7}
	cw.TimeFormat = "%Y-%m-%d %H:%M:%S"
	cw.SetMessages([]ConversationMessage{{
		Content:    "big body",
		Timestamp:  time.Unix(1700000000, 0).UTC(),
		State:      lxmfStateSending,
		SourceHash: []byte{7},
		Hash:       []byte{0x51},
	}})

	if cw.ProgressPollNeeded() {
		t.Fatal("ProgressPollNeeded() = true with no OnLiveTransfer callback, want false")
	}

	live := true
	cw.OnLiveTransfer = func([]byte) (LiveTransfer, bool) {
		if !live {
			return LiveTransfer{}, false
		}
		return LiveTransfer{Progress: 0.5, State: lxmfStateSending, Representation: lxmfRepresentationResource}, true
	}
	if !cw.ProgressPollNeeded() {
		t.Error("ProgressPollNeeded() = false during a live resource transfer, want true")
	}

	live = false
	if cw.ProgressPollNeeded() {
		t.Error("ProgressPollNeeded() = true after the transfer left the queue, want false")
	}
}
