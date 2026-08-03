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

func TestBrowseHistoryNew(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	if bh.CurrentURL() != "" {
		t.Errorf("CurrentURL() = %q, want empty", bh.CurrentURL())
	}
	if bh.CanGoBack() {
		t.Error("CanGoBack() = true on empty history")
	}
	if bh.CanGoForward() {
		t.Error("CanGoForward() = true on empty history")
	}
}

func TestBrowseHistoryNavigate(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("http://example.com")

	if bh.CurrentURL() != "http://example.com" {
		t.Errorf("CurrentURL() = %q, want %q", bh.CurrentURL(), "http://example.com")
	}
	if bh.CanGoBack() {
		t.Error("CanGoBack() = true after single navigation")
	}
	if bh.CanGoForward() {
		t.Error("CanGoForward() = true after single navigation")
	}
}

func TestBrowseHistoryBackForward(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("page1")
	bh.Navigate("page2")
	bh.Navigate("page3")

	if bh.CurrentURL() != "page3" {
		t.Errorf("after 3 navs: CurrentURL() = %q, want %q", bh.CurrentURL(), "page3")
	}

	bh.GoBack()
	if bh.CurrentURL() != "page2" {
		t.Errorf("after GoBack: CurrentURL() = %q, want %q", bh.CurrentURL(), "page2")
	}

	bh.GoForward()
	if bh.CurrentURL() != "page3" {
		t.Errorf("after GoForward: CurrentURL() = %q, want %q", bh.CurrentURL(), "page3")
	}
}

func TestBrowseHistoryBranchTruncation(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("page1")
	bh.Navigate("page2")
	bh.Navigate("page3")
	bh.GoBack()
	bh.GoBack()

	// Now navigate to new page — should truncate forward history
	bh.Navigate("page_new")

	if bh.CurrentURL() != "page_new" {
		t.Errorf("CurrentURL() = %q, want %q", bh.CurrentURL(), "page_new")
	}
	if bh.CanGoForward() {
		t.Error("CanGoForward() = true after branch navigation")
	}
	if !bh.CanGoBack() {
		t.Error("CanGoBack() = false after branch navigation")
	}
}

func TestBrowseHistoryBackAtStart(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("page1")
	bh.GoBack()

	if bh.CurrentURL() != "page1" {
		t.Errorf("GoBack at start: CurrentURL() = %q, want %q", bh.CurrentURL(), "page1")
	}
}

func TestBrowseHistoryForwardAtEnd(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("page1")
	bh.GoForward()

	if bh.CurrentURL() != "page1" {
		t.Errorf("GoForward at end: CurrentURL() = %q, want %q", bh.CurrentURL(), "page1")
	}
}

func TestBrowseHistoryEmptyNavigateIgnored(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("")
	bh.Navigate("page1")

	if bh.CurrentURL() != "page1" {
		t.Errorf("CurrentURL() = %q, want %q", bh.CurrentURL(), "page1")
	}
	if bh.CanGoBack() {
		t.Error("CanGoBack() = true after empty navigate")
	}
}

func TestBrowseHistoryHistory(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("page1")
	bh.Navigate("page2")
	bh.Navigate("page3")

	history := bh.History()
	if len(history) != 3 {
		t.Errorf("History() returned %v items, want 3", len(history))
	}
	if history[0] != "page1" || history[1] != "page2" || history[2] != "page3" {
		t.Errorf("History() = %v, want [page1 page2 page3]", history)
	}
}

func TestBrowseHistoryBranchThenNavigate(t *testing.T) {
	t.Parallel()

	bh := NewBrowseHistory()
	bh.Navigate("page1")
	bh.Navigate("page2")
	bh.Navigate("page3")
	bh.GoBack()           // at page2
	bh.GoBack()           // at page1
	bh.Navigate("branch") // truncates page2, page3

	history := bh.History()
	if len(history) != 2 {
		t.Errorf("History() after branch: %v items, want 2", len(history))
	}
	if history[0] != "page1" || history[1] != "branch" {
		t.Errorf("History() = %v, want [page1 branch]", history)
	}
}
