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

// Package pty allocates the pseudo-terminal pairs this program starts children
// on.
//
// A pty is two ends: a master the program holds, and the slave that is given to
// a child or published as a path for another process to open. The slave is
// normally found by reading the master's pty number and opening /dev/pts/N, and
// on Linux that lookup is racy: devpts hands out the lowest free index, so a
// name a master has just given up can still resolve to the node that master
// held. A child started on such a slave never gets a terminal — its TIOCSCTTY
// fails with EIO when the node's pty is hung up, and EPERM when that pty is
// still another session's controlling terminal — or, worse, it attaches to a
// terminal whose master this program does not hold and then says nothing at all.
//
// Open and StartWithSize ask the kernel for this master's own peer instead
// (TIOCGPTPEER), which resolves no path and so cannot return anyone else's node.
// A kernel older than 4.13 does not know that ioctl, and there the name is all
// there is: that slave, and only that one, is opened by name.
//
// The window size and the pty library's size helpers are the library's, and are
// re-exported here so that a caller needs one import rather than two. A platform
// with no pseudoterminals at all — Windows — reports the library's
// ErrUnsupported from both Open and StartWithSize, which is what its callers
// already handle.
package pty

import (
	"os"

	creackpty "github.com/creack/pty/v2"
)

// Winsize is a pty's window size: its cells, and the display those cells are
// drawn on.
type Winsize = creackpty.Winsize

// Setsize applies ws to a pty, named by either of its ends.
func Setsize(f *os.File, ws *Winsize) error {
	return creackpty.Setsize(f, ws)
}

// GetsizeFull returns the window size of the pty f names, cells and pixels.
func GetsizeFull(f *os.File) (*Winsize, error) {
	return creackpty.GetsizeFull(f)
}

// Open returns a pty's master and its slave, which is always the peer of the
// master returned with it; see the package comment for the lookup that makes
// that worth saying.
//
// The slave's Name is the /dev/pts path its pty number names, which is what a
// caller that publishes the slave for another process to open hands over.
func Open() (*os.File, *os.File, error) {
	return open()
}
