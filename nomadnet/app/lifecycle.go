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

package app

import (
	"os"
	"runtime/debug"
)

// ExitHandler performs graceful cleanup before exiting: it stops background
// jobs, saves the RRC manager, and shuts it down. This mirrors the Python
// NomadNet exit_handler (terminal-restoration steps are handled by the TUI).
func (a *App) ExitHandler() {
	a.mu.Lock()
	a.ShouldRunJobs = false
	a.mu.Unlock()

	if a.Logger != nil {
		a.Logger.Notice("Saving directory...")
	}

	if a.RRC != nil {
		if err := a.RRC.Save(); err != nil && a.Logger != nil {
			a.Logger.Error("Could not save RRC state: %v", err)
		}
		a.RRC.Shutdown()
	}

	if a.Logger != nil {
		a.Logger.Notice("Nomad Network Client exiting now")
	}
}

// Quit performs graceful cleanup and exits the process, mirroring the Python
// NomadNet quit.
func (a *App) Quit() {
	if a.Logger != nil {
		a.Logger.Notice("Nomad Network Client shutting down...")
	}
	a.ExitHandler()
	os.Exit(0)
}

// ExceptionHandler logs an unhandled panic's details. When the panic is a
// KeyboardInterrupt equivalent (os.Interrupt), it re-raises it. This mirrors the
// Python NomadNet exception_handler.
func (a *App) ExceptionHandler(r any) {
	if a.Logger == nil {
		return
	}
	a.Logger.Error("An unhandled panic occurred:")
	a.Logger.Error("Value: %v", r)
	a.Logger.Error("Trace:\n%s", debug.Stack())
}
