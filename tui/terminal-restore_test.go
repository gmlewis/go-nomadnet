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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/pty"
	"golang.org/x/term"
)

// TestTerminalRestoreSeqCoversEveryModeTheClientTurnsOn pins the sequence a
// client writes as the last act of its life.
//
// The failure it guards against is the one a user reported: quit the client,
// move the mouse, and the window fills with escape-sequence garbage, because
// the terminal was still reporting mouse events to the shell. So every mouse
// mode tcell can enable must have its "off" code here, along with the other
// modes that make a terminal unusable when they outlive their program, and the
// sequence must contain no enable code at all — an enable here would turn
// tracking back on during the restore.
func TestTerminalRestoreSeqCoversEveryModeTheClientTurnsOn(t *testing.T) {
	t.Parallel()

	// Every mode the client turns on, and the code that turns it off.
	modes := []struct {
		name  string
		on    string
		off   string
		notes string
	}{
		{"mouse: normal tracking", "\x1b[?1000h", "\x1b[?1000l", "clicks"},
		{"mouse: button-event (drag)", "\x1b[?1002h", "\x1b[?1002l", "drags"},
		{"mouse: any-event (motion)", "\x1b[?1003h", "\x1b[?1003l", "motion"},
		{"mouse: SGR coordinates", "\x1b[?1006h", "\x1b[?1006l", "the encoding of every mouse event"},
		{"mouse: UTF-8 coordinates", "\x1b[?1005h", "\x1b[?1005l", "the older encoding"},
		{"mouse: urxvt coordinates", "\x1b[?1015h", "\x1b[?1015l", "the other older encoding"},
		{"focus reporting", "\x1b[?1004h", "\x1b[?1004l", "focus in/out events"},
		{"bracketed paste", "\x1b[?2004h", "\x1b[?2004l", "paste markers"},
		{"synchronized output", "\x1b[?2026h", "\x1b[?2026l", "a frozen screen: every later update is held back"},
		{"automatic margin", "\x1b[?7l", "\x1b[?7h", "wraparound"},
		{"application cursor keys", "\x1b[?1h", "\x1b[?1l", "the shell's arrow keys, which would emit escape garbage"},
		{"application keypad", "\x1b=", "\x1b>", "the keypad"},
		{"alternate screen", "\x1b[?1049h", "\x1b[?1049l", "the screen the client drew on"},
		{"hidden cursor", "\x1b[?25l", "\x1b[?25h", "the cursor"},
		{"attributes", "", "\x1b[0m", "colours and other attributes"},
		{"win32 input mode", "\x1b[?9001h", "\x1b[?9001l", "the keyboard protocol"},
		{"kitty keyboard protocol", "\x1b[>1u", "\x1b[<u", "the keyboard protocol"},
	}
	for _, m := range modes {
		if !strings.Contains(terminalRestoreSeq, m.off) {
			t.Errorf("the restore sequence does not turn off %v (%q is missing); the terminal would keep %v after the client exits",
				m.name, m.off, m.notes)
		}
		if m.on != "" && strings.Contains(terminalRestoreSeq, m.on) {
			t.Errorf("the restore sequence turns %v ON (%q is present); the restore must only ever turn modes off", m.name, m.on)
		}
	}

	// The sequence is written verbatim to a terminal, so an accidental literal
	// newline would print a blank line into the shell the user returns to.
	if strings.ContainsAny(terminalRestoreSeq, "\r\n") {
		t.Errorf("the restore sequence contains a newline: %q", terminalRestoreSeq)
	}
	if !strings.Contains(terminalRestoreSeq, "\x1b[") {
		t.Errorf("the restore sequence is not a sequence of escape codes: %q", terminalRestoreSeq)
	}
}

// TestRestoreTerminalLeavesAPtyCookedAndMouseFree is the whole guarantee, on a
// real terminal: after a client restores, the pty has been told to stop
// reporting the mouse and it is back in cooked mode.
//
// The mouse half is read back from the master — what the terminal emulator
// would have received — and the cooked-mode half is measured the way a user
// feels it: a pty in canonical mode with echo on echoes what is typed, and one
// left in raw mode swallows it. Both halves are asserted, because a client that
// restores only one of them still leaves a window that is unusable after it
// exits. The echo probe runs twice before the restore — on a fresh pty and in
// raw mode — as its own positive and negative control, so a passing test cannot
// be a probe that never worked.
func TestRestoreTerminalLeavesAPtyCookedAndMouseFree(t *testing.T) {
	master, slave, err := ptyPair(t)
	if err != nil {
		t.Skipf("no pty on this machine: %v", err)
	}
	out := newPtyReader(t, master)

	if !ptyEchoes(master, out) {
		t.Fatal("a fresh pty did not echo a typed character; the cooked-mode probe itself is broken")
	}

	// A client that has taken the terminal: raw mode, mouse tracking on.
	state, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatalf("reading the pty's initial state: %v", err)
	}
	if _, err := term.MakeRaw(int(slave.Fd())); err != nil {
		t.Fatalf("putting the pty into raw mode: %v", err)
	}
	if ptyEchoes(master, out) {
		t.Fatal("a pty in raw mode still echoed a typed character; the raw-mode probe itself is broken")
	}
	_, _ = master.WriteString("\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h")
	out.Drain()

	// The client exits.
	restoreTerminalTo(slave, slave, state)

	got := out.ReadWithin(250 * time.Millisecond)
	for _, want := range []string{"\x1b[?1000l", "\x1b[?1002l", "\x1b[?1003l", "\x1b[?1006l", "\x1b[?1049l", "\x1b[?25h"} {
		if !strings.Contains(got, want) {
			t.Errorf("the pty never received %q; the terminal would keep reporting to the shell. got %q", want, got)
		}
	}
	if !ptyEchoes(master, out) {
		t.Error("the pty did not echo after the restore: it was left in raw mode, " +
			"where the shell's line editing does not work")
	}
}

// TestRestoreTerminalUsesTheRegisteredWriter checks the seam an appliance uses
// to give the client a terminal that is not a file descriptor.
//
// iOS is the case: the process has no controlling terminal, so the restore has
// nowhere of its own to write. A registered writer must take precedence, or the
// console an iOS user is looking at would never be told to stop reporting the
// mouse.
func TestRestoreTerminalUsesTheRegisteredWriter(t *testing.T) {
	// Not parallel: the registered writer is package state, and every test in
	// this package that restores a terminal reads it.
	t.Cleanup(func() { SetTerminalRestoreWriter(nil) })

	var buf strings.Builder
	SetTerminalRestoreWriter(&buf)

	restoreTerminalTo(terminalRestoreWriter(), nil, nil)

	if got := buf.String(); got != terminalRestoreSeq {
		t.Errorf("the registered writer received %q, want the restore sequence %q", got, terminalRestoreSeq)
	}
}

// TestAppRestoreTerminalIsIdempotent checks that the several paths that can end
// a session — the menu's Quit, Ctrl-Q, a signal, a panic in a background
// goroutine — can all ask for the terminal back.
//
// The restore runs a sync.Once, so the later callers are no-ops rather than a
// second write racing the first. What must not happen is a restore that never
// runs because an earlier caller got there first.
func TestAppRestoreTerminalIsIdempotent(t *testing.T) {
	t.Cleanup(func() { SetTerminalRestoreWriter(nil) })

	var buf strings.Builder
	SetTerminalRestoreWriter(&buf)

	app := NewApp(ThemeDark, "unicode", ColorModeTrue)
	app.RestoreTerminal()
	app.RestoreTerminal()
	app.RestoreTerminal()

	if got := strings.Count(buf.String(), terminalRestoreSeq); got != 1 {
		t.Errorf("the restore sequence was written %v times, want exactly once", got)
	}
}

// ptyReader delivers what a pty master produces.
//
// It reads on one goroutine that lives as long as the pty does, rather than
// giving each probe its own reader: two goroutines on one master would race for
// the bytes, and a probe reading after a drain could swallow bytes meant for
// the next one. Every read the tests make goes through this type, with a
// deadline — os.File deadlines are not honored on a pty master on macOS, so the
// deadline is the reader's own and a quiet terminal can never hang a test.
type ptyReader struct {
	chunks chan string
}

// newPtyReader starts reading master. The goroutine ends when the read fails,
// which the test's cleanup arranges by closing the pty.
func newPtyReader(t *testing.T, master *os.File) *ptyReader {
	t.Helper()
	r := &ptyReader{chunks: make(chan string, 64)}
	go func() {
		defer close(r.chunks)
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				r.chunks <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return r
}

// Drain reads and discards whatever the pty has already produced, so the next
// ReadWithin sees only what the code under test wrote from now on.
func (r *ptyReader) Drain() {
	r.ReadWithin(100 * time.Millisecond)
}

// ReadWithin returns what the pty produced, giving up after d of silence.
func (r *ptyReader) ReadWithin(d time.Duration) string {
	var got strings.Builder
	deadline := time.After(d)
	for {
		select {
		case chunk, ok := <-r.chunks:
			if !ok {
				return got.String()
			}
			got.WriteString(chunk)
		case <-deadline:
			return got.String()
		}
	}
}

// ptyEchoes reports whether a character written to the master comes back: a
// terminal in canonical mode echoes what is typed, and one left in raw mode
// does not. It is how a user experiences the difference, and it needs no ioctl.
func ptyEchoes(master *os.File, out *ptyReader) bool {
	if _, err := master.WriteString("x"); err != nil {
		return false
	}
	return strings.Contains(out.ReadWithin(200*time.Millisecond), "x")
}

// ptyPair opens a pseudo-terminal and returns its master and slave halves.
// The pair is closed with the test, which is also what releases the reader.
func ptyPair(t *testing.T) (master, slave *os.File, err error) {
	t.Helper()
	master, slave, err = pty.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("opening a pty: %w", err)
	}
	t.Cleanup(func() {
		_ = master.Close()
		_ = slave.Close()
	})
	return master, slave, nil
}
