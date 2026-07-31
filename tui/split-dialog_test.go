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
	"reflect"
	"strings"
	"testing"
)

// TestSplitDialogPythonParity verifies the pure content computation behind
// RoomWidget.openSplitDialog against Python's _open_split_dialog
// (Channels.py:889): the UTF-8 byte count, the split part count and noun, the
// part-1 preview (truncated to 70 code points with a U+2026 ellipsis, then
// newlines/tabs replaced by spaces), and the "too small to split" error
// message. Expected values were captured from /tmp/splitdialog_ref.py.
func TestSplitDialogPythonParity(t *testing.T) {
	t.Parallel()

	previewTrunc := func(prefix, char string) string {
		// "(i/K) " is 6 code points; first 70 code points = prefix + (70-len(prefix)) repeats.
		reps := 70 - len(prefix)
		return prefix + strings.Repeat(char, reps) + "…"
	}

	tests := []struct {
		name      string
		text      string
		limit     int
		wantBytes int
		wantK     int
		wantNoun  string
		wantPrev  string
		wantErr   string
	}{
		{
			name: "three parts ascii", text: "hello world this is a long message",
			limit: 20, wantBytes: 34, wantK: 3, wantNoun: "messages",
			wantPrev: "(1/3) hello world", wantErr: "",
		},
		{
			name: "too small to split", text: "hello world", limit: 5,
			wantBytes: 11, wantErr: "Message is 11 bytes but per-message limit is too small to split.",
		},
		{
			name: "preview truncation 100 x", text: strings.Repeat("x", 100), limit: 200,
			wantBytes: 100, wantK: 1, wantNoun: "message",
			wantPrev: previewTrunc("(1/1) ", "x"),
		},
		{
			name: "multiline preview", text: "line1\nline2\ttab here", limit: 200,
			wantBytes: 20, wantK: 1, wantNoun: "message",
			wantPrev: "(1/1) line1 line2 tab here",
		},
		{
			name: "exactly 70 chars truncates", text: strings.Repeat("y", 70), limit: 200,
			wantBytes: 70, wantK: 1, wantNoun: "message",
			wantPrev: previewTrunc("(1/1) ", "y"),
		},
		{
			name: "71 chars truncates same", text: strings.Repeat("y", 71), limit: 200,
			wantBytes: 71, wantK: 1, wantNoun: "message",
			wantPrev: previewTrunc("(1/1) ", "y"),
		},
		{
			name: "three parts multibyte", text: "café résumé nomadnet test message",
			limit: 18, wantBytes: 36, wantK: 3, wantNoun: "messages",
			wantPrev: "(1/3) café résum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info := ComputeSplitDialog(tt.text, tt.limit)
			if info.BodyBytes != tt.wantBytes {
				t.Errorf("BodyBytes = %d, want %d", info.BodyBytes, tt.wantBytes)
			}
			if info.Error != tt.wantErr {
				t.Errorf("Error = %q, want %q", info.Error, tt.wantErr)
			}
			if tt.wantErr != "" {
				return
			}
			if info.K != tt.wantK {
				t.Errorf("K = %d, want %d", info.K, tt.wantK)
			}
			if info.Noun != tt.wantNoun {
				t.Errorf("Noun = %q, want %q", info.Noun, tt.wantNoun)
			}
			if info.Preview != tt.wantPrev {
				t.Errorf("Preview = %q, want %q", info.Preview, tt.wantPrev)
			}
			if len(info.Parts) != tt.wantK {
				t.Errorf("len(Parts) = %d, want %d", len(info.Parts), tt.wantK)
			}
		})
	}
}

// TestSplitDialogLines verifies the literal dialog body lines built from a
// SplitDialogInfo, matching the urwid.Text strings Python assembles in
// _open_split_dialog (Channels.py:920-925).
func TestSplitDialogLines(t *testing.T) {
	t.Parallel()

	info := ComputeSplitDialog("hello world this is a long message", 20)
	lines := SplitDialogLines(info)
	want := []string{
		"  Message is 34 bytes.",
		"  Hub limit  : 20 bytes per message.",
		"  Split into 3 messages.",
		"  Preview of part 1:",
		"    (1/3) hello world",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("SplitDialogLines = %v, want %v", lines, want)
	}

	infoSingular := ComputeSplitDialog("short message fits", 200)
	wantSingular := []string{
		"  Message is 18 bytes.",
		"  Hub limit  : 200 bytes per message.",
		"  Split into 1 message.",
		"  Preview of part 1:",
		"    (1/1) short message fits",
	}
	if got := SplitDialogLines(infoSingular); !reflect.DeepEqual(got, wantSingular) {
		t.Errorf("SplitDialogLines singular = %v, want %v", got, wantSingular)
	}
}
