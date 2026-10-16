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

//go:build !windows

package pty

import (
	"os"
	"os/exec"
	"syscall"
)

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
