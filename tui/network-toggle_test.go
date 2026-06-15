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
)

func TestAnnounceDisplayToggle(t *testing.T) {
	t.Parallel()

	ann := AnnounceEntry{
		Timestamp:   time.Now(),
		SourceHash:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
		DisplayName: "Alice",
		Type:       "peer",
	}

	got := FormatAnnounceEntry(ann, false)
	if got != "Alice" {
		t.Errorf("show name: got %q, want %q", got, "Alice")
	}

	got = FormatAnnounceEntry(ann, true)
	if got != ann.SourceHash {
		t.Errorf("show hash: got %q, want %q", got, ann.SourceHash)
	}
}

func TestAnnounceDisplayToggleEmptyName(t *testing.T) {
	t.Parallel()

	ann := AnnounceEntry{
		Timestamp:  time.Now(),
		SourceHash: "deadbeef",
		Type:       "node",
	}

	// Empty name falls back to hash
	got := FormatAnnounceEntry(ann, false)
	if got != "deadbeef" {
		t.Errorf("empty name fallback: got %q, want %q", got, "deadbeef")
	}
}

func TestAnnounceDisplayToggleTypeIcons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		annType string
		wantIcon string
	}{
		{"node", "Ⓝ"},
		{"peer", "Ⓟ"},
		{"pn", "↑"},
		{"", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.annType, func(t *testing.T) {
			t.Parallel()
			ann := AnnounceEntry{
				Timestamp:   time.Now(),
				SourceHash:  "hash",
				DisplayName: "Test",
				Type:        tt.annType,
			}
			got := FormatAnnounceFull(ann, false)
			if len(got) == 0 {
				t.Error("FormatAnnounceFull returned empty")
			}
			gotRunes := []rune(got)
			wantRunes := []rune(tt.wantIcon)
			if gotRunes[0] != wantRunes[0] {
				t.Errorf("type %q: icon = %c, want %c", tt.annType, gotRunes[0], wantRunes[0])
			}
		})
	}
}

func TestDisplayModeToggle(t *testing.T) {
	t.Parallel()

	mode := DisplayName
	if mode != DisplayName {
		t.Error("initial mode should be DisplayName")
	}

	mode = ToggleDisplayMode(mode)
	if mode != DisplayHash {
		t.Errorf("after toggle: got %d, want DisplayHash(%d)", mode, DisplayHash)
	}

	mode = ToggleDisplayMode(mode)
	if mode != DisplayName {
		t.Errorf("after second toggle: got %d, want DisplayName(%d)", mode, DisplayName)
	}
}

func TestFormatAnnounceEntryTruncate(t *testing.T) {
	t.Parallel()

	ann := AnnounceEntry{
		Timestamp:  time.Now(),
		SourceHash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
		Type:       "peer",
	}

	got := FormatAnnounceEntry(ann, true)
	if got != ann.SourceHash {
		t.Errorf("full hash display: got %q, want %q", got, ann.SourceHash)
	}
}
