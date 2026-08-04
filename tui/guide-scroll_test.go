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

import "testing"

// TestShowTopicResetsScrollToTop pins the fix for the Guide scroll-reset-to-top
// bug. Python's Guide.display_topic -> set_content_widgets (Guide.py:138-154,
// 226-234) rebuilds the entire Scrollable widget on every topic switch, which
// implicitly resets scroll to the top. The Go port reuses a single tview
// TextView and calls SetText, which does NOT reset the line offset (tview's
// clear/SetText leave lineOffset untouched), so the offset leaks from the
// previously-viewed topic: selecting topic N after scrolling topic N-1 to the
// bottom opens topic N already scrolled part-way down — the first visible line
// is NOT the topic's `>Title` heading. showTopic must explicitly ScrollTo(0,0).
func TestShowTopicResetsScrollToTop(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 40, 12)

	gd.showTopic(0)
	// Simulate the user scrolling the reader down on topic 0 (e.g. End).
	gd.reader.ScrollTo(5, 0)
	if row, _ := gd.reader.GetScrollOffset(); row == 0 {
		t.Fatal("precondition: reader should be scrolled down before switching topics")
	}

	// Switching to a new topic must reset the reader to the top.
	gd.showTopic(1)
	if row, _ := gd.reader.GetScrollOffset(); row != 0 {
		t.Errorf("after showTopic(1) scroll row = %v, want 0 (offset leaked from previous topic)", row)
	}
}

// TestShowPlaceholderResetsScrollToTop asserts the placeholder path also
// resets the reader, so a topic shown afterwards is not at a stale offset.
func TestShowPlaceholderResetsScrollToTop(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 40, 12)

	gd.showTopic(0)
	gd.reader.ScrollTo(5, 0)
	gd.showPlaceholder()
	if row, _ := gd.reader.GetScrollOffset(); row != 0 {
		t.Errorf("after showPlaceholder scroll row = %v, want 0", row)
	}
}

// TestRerenderPreservesScrollOffset locks that the resize re-render path does
// NOT reset scroll — unlike a topic switch, a width change must keep the
// reader at the user's current position. The reset must live in showTopic, not
// in the shared renderMarkup/rerender path (also used by jumpToAnchor/resize).
func TestRerenderPreservesScrollOffset(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 40, 12)

	gd.showTopic(0)
	gd.reader.ScrollTo(4, 0)
	before, _ := gd.reader.GetScrollOffset()
	if before == 0 {
		t.Fatal("precondition: reader should be scrolled before resize")
	}

	// A width change triggers guideReader.SetRect -> rerender; the offset must
	// be preserved (not reset to 0).
	gd.rerender(50)
	after, _ := gd.reader.GetScrollOffset()
	if after == 0 {
		t.Errorf("rerender reset scroll to 0 (before=%v); resize must preserve the user's scroll position", before)
	}
}
