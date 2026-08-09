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

package tui

import (
	"fmt"
	"log"
	"os"
)

// mouseDebug is an optional file logger for the mouse-wheel dispatch path. It
// is enabled by setting GONOMADNET_MOUSE_DEBUG to a file path before launch,
// e.g.  `GONOMADNET_MOUSE_DEBUG=/tmp/mouse.log gonomadnet -textui`. The TUI owns
// the terminal, so debug output must go to a FILE (never stderr/stdout, which
// would corrupt the display). When the env var is unset, mouseDebug is nil and
// dbgMouse is a no-op — zero overhead in normal runs and tests.
//
// It exists to diagnose a live-only symptom: the user reports the mouse wheel
// over the Network "Announce Stream" list does not scroll a long list, yet the
// same path scrolls correctly headless through the full app root. The
// instrumentation in IndicativeListBox.MouseHandler and pileFiller.MouseHandler
// logs which primitive receives each wheel event, whether InRect passes, the
// current offset/item count, and the consumed result — enough to localize
// whether the event never reaches the list, reaches it but InRect bails, or
// reaches it but the list fits the viewport.
var mouseDebug *log.Logger

func init() {
	path := os.Getenv("GONOMADNET_MOUSE_DEBUG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// A bad path is a debug-setup mistake, not a reason to crash the TUI;
		// leave the logger disabled so the app runs normally.
		return
	}
	mouseDebug = log.New(f, "", log.Ltime|log.Lmicroseconds)
	mouseDebug.Printf("mouse debug log opened (pid=%v)", os.Getpid())
}

// dbgMouse logs a mouse-dispatch diagnostic line when the debug logger is
// enabled, and does nothing otherwise. The %v convention (not %d/%s) matches
// the repo debug style.
func dbgMouse(format string, args ...any) {
	if mouseDebug != nil {
		_ = mouseDebug.Output(3, fmt.Sprintf(format, args...))
	}
}
