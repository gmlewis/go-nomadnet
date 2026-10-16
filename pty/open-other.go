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

//go:build !linux

package pty

import (
	"os"

	creackpty "github.com/creack/pty/v2"
)

// open returns a pty's master and its slave.
//
// Only Linux recycles the node a master has just given up the way the package
// comment describes, so everywhere else the pty library's own lookup is used:
// here the name a master reports is its own slave.
func open() (*os.File, *os.File, error) {
	return creackpty.Open()
}
