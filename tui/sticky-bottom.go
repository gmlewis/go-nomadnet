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

// StickyBottom tracks whether the chat should auto-scroll to the bottom.
// Matches Python's StickyBottom behavior in Channels.py.
type StickyBottom struct {
	active bool
}

// NewStickyBottom creates a new sticky bottom tracker.
// Initial state is active (sticky on by default).
func NewStickyBottom() *StickyBottom {
	return &StickyBottom{active: true}
}

// IsActive returns true if the view should auto-scroll to the bottom.
func (sb *StickyBottom) IsActive() bool {
	return sb.active
}

// OnScrollUp is called when the user scrolls up.
// Disables sticky bottom.
func (sb *StickyBottom) OnScrollUp() {
	sb.active = false
}

// OnScrollDown is called when the user scrolls to the bottom.
// Re-enables sticky bottom.
func (sb *StickyBottom) OnScrollDown() {
	sb.active = true
}

// OnNewMessage is called when a new message arrives.
// Re-enables sticky bottom to auto-scroll to the new message.
func (sb *StickyBottom) OnNewMessage() {
	sb.active = true
}

// OnResize is called when the terminal is resized.
// Re-enables sticky bottom to handle the new layout.
func (sb *StickyBottom) OnResize(newHeight int) {
	sb.active = true
}
