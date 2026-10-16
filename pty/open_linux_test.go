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
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// TestNamedSlaveReturnsTheMasterName covers the slave a kernel without
// TIOCGPTPEER is handed: no kernel in service here is old enough to take that
// path, so it is exercised on its own.
func TestNamedSlaveReturnsTheMasterName(t *testing.T) {
	t.Parallel()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening a pty's master: %v", err)
	}
	defer func() { _ = master.Close() }()
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatalf("unlocking the pty's slave: %v", err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatalf("reading the pty's number: %v", err)
	}

	slave, err := namedSlave(slavePath(number))
	if err != nil {
		t.Fatalf("namedSlave returned an error: %v", err)
	}
	defer func() { _ = slave.Close() }()

	requireSlaveIsTheMastersPeer(t, master, slave)
}
