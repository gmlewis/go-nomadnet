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
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// terminalRestoreSeq is what the client writes to its terminal as the last act
// of its life, once tview's event loop has returned.
//
// tcell emits some of this from its own Fini, but the client must not depend on
// that: Fini is skipped by any exit that never reaches it, and the modes it
// turns off have to be turned off here for the promise to hold. A terminal left
// reporting the mouse prints one escape sequence into the shell for every
// movement of the mouse — the garbage a user sees in the window where the
// client has just quit — and a terminal left in raw mode swallows the shell's
// line editing, so the user has to run `reset` before the window is usable
// again.
//
// Every code here turns a mode OFF. Each covers a way the terminal can have
// been left changed: the three generations of mouse tracking (1000, 1002,
// 1003) and the three coordinate encodings (1005, 1006, 1015), focus reporting
// (1004), bracketed paste (2004), and the keyboard protocols tcell enables on
// an xterm-like terminal (win32 input mode, the kitty CSI-u form, and xterm's
// modifyOtherKeys). The rest hand the screen itself back: synchronized output
// off, so a terminal that was interrupted between the start and the end of a
// frame is not left holding every later update back like a frozen screen;
// automatic margin (wraparound) on, the keypad and cursor keys back in their
// ordinary modes so the shell's arrow keys do not emit escape garbage, out of
// the alternate screen, the cursor visible, and the character attributes reset.
// Unknown codes are ignored by terminals that do not implement them, so one
// sequence serves every terminal the client runs on.
const terminalRestoreSeq = "" +
	"\x1b[?2026l" + // synchronized output off (an interrupted frame would freeze the screen)
	"\x1b[?1000l" + // mouse: normal tracking off
	"\x1b[?1002l" + // mouse: button-event (drag) tracking off
	"\x1b[?1003l" + // mouse: any-event (motion) tracking off
	"\x1b[?1005l" + // mouse: UTF-8 coordinates off
	"\x1b[?1006l" + // mouse: SGR coordinates off
	"\x1b[?1015l" + // mouse: urxvt coordinates off
	"\x1b[?1004l" + // focus reporting off
	"\x1b[?2004l" + // bracketed paste off
	"\x1b[?9001l" + // win32 input mode off
	"\x1b[<u" + //     kitty keyboard protocol: pop
	"\x1b[>4;0m" + //  xterm modifyOtherKeys: reset
	"\x1b[?7h" + //    automatic margin (wraparound) back on
	"\x1b[?1l" + //    cursor keys back to their ordinary (non-application) codes
	"\x1b>" + //       keypad back to its ordinary mode
	"\x1b[?1049l" + // leave the alternate screen, restoring the primary one
	"\x1b[?25h" + //   cursor visible
	"\x1b[0m" //       character attributes reset

// TerminalRestoreSeq returns the sequence the client writes to its terminal as
// the last act of its life: every mode the client can have turned on, turned
// off, the alternate screen left, the cursor shown, the character attributes
// reset. See terminalRestoreSeq for what each code is for.
//
// It is exported for the launcher's own backstop. gonomadnet.sh runs the client
// in the foreground, so its EXIT trap is the only thing left to clean up after
// a death that runs no code of the client's own — a panic in a goroutine the
// client does not wrap (go-reticulum's transports and timers), or a SIGKILL —
// and it has to emit exactly these codes. A test keeps the two from drifting
// apart; neither may be changed without the other.
func TerminalRestoreSeq() string { return terminalRestoreSeq }

// restoreWriter, when set, is the terminal the client's final restore is
// written to instead of the one the process itself owns. It is read on the way
// out of the process, under restoreWriterMu.
var (
	restoreWriterMu sync.Mutex
	restoreWriter   io.Writer
)

// SetTerminalRestoreWriter registers the stream the client's final terminal
// restore is written to, or clears the registration when given nil.
//
// A desktop client needs nothing here: its terminal is the process's
// controlling terminal, which the restore opens for itself. An appliance whose
// client draws into a terminal it built in-process — iOS, where the process has
// no controlling terminal to open — registers the same tcell.Tty it handed the
// screen, and the restore then reaches the console through it.
//
// It must be called before the event loop starts, and it is read only when the
// client is already on its way out.
func SetTerminalRestoreWriter(w io.Writer) {
	restoreWriterMu.Lock()
	restoreWriter = w
	restoreWriterMu.Unlock()
}

// terminalRestoreWriter returns the registered restore stream, or nil when the
// client owns a terminal of its own.
func terminalRestoreWriter() io.Writer {
	restoreWriterMu.Lock()
	defer restoreWriterMu.Unlock()
	return restoreWriter
}

// openTerminal opens the process's controlling terminal for the final restore,
// or returns nil when the process has none: an appliance that draws into a
// terminal it built itself, a Windows console, a test.
//
// It is the same terminal tcell opens to draw on, and it is opened afresh — the
// restore happens long after tcell has closed its own handle to it.
func openTerminal() *os.File {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return f
}

// captureTerminalState records the terminal's line settings as they are before
// the event loop starts, so the final restore can put cooked mode back even on
// an exit path that never reaches tcell's own tty.Stop.
//
// Run calls it before handing the terminal to tcell, because that is the last
// moment at which cooked mode can still be observed: tcell puts the terminal
// into raw mode when the application starts. A client with no terminal of its
// own returns nil and restores no settings.
func captureTerminalState() *term.State {
	f := openTerminal()
	if f == nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	state, err := term.GetState(int(f.Fd()))
	if err != nil {
		return nil
	}
	return state
}

// restoreTerminalTo writes the terminal restore sequence to w and, when f names
// a terminal whose settings were recorded before the event loop started, puts
// those settings back.
//
// It reports nothing and never panics: it is the last thing a client does, and
// the process may already be unwinding from a panic. The terminal has to be
// usable either way.
func restoreTerminalTo(w io.Writer, f *os.File, state *term.State) {
	if w != nil {
		_, _ = io.WriteString(w, terminalRestoreSeq)
	}
	if f != nil && state != nil {
		_ = term.Restore(int(f.Fd()), state)
	}
}

// restoreTerminal gives the terminal back to the shell it was borrowed from:
// mouse tracking, focus reporting, bracketed paste and the keyboard protocols
// off, the alternate screen left, the cursor visible, the attributes reset, and
// cooked line settings restored from the state captured before the event loop
// started.
//
// The write goes to the registered restore stream when there is one, else to
// the process's controlling terminal, else to standard output — the last being
// a terminal-less process, where the escape codes are inert.
func restoreTerminal(state *term.State) {
	f := openTerminal()
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()

	w := terminalRestoreWriter()
	switch {
	case w != nil:
	case f != nil:
		w = f
	default:
		w = os.Stdout
	}
	restoreTerminalTo(w, f, state)
}
