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

// FullscreenState tracks the list panel width for fullscreen toggle.
// When toggled fullscreen, the list width is saved and set to 0;
// toggling again restores the saved width.
type FullscreenState struct {
	ListWidth  int
	savedWidth int
}

// IsFullscreen returns true if the list panel is hidden (width = 0).
func (fs *FullscreenState) IsFullscreen() bool {
	return fs.ListWidth == 0 && fs.savedWidth > 0
}

// Toggle switches between fullscreen (hidden list) and normal view.
func (fs *FullscreenState) Toggle() {
	if fs.ListWidth != 0 {
		fs.savedWidth = fs.ListWidth
		fs.ListWidth = 0
	} else {
		fs.ListWidth = fs.savedWidth
	}
}

// SetWidth sets the list panel width and clears the saved width.
func (fs *FullscreenState) SetWidth(w int) {
	fs.ListWidth = w
	fs.savedWidth = w
}
