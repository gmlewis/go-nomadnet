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

//go:build linux

package pty

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// open returns a pty's master and its own slave, which the kernel is asked for
// by peer rather than by name; see the package comment.
func open() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			_ = master.Close()
		}
	}()

	// A pty's slave is locked until its master unlocks it, and neither opening
	// it by name nor taking it as the peer works while it is.
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, fmt.Errorf("unlocking the pseudo-terminal's slave: %w", err)
	}

	// The slave's name is the pty number the master reports, and it is the name
	// a caller publishes for another process to open, so both routes below end
	// with the slave named that way.
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the pseudo-terminal's number: %w", err)
	}
	path := slavePath(number)

	fd, _, errno := unix.Syscall(unix.SYS_IOCTL, master.Fd(), unix.TIOCGPTPEER,
		uintptr(unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC))
	if errno == unix.ENOTTY {
		// A kernel older than 4.13 does not know TIOCGPTPEER, and there the name
		// is what a caller has: that lookup is the racy one, taken only where
		// the kernel leaves no other way.
		named, err := namedSlave(path)
		if err != nil {
			return nil, nil, err
		}
		return master, named, nil
	}
	if errno != 0 {
		return nil, nil, fmt.Errorf("taking the pseudo-terminal's peer: %w", errno)
	}
	return master, os.NewFile(fd, path), nil
}

// slavePath is the /dev/pts path that a master's pty number names.
func slavePath(number int) string {
	return fmt.Sprintf("/dev/pts/%d", number)
}

// namedSlave opens the slave of a master's pty number by name, which is all a
// kernel older than 4.13 leaves to name it by.
func namedSlave(path string) (*os.File, error) {
	slave, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("opening the pseudo-terminal's slave %v: %w", path, err)
	}
	return slave, nil
}
