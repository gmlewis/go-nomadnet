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

// This file is the logger a page plugin reaches through the rns.log host
// import. It compiles in every build so the package's API does not depend on
// the wago tag; without that tag no page runs and the logger is never called.

package wasmpages

import "sync"

// logMu guards logFunc, which an embedding tool may replace while pages render
// concurrently.
var logMu sync.Mutex

// logFunc receives the messages a page plugin emits through rns.log. It is nil
// until SetLogFunc installs one, in which case guest log messages are dropped.
var logFunc func(format string, args ...any)

// SetLogFunc installs the logger that page plugins reach through the rns.log
// host import. A nil function drops guest log messages.
func SetLogFunc(fn func(format string, args ...any)) {
	logMu.Lock()
	logFunc = fn
	logMu.Unlock()
}

// guestLogger returns the currently installed logger.
func guestLogger() func(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	return logFunc
}
