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

//go:build embedded || pocket_communicator || pocket_hub

package main

import (
	"os"
)

// hasTTY always returns false on embedded platforms.
func hasTTY() bool {
	return false
}

// hasTTYFromFile always returns false on embedded platforms.
func hasTTYFromFile(f *os.File) bool {
	return false
}
