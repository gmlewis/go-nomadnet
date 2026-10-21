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

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleCP     = kernel32.NewProc("GetConsoleCP")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
)

// callProc invokes a lazily-loaded Win32 procedure and reports an error when
// the export cannot be found.
//
// A LazyProc resolves its export on first Call and panics when the procedure
// does not exist, which is not acceptable in a diagnostic tool: one missing
// API would abort the run before it printed anything. Find is therefore called
// first, and the lookup error is returned instead of raised.
func callProc(proc *windows.LazyProc, args ...uintptr) (uintptr, error) {
	if err := proc.Find(); err != nil {
		return 0, err
	}
	r, _, _ := proc.Call(args...)
	return r, nil
}

// noteUnavailable records a Win32 procedure that could not be resolved, so the
// report can say a fact is unknown rather than reporting a default as a
// measurement.
func noteUnavailable(c *ConsoleFacts, dll, name string, err error) {
	c.UnavailableAPIs = append(c.UnavailableAPIs, fmt.Sprintf("%v!%v: %v", dll, name, err))
}

// consoleProbe is the result of opening one console device name and asking it
// about itself.
type consoleProbe struct {
	opens     bool
	mode      uint32
	modeKnown bool
	info      windows.ConsoleScreenBufferInfo
	infoKnown bool
	errText   string
}

// inspectConsole opens a console device name such as CONIN$ or CONOUT$ and
// reads its mode and screen-buffer information. It changes nothing: the handle
// is opened, queried, and closed without a single SetConsoleMode call.
func inspectConsole(name string) consoleProbe {
	var p consoleProbe
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		p.errText = err.Error()
		return p
	}
	defer func() { _ = f.Close() }()
	p.opens = true

	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err == nil {
		p.mode, p.modeKnown = mode, true
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err == nil {
		p.info, p.infoKnown = info, true
	}
	return p
}

// gatherConsole reads the Windows console state without changing it. The
// process is a native Windows program: even when it was launched from mintty
// it talks to CONIN$/CONOUT$, not to the pty, so those handles are where the
// truth about the terminal lives.
func gatherConsole() *ConsoleFacts {
	c := &ConsoleFacts{}

	c.StdinMode, c.StdinIsConsole = consoleMode(os.Stdin)
	c.StdoutMode, c.StdoutIsConsole = consoleMode(os.Stdout)
	c.StderrMode, c.StderrIsConsole = consoleMode(os.Stderr)
	c.StdinType = handleType(os.Stdin)
	c.StdoutType = handleType(os.Stdout)
	c.StderrType = handleType(os.Stderr)

	in := inspectConsole("CONIN$")
	c.ConInOpens = in.opens
	c.ConInMode, c.ConInModeKnown = in.mode, in.modeKnown
	c.ConInError = in.errText

	out := inspectConsole("CONOUT$")
	c.ConOutOpens = out.opens
	c.ConOutMode, c.ConOutModeKnown = out.mode, out.modeKnown
	c.ConOutError = out.errText
	if out.infoKnown {
		c.BufferInfo = true
		c.BufferWidth = int(out.info.Size.X)
		c.BufferHeight = int(out.info.Size.Y)
		c.CursorX = int(out.info.CursorPosition.X)
		c.CursorY = int(out.info.CursorPosition.Y)
		c.Attributes = out.info.Attributes
		c.WindowWidth = int(out.info.Window.Right-out.info.Window.Left) + 1
		c.WindowHeight = int(out.info.Window.Bottom-out.info.Window.Top) + 1
		c.MaxWindowWidth = int(out.info.MaximumWindowSize.X)
		c.MaxWindowHeight = int(out.info.MaximumWindowSize.Y)
	}

	c.Attached = c.StdinIsConsole || c.StdoutIsConsole || c.StderrIsConsole ||
		c.ConInOpens || c.ConOutOpens

	hwnd, err := callProc(procGetConsoleWindow)
	switch {
	case err != nil:
		noteUnavailable(c, "kernel32", "GetConsoleWindow", err)
	case hwnd != 0:
		c.ConsoleWindow = hwnd
		// A console window that is not visible means the process was given a
		// console that is not the window the user is looking at: mintty's
		// relay, or a ConPTY. It is the strongest single sign that tcell's
		// output has a translation layer in front of it. Visibility is only
		// known when the call succeeds, so an unknown value is never reported
		// as "hidden".
		visible, err := callProc(procIsWindowVisible, hwnd)
		if err != nil {
			noteUnavailable(c, "user32", "IsWindowVisible", err)
		} else {
			c.ConsoleVisible, c.ConsoleVisibleKnown = visible != 0, true
		}
	}
	if cp, err := callProc(procGetConsoleCP); err != nil {
		noteUnavailable(c, "kernel32", "GetConsoleCP", err)
	} else if cp != 0 {
		c.InputCP = uint32(cp)
	}
	if cp, err := windows.GetConsoleOutputCP(); err == nil {
		c.OutputCP = cp
	}
	return c
}

// consoleMode returns a file's console mode and whether it is a console.
// GetConsoleMode is the authoritative test: a NUL device is also a character
// device, but it has no console mode.
func consoleMode(f *os.File) (uint32, bool) {
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		return 0, false
	}
	return mode, true
}

// handleType names the kind of handle, which distinguishes a console from a
// redirected file or a pipe when the console mode is unavailable.
func handleType(f *os.File) string {
	t, err := windows.GetFileType(windows.Handle(f.Fd()))
	if err != nil {
		return "unknown"
	}
	switch t {
	case windows.FILE_TYPE_CHAR:
		return "char device"
	case windows.FILE_TYPE_DISK:
		return "file"
	case windows.FILE_TYPE_PIPE:
		return "pipe"
	default:
		return "unknown"
	}
}
