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

//go:build windows

package pty

import (
	"os"
	"os/exec"

	creackpty "github.com/creack/pty/v2"
)

// StartWithSize reports that the command cannot be started on a pty here: a
// Windows console has no pseudo-terminals, so there is no terminal to give it.
//
// The answer is the pty library's own error value, which is what a caller saw
// before this package existed and what its error path already reports.
func StartWithSize(cmd *exec.Cmd, ws *Winsize) (*os.File, error) {
	return nil, creackpty.ErrUnsupported
}
