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

//go:build !unix

package tui

// stdinTerminalPixels reports that this platform's terminal has no physical size
// to ask for.
//
// A window size on a platform without a pty carries cells and nothing else, so
// there is no pixel field for a terminal to fill in and no ioctl that would read
// one. The caller falls back to the cell grid, which is the whole of what such a
// terminal can say about its shape.
func stdinTerminalPixels() (width, height int, ok bool) {
	return 0, 0, false
}
