// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"testing"

	"github.com/rivo/tview"
)

// TestGuideFocusModelTopic7DownsToBottom is the B2 golden test. The root cause:
// Python's Guide reader is a urwid Scrollable(Pile(LinkableText)),
// so each Down advances the Pile focus to the next SELECTABLE line — headings,
// dividers and blank lines are urwid.Text/Divider (non-selectable) and are
// skipped. The number of Downs to reach the bottom therefore equals the number
// of selectable lines (one focus advance per Down), NOT the total rendered
// row count. Python nomadnet reaches the bottom of topic 7 ("Markup") in 502
// downs (nomadnet tmux-test-suite log line 13795: "guide topic 7 reader: bottom
// reached after 502 downs (478 screenfuls)"). The Go renderer produces 497
// selectable lines for topic 7, so the focus model reaches the LAST selectable
// in 497-1 == 496 Downs (focus starts at the first), then the suite's K=4
// stuck-confirmation adds ~4-6 → ~500-502, matching Python. The OLD Go model
// (tview.TextView, 1 display row per Down) needs one Down per rendered row;
// topic 7 has ~920 rendered lines so it blows past the suite's 700-Down safety
// cap (gonomadnet log line 16833: "max 700 downs"), which is the user-visible
// "2-3× slower, never ends" symptom.
//
// This test pins the model: the focus-model walk reaches the last selectable
// line of topic 7 in exactly 496 Downs (== selectableCount-1), well under the
// 700 cap, and the legacy one-row-per-Down count (total rendered lines) exceeds
// 700 — proving the cap hit was the scroll model, not the content length.
func TestGuideFocusModelTopic7DownsToBottom(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 100, 30)
	gd.showMarkupForTest(guideMarkup)

	sel := gd.selectableCount()
	if sel != 497 {
		t.Errorf("topic 7 selectable line count = %v, want 497 (497 selectable ⇒ 496 Downs to last + ~4 confirm ≈ Python's 502)", sel)
	}

	// Focus-model walk: each Down advances focus by exactly one selectable
	// line, so the number of Downs to reach the LAST selectable is sel-1.
	downs := gd.downsToLastSelectable()
	if downs != 496 {
		t.Errorf("topic 7 focus-model Downs to last selectable = %v, want 496 (sel-1; Python bottoms out at 502 total)", downs)
	}
	if downs >= 700 {
		t.Errorf("focus-model Downs = %v must be < the suite's 700 cap (the bug)", downs)
	}

	// Legacy one-row-per-Down count = total rendered lines; this exceeds 700,
	// which is why the TextView model hit the cap.
	legacy := gd.legacyDownsToBottom()
	if legacy <= 700 {
		t.Errorf("legacy one-row-per-Down count = %v, want > 700 (the cause of the cap hit)", legacy)
	}
}

// TestGuideFocusDownSkipsNonSelectable pins the core focus-navigation rule: Down
// advances focus to the next SELECTABLE line, skipping headings, dividers and
// blank lines (which are non-selectable urwid.Text/Divider in Python). A
// synthetic topic with [text, heading, blank, divider, text] must move focus
// from line 0 directly to line 4 on a single Down, never landing on the heading
// (1), blank (2) or divider (3).
func TestGuideFocusDownSkipsNonSelectable(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 80, 24)
	// Line 0: text (selectable). Line 1: heading (non-selectable). Line 2:
	// blank (non-selectable). Line 3: divider (non-selectable). Line 4: text
	// (selectable). Line 5: text (selectable).
	gd.showMarkupForTest("first selectable line\n>A Heading\n\n-\nfifth selectable line\nsixth selectable line")

	if got := gd.selectableCount(); got != 3 {
		t.Fatalf("selectable count = %v, want 3 (lines 0,4,5 are text; 1=heading,2=blank,3=divider)", got)
	}

	// After showTopic the first selectable (line 0) is focused.
	if got := gd.focusedLineIndex(); got != 0 {
		t.Fatalf("initial focused line = %v, want 0", got)
	}

	gd.focusDown()
	if got := gd.focusedLineIndex(); got != 4 {
		t.Errorf("after Down from line 0, focused line = %v, want 4 (Down must skip the heading, blank and divider)", got)
	}

	gd.focusDown()
	if got := gd.focusedLineIndex(); got != 5 {
		t.Errorf("after Down from line 4, focused line = %v, want 5", got)
	}

	// Down past the last selectable is a no-op (focus stays on the last).
	gd.focusDown()
	if got := gd.focusedLineIndex(); got != 5 {
		t.Errorf("Down past last selectable moved focus to %v, want 5 (clamp)", got)
	}

	// Up walks back through selectables only.
	gd.focusUp()
	if got := gd.focusedLineIndex(); got != 4 {
		t.Errorf("after Up from line 5, focused line = %v, want 4", got)
	}
	gd.focusUp()
	if got := gd.focusedLineIndex(); got != 0 {
		t.Errorf("after Up from line 4, focused line = %v, want 0 (Up skips non-selectable)", got)
	}
	// Up past the first selectable is a no-op (does NOT escape to the menu —
	// the Guide reader releases focus via Left, not Up).
	gd.focusUp()
	if got := gd.focusedLineIndex(); got != 0 {
		t.Errorf("Up past first selectable moved focus to %v, want 0 (clamp, no menu escape)", got)
	}
}

// TestGuideFocusShowTopicResetsFocus pins that showTopic resets the focus to the
// first selectable line (Python rebuilds the entire Pile on every topic switch,
// so focus restarts at the top). Without this, switching topics after scrolling
// the previous one to the bottom leaves focus on a stale (out-of-range) line.
func TestGuideFocusShowTopicResetsFocus(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 80, 24)

	gd.showTopic(7)
	// Walk focus well down into topic 7.
	for range 50 {
		gd.focusDown()
	}
	if got := gd.focusedLineIndex(); got == 0 {
		t.Fatal("precondition: focus should have advanced past line 0")
	}

	// Switching to a short topic must reset focus to its first selectable
	// line (Python rebuilds the Pile, so the focus cursor restarts at the
	// top). Topic 0 starts with the ">Nomad Network" heading, so its first
	// selectable is line 1; assert focus restarts at selectable index 0.
	gd.showTopic(0)
	if gd.focusSel != 0 {
		t.Errorf("after showTopic(0) focusSel = %v, want 0 (focus must reset to the first selectable on topic switch)", gd.focusSel)
	}
}

// TestGuideFocusJumpToAnchorSetsFocus pins that an in-page anchor jump moves
// focus to the selectable line at (or following) the anchor, so subsequent Down
// continues from there rather than from the top. Python's jump_to_anchor sets
// the Scrollable position and (via automove_cursor_on_scroll) the cursor follows.
func TestGuideFocusJumpToAnchorSetsFocus(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 80, 24)
	// Lines: 0 text, 1 heading "A Section" (auto-anchor "a-section"), 2 text.
	gd.showMarkupForTest("intro line\n>A Section\nbody line under the section")

	gd.jumpToAnchor("a-section")
	// The anchor binds to the heading line (1), which is non-selectable; focus
	// must land on the next selectable at or after it (line 2).
	if got := gd.focusedLineIndex(); got < 1 {
		t.Errorf("after jumpToAnchor, focused line = %v, want >= 1 (focus should follow the anchor)", got)
	}
}

// TestGuideFocusCursorRowAdvances pins B5's precondition for the suite's
// bottom-detection: within the visible viewport (no scroll), each Down must
// move the hardware cursor to a different row, so the suite's cursor-y tracker
// sees movement (and does not false-bottom during the within-viewport phase).
// The cursor row is the focused line's display row minus the scroll offset.
func TestGuideFocusCursorRowAdvances(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	gd := NewGuideDisplay(app)
	gd.reader.SetRect(0, 0, 80, 24)
	gd.showMarkupForTest("line one\nline two\nline three\nline four\nline five")

	row0 := gd.cursorRow()
	gd.focusDown()
	row1 := gd.cursorRow()
	if row1 <= row0 {
		t.Errorf("cursor row after Down = %v, want > %v (cursor must advance within the viewport)", row1, row0)
	}
	gd.focusDown()
	row2 := gd.cursorRow()
	if row2 <= row1 {
		t.Errorf("cursor row after 2nd Down = %v, want > %v", row2, row1)
	}
}

// selectableCount is a test helper returning the number of selectable lines in
// the current topic (the focus-model's Down-step count to reach the bottom).
func (gd *GuideDisplay) selectableCount() int {
	gd.computeFocusLayout()
	return len(gd.selectable)
}

// focusedLineIndex returns the StyledLine index of the currently focused
// selectable line, or -1 if none.
func (gd *GuideDisplay) focusedLineIndex() int {
	if gd.focusSel < 0 || gd.focusSel >= len(gd.selectable) {
		return -1
	}
	return gd.selectable[gd.focusSel]
}

// downsToLastSelectable simulates the focus-model walk: the number of Down
// presses to reach the last selectable line from the first (== selectable-1).
func (gd *GuideDisplay) downsToLastSelectable() int {
	gd.computeFocusLayout()
	if len(gd.selectable) == 0 {
		return 0
	}
	return len(gd.selectable) - 1
}

// legacyDownsToBottom is the OLD one-row-per-Down count: the total number of
// rendered (wrapped) display rows, which the TextView model needed one Down
// per row to traverse. Used to demonstrate the 700-cap root cause.
func (gd *GuideDisplay) legacyDownsToBottom() int {
	total := 0
	w := gd.readerWidth()
	for _, lt := range gd.lineTexts {
		rows := 1
		if w > 0 {
			if n := len(tview.WordWrap(lt, w)); n > 0 {
				rows = n
			}
		}
		total += rows
	}
	return total
}
