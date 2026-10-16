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
// re-exported here so that a caller needs one import rather than two.
package pty

import (
	"os"
	"os/exec"
	"syscall"

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

// StartWithSize gives cmd a fresh pty, starts it on the slave, and returns the
// master.
//
// The child gets a session of its own and the slave as its controlling terminal,
// which is the whole point: term.IsTerminal(0) is what makes a program take its
// terminal-UI path instead of falling back to a non-interactive one. Because the
// kernel owns the terminal, the child is never signalled by hand — a window size
// change is an ioctl, and the kernel delivers the SIGWINCH.
//
// The command's three standard descriptors are the slave unless the caller has
// already set them, and ws is applied before the child is started. A nil ws
// leaves the pty at the kernel's default size.
func StartWithSize(cmd *exec.Cmd, ws *Winsize) (*os.File, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true

	master, slave, err := Open()
	if err != nil {
		return nil, err
	}
	if ws != nil {
		if err := Setsize(master, ws); err != nil {
			_ = slave.Close()
			_ = master.Close()
			return nil, err
		}
	}
	if cmd.Stdout == nil {
		cmd.Stdout = slave
	}
	if cmd.Stderr == nil {
		cmd.Stderr = slave
	}
	if cmd.Stdin == nil {
		cmd.Stdin = slave
	}
	// Ctty is an index into the command's standard descriptors, not a file
	// descriptor, so the controlling terminal is the slot the slave landed in —
	// which is not stdin when the caller set one of its own.
	switch {
	case cmd.Stdin == slave:
		cmd.SysProcAttr.Ctty = 0
	case cmd.Stdout == slave:
		cmd.SysProcAttr.Ctty = 1
	case cmd.Stderr == slave:
		cmd.SysProcAttr.Ctty = 2
	}
	if err := cmd.Start(); err != nil {
		_ = slave.Close()
		_ = master.Close()
		return nil, err
	}
	// The child holds its own descriptors for the slave from here on.
	_ = slave.Close()
	return master, nil
}
