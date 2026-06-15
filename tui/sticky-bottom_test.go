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

func TestStickyBottomTrackerNew(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	if !tracker.IsActive() {
		t.Error("new tracker should be active (sticky)")
	}
}

func TestStickyBottomTrackerScrollUp(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	tracker.OnScrollUp()
	if tracker.IsActive() {
		t.Error("should be inactive after scroll up")
	}
}

func TestStickyBottomTrackerNewMessage(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	tracker.OnScrollUp()
	tracker.OnNewMessage()

	if !tracker.IsActive() {
		t.Error("should be active after new message when previously scrolled up")
	}
}

func TestStickyBottomTrackerScrollDown(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	tracker.OnScrollUp()
	tracker.OnScrollDown()

	if !tracker.IsActive() {
		t.Error("should be active after scrolling to bottom")
	}
}

func TestStickyBottomTrackerScrollMid(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	tracker.OnScrollUp()
	tracker.OnScrollDown()
	tracker.OnScrollUp()

	if tracker.IsActive() {
		t.Error("should be inactive after scrolling back up from bottom")
	}
}

func TestStickyBottomTrackerResize(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	tracker.OnScrollUp()
	tracker.OnResize(100)

	if !tracker.IsActive() {
		t.Error("should be active after resize when previously at top")
	}
}

func TestStickyBottomTrackerMultipleNewMessages(t *testing.T) {
	t.Parallel()

	tracker := NewStickyBottom()
	tracker.OnScrollUp()
	tracker.OnNewMessage()
	tracker.OnNewMessage()

	if !tracker.IsActive() {
		t.Error("should be active after multiple new messages")
	}
}
