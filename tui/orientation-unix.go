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

//go:build unix

package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// stdinTerminalPixels asks the client's own terminal how big it is, in pixels.
//
// The descriptor is stdin, which is the terminal by the client's own definition:
// a client that does not have one on stdin does not take its text-UI path at all
// (see cmd/gonomadnet/tty-hosted.go), and the appliance's console host gives the
// client a pty on all three descriptors, so the window size it reads here is the
// one the host last applied.
//
// A terminal that reports no physical size leaves both pixel fields at zero —
// the same way it does for any program asking — and a descriptor that is not a
// terminal at all fails the ioctl. Both are the same answer here: no physical
// size to report, so the caller falls back to the cell grid.
func stdinTerminalPixels() (width, height int, ok bool) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdin.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 0, 0, false
	}
	return int(ws.Xpixel), int(ws.Ypixel), true
}
