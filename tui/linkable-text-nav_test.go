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

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
)

// linkCall records one delegate invocation with its target+fields.
type linkCall struct {
	target string
	fields string
}

// fakeLinkDelegate records HandleLink / MarkedLink / MicronReleasedFocus calls
// so tests can assert the exact dispatch sequence of LinkableText, mirroring
// the Python url_delegate interface (MicronParser.py:881-918).
type fakeLinkDelegate struct {
	handleLinkCalls    []linkCall
	markedLinkCalls    []linkCall
	releasedFocusCount int
}

func (d *fakeLinkDelegate) HandleLink(target, fields string) {
	d.handleLinkCalls = append(d.handleLinkCalls, linkCall{target, fields})
}

func (d *fakeLinkDelegate) MarkedLink(target, fields string) {
	d.markedLinkCalls = append(d.markedLinkCalls, linkCall{target, fields})
}

func (d *fakeLinkDelegate) MicronReleasedFocus() {
	d.releasedFocusCount++
}

// navLinkSpans builds the canonical 3-span line used across the navigation
// tests: "See " + link("Click"→http://example.com, fields f1|f2) + " for info".
// Part positions are [0,4,9,18]; the link occupies char range [4,9).
func navLinkSpans() []micron.StyledSpan {
	return []micron.StyledSpan{
		{Text: "See ", FG: "#dddddd", BG: "default"},
		{Text: "Click", FG: "#dddddd", BG: "default", Link: &micron.LinkSpec{
			Label: "Click", URL: "http://example.com", Fields: "f1|f2",
		}},
		{Text: " for info", FG: "#dddddd", BG: "default"},
	}
}

// TestLinkNavPartPositions asserts the cumulative part-position table matches
// Python LinkableText.keypress's part_positions build (MicronParser.py:921-929):
// [0] followed by each part's length+running total.
func TestLinkNavPartPositions(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), &fakeLinkDelegate{})

	if got, want := lt.Text(), "See Click for info"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	want := []int{0, 4, 9, 18}
	got := lt.PartPositions()
	if len(got) != len(want) {
		t.Fatalf("PartPositions() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PartPositions()[%v] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestLinkNavLinkAtCursorTargetAndDisplay asserts find_item_at_pos lands on the
// link span and exposes both the target URL and the display (label) text — the
// "test target/display-text" mandate of task 3.2.
func TestLinkNavLinkAtCursorTargetAndDisplay(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), &fakeLinkDelegate{})

	lt.SetCursor(4) // first char of the "Click" link part
	link := lt.LinkAtCursor()
	if link == nil {
		t.Fatal("LinkAtCursor(4) = nil, want the Click link")
	}
	if link.URL != "http://example.com" {
		t.Errorf("link target (URL) = %q, want http://example.com", link.URL)
	}
	if link.Label != "Click" {
		t.Errorf("link display (Label) = %q, want Click", link.Label)
	}
	if link.Fields != "f1|f2" {
		t.Errorf("link Fields = %q, want f1|f2", link.Fields)
	}

	lt.SetCursor(0) // plain "See " part
	if lt.LinkAtCursor() != nil {
		t.Error("LinkAtCursor(0) != nil, want nil on a plain part")
	}
}

// TestLinkNavRightNavigation asserts "right" advances the cursor through each
// part position and, at the last position, wraps to cursor 0 and propagates
// "down" (the non-in_columns path). Mirrors MicronParser.py:951-963.
func TestLinkNavRightNavigation(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), &fakeLinkDelegate{})
	now := time.Unix(1000, 0)

	if got := lt.HandleKey("right", now); got != "" {
		t.Errorf("right #1 propagated %q, want %q", got, "")
	}
	if lt.Cursor() != 4 {
		t.Errorf("after right #1 cursor = %v, want 4", lt.Cursor())
	}

	lt.HandleKey("right", now)
	if lt.Cursor() != 9 {
		t.Errorf("after right #2 cursor = %v, want 9", lt.Cursor())
	}

	lt.HandleKey("right", now)
	if lt.Cursor() != 18 {
		t.Errorf("after right #3 cursor = %v, want 18", lt.Cursor())
	}

	// At the last part position: no further move → wrap to 0 and propagate "down".
	if got := lt.HandleKey("right", now); got != "down" {
		t.Errorf("right at last propagated %q, want %q", got, "down")
	}
	if lt.Cursor() != 0 {
		t.Errorf("after right-at-last cursor = %v, want 0", lt.Cursor())
	}
}

// TestLinkNavLeftNavigationAndRelease asserts "left" steps the cursor back to
// the previous part position, and "left" at position 0 releases focus to the
// delegate (micron_released_focus) rather than propagating. Mirrors
// MicronParser.py:964-974.
func TestLinkNavLeftNavigationAndRelease(t *testing.T) {
	t.Parallel()
	d := &fakeLinkDelegate{}
	lt := NewLinkableTextFromSpans(navLinkSpans(), d)
	now := time.Unix(1000, 0)

	lt.SetCursor(9)
	if got := lt.HandleKey("left", now); got != "" {
		t.Errorf("left from 9 propagated %q, want %q", got, "")
	}
	if lt.Cursor() != 4 {
		t.Errorf("after left from 9 cursor = %v, want 4", lt.Cursor())
	}

	// left at position 0 releases focus; no key propagation.
	lt.SetCursor(0)
	if got := lt.HandleKey("left", now); got != "" {
		t.Errorf("left at 0 propagated %q, want %q", got, "")
	}
	if d.releasedFocusCount != 1 {
		t.Errorf("MicronReleasedFocus called %v times, want 1", d.releasedFocusCount)
	}
}

// TestLinkNavActivateDispatches asserts enter/ACTIVATE on a link part dispatches
// delegate.handle_link with the link's target+fields. Mirrors MicronParser.py:937-941.
func TestLinkNavActivateDispatches(t *testing.T) {
	t.Parallel()
	d := &fakeLinkDelegate{}
	lt := NewLinkableTextFromSpans(navLinkSpans(), d)
	now := time.Unix(1000, 0)

	lt.SetCursor(4)
	if got := lt.HandleKey("enter", now); got != "" {
		t.Errorf("enter propagated %q, want %q", got, "")
	}
	if len(d.handleLinkCalls) != 1 {
		t.Fatalf("HandleLink calls = %v, want 1", len(d.handleLinkCalls))
	}
	if d.handleLinkCalls[0].target != "http://example.com" || d.handleLinkCalls[0].fields != "f1|f2" {
		t.Errorf("HandleLink = %+v, want {http://example.com f1|f2}", d.handleLinkCalls[0])
	}

	// enter on a plain part does nothing.
	lt.SetCursor(0)
	d.handleLinkCalls = nil
	lt.HandleKey("enter", now)
	if len(d.handleLinkCalls) != 0 {
		t.Errorf("enter on plain part dispatched %v, want 0", len(d.handleLinkCalls))
	}
}

// TestLinkNavUpDownPropagate asserts up/down reset the cursor to 0 and propagate
// the key unchanged (scrolling). Mirrors MicronParser.py:943-949.
func TestLinkNavUpDownPropagate(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), &fakeLinkDelegate{})
	now := time.Unix(1000, 0)

	lt.SetCursor(9)
	if got := lt.HandleKey("up", now); got != "up" {
		t.Errorf("up propagated %q, want %q", got, "up")
	}
	if lt.Cursor() != 0 {
		t.Errorf("after up cursor = %v, want 0", lt.Cursor())
	}

	lt.SetCursor(9)
	if got := lt.HandleKey("down", now); got != "down" {
		t.Errorf("down propagated %q, want %q", got, "down")
	}
	if lt.Cursor() != 0 {
		t.Errorf("after down cursor = %v, want 0", lt.Cursor())
	}
}

// TestLinkNavInColumns asserts in_columns mode makes "right" at the last
// position and "left" with cursor>0 propagate the key instead of wrapping/
// stepping — the columnar-layout path. Mirrors MicronParser.py:956-957,966-967.
func TestLinkNavInColumns(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), &fakeLinkDelegate{})
	lt.SetInColumns(true)
	now := time.Unix(1000, 0)

	lt.SetCursor(18) // last position (end of text)
	if got := lt.HandleKey("right", now); got != "right" {
		t.Errorf("in_columns right at last propagated %q, want %q", got, "right")
	}
	if lt.Cursor() != 18 {
		t.Errorf("in_columns right at last moved cursor to %v, want 18", lt.Cursor())
	}

	lt.SetCursor(9)
	if got := lt.HandleKey("left", now); got != "left" {
		t.Errorf("in_columns left propagated %q, want %q", got, "left")
	}
	if lt.Cursor() != 9 {
		t.Errorf("in_columns left moved cursor to %v, want 9", lt.Cursor())
	}
}

// TestLinkNavPeekLink asserts peek_link reports the focused link's target+fields
// to the delegate, or clears it (marked_link(None)) on a plain part. Mirrors
// MicronParser.py:910-918.
func TestLinkNavPeekLink(t *testing.T) {
	t.Parallel()
	d := &fakeLinkDelegate{}
	lt := NewLinkableTextFromSpans(navLinkSpans(), d)

	lt.SetCursor(4)
	lt.PeekLink()
	if len(d.markedLinkCalls) != 1 {
		t.Fatalf("MarkedLink calls = %v, want 1", len(d.markedLinkCalls))
	}
	if d.markedLinkCalls[0].target != "http://example.com" || d.markedLinkCalls[0].fields != "f1|f2" {
		t.Errorf("MarkedLink = %+v, want {http://example.com f1|f2}", d.markedLinkCalls[0])
	}

	// Plain part → clear the peek (empty target).
	d.markedLinkCalls = nil
	lt.SetCursor(0)
	lt.PeekLink()
	if len(d.markedLinkCalls) != 1 || d.markedLinkCalls[0].target != "" {
		t.Errorf("MarkedLink on plain = %+v, want one call with empty target", d.markedLinkCalls)
	}
}

// TestLinkNavKeyTimeout asserts the 2s key-timeout cursor visibility model: with
// a delegate the cursor is hidden until a keypress, then visible for key_timeout
// (2s) and hidden after. Mirrors MicronParser.py:982-992.
func TestLinkNavKeyTimeout(t *testing.T) {
	t.Parallel()
	d := &fakeLinkDelegate{}
	lt := NewLinkableTextFromSpans(navLinkSpans(), d)
	t0 := time.Unix(1000, 0)

	// Before any keypress (delegate.last_keypress=0): cursor hidden when focused.
	if lt.CursorVisible(t0, true) {
		t.Error("CursorVisible before keypress = true, want false")
	}

	// A keypress stamps last_keypress; cursor visible within 2s.
	lt.HandleKey("right", t0)
	if !lt.CursorVisible(t0.Add(1*time.Second), true) {
		t.Error("CursorVisible 1s after keypress = false, want true")
	}
	if lt.CursorVisible(t0.Add(3*time.Second), true) {
		t.Error("CursorVisible 3s after keypress = true, want false (2s timeout)")
	}
}

// TestLinkNavNoDelegateCursorAlwaysVisible asserts that without a delegate the
// cursor is always visible when focused (render condition delegate==None).
func TestLinkNavNoDelegateCursorAlwaysVisible(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), nil)
	if !lt.CursorVisible(time.Unix(1000, 0), true) {
		t.Error("CursorVisible with no delegate = false, want true")
	}
	if lt.CursorVisible(time.Unix(1000, 0), false) {
		t.Error("CursorVisible unfocused = true, want false")
	}
}

// TestLinkNavUnknownKeyPropagates asserts an unmapped key is returned unchanged
// for the parent to handle. Mirrors MicronParser.py:976-977.
func TestLinkNavUnknownKeyPropagates(t *testing.T) {
	t.Parallel()
	lt := NewLinkableTextFromSpans(navLinkSpans(), &fakeLinkDelegate{})
	if got := lt.HandleKey("x", time.Unix(1000, 0)); got != "x" {
		t.Errorf("unknown key propagated %q, want %q", got, "x")
	}
}
