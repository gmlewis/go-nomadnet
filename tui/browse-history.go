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

// BrowseHistory manages browsing history with back/forward navigation.
// This is the pure-logic state machine extracted from BrowserDisplay.
// Matches the history logic in Python's Browser.py.
type BrowseHistory struct {
	history []string
	idx     int
}

// NewBrowseHistory creates an empty browsing history.
func NewBrowseHistory() *BrowseHistory {
	return &BrowseHistory{}
}

// Navigate loads a URL into the history. Empty URLs are ignored.
// If we're not at the end of history, forward entries are truncated
// (branching navigation), matching Python's behavior.
func (bh *BrowseHistory) Navigate(url string) {
	if url == "" {
		return
	}

	// Truncate forward history if branching
	if bh.idx < len(bh.history)-1 {
		bh.history = bh.history[:bh.idx+1]
	}

	bh.history = append(bh.history, url)
	bh.idx = len(bh.history) - 1
}

// GoBack moves to the previous URL. No-op if already at start.
func (bh *BrowseHistory) GoBack() {
	if bh.idx > 0 {
		bh.idx--
	}
}

// GoForward moves to the next URL. No-op if already at end.
func (bh *BrowseHistory) GoForward() {
	if bh.idx < len(bh.history)-1 {
		bh.idx++
	}
}

// CurrentURL returns the currently displayed URL, or empty string.
func (bh *BrowseHistory) CurrentURL() string {
	if len(bh.history) == 0 || bh.idx < 0 || bh.idx >= len(bh.history) {
		return ""
	}
	return bh.history[bh.idx]
}

// CanGoBack returns true if there are entries before the current position.
func (bh *BrowseHistory) CanGoBack() bool {
	return bh.idx > 0
}

// CanGoForward returns true if there are entries after the current position.
func (bh *BrowseHistory) CanGoForward() bool {
	return bh.idx < len(bh.history)-1
}

// History returns a copy of the full history list.
func (bh *BrowseHistory) History() []string {
	out := make([]string, len(bh.history))
	copy(out, bh.history)
	return out
}
