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

//go:build !embedded && !pocket_communicator && !pocket_hub

package main

import (
	"os"

	"golang.org/x/term"
)

// hasTTY reports whether the process has a terminal on stdin. The tview TUI
// requires a terminal; without one, Application.Run() fails and gonomadnet
// exits silently with code 0, leaving the operator with no node and no error.
// Used to auto-fallback to daemon mode when launched without a terminal
// (e.g. via nohup, systemd, or a non-interactive SSH session).
func hasTTY() bool {
	return hasTTYFromFile(os.Stdin)
}

// hasTTYFromFile reports whether the given file is a terminal. Uses
// term.IsTerminal (ioctl-based) rather than os.File Stat ModeCharDevice
// because /dev/null IS a character device but is NOT a terminal. Separated
// from hasTTY so tests can inject non-stdin files.
func hasTTYFromFile(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
