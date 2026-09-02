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

// The sync footer must span the left pane's full inner width: Python renders
// it as a Text inside an AttrMap whose style paints the entire row
// (Conversations.py:517-545). tview's TextView paints its background only over
// the text run, so the line is padded to the row width — otherwise the
// trailing columns lose the menubar background (attribute-only divergence
// found by the differential explorer; text-only diffs cannot see it).
func TestSyncFooterPaddedToFullRowWidth(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, deleteSelectedModel())
	cd.setSyncText(cd.syncStatusLine())

	got := cd.syncStatus.GetText(false)
	want := cd.listWidth - 2
	if cd.listWidth != 52 {
		t.Fatalf("precondition: listWidth = %v, expected the default 52", cd.listWidth)
	}
	if len(got) != want {
		t.Fatalf("sync footer length = %v, want %v (padded to the pane's inner width)", len(got), want)
	}
	if !strings.HasPrefix(got, " Last sync: ") {
		t.Errorf("sync footer = %q, want the standard \" Last sync: \" prefix", got)
	}
	if strings.TrimRight(got, " ") != got[:len(strings.TrimRight(got, " "))] || strings.Contains(strings.TrimRight(got, " ")[17:], "\t") {
		t.Errorf("sync footer padding contains non-space runes: %q", got)
	}
}

// A longer line (sync label appended) is not truncated by the padding.
func TestSyncFooterLongLineNotTruncated(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	cd := NewConversationsDisplay(app, deleteSelectedModel())
	line := " Last sync: 2026-09-01 20:42:27  (via a very long propagation node label)"
	cd.setSyncText(line)
	if got := cd.syncStatus.GetText(false); strings.TrimSpace(got) != strings.TrimSpace(line) {
		t.Errorf("sync footer = %q; content must be preserved verbatim", got)
	}
}
