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

//go:build integration && !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/pty"
	"github.com/gmlewis/go-reticulum/testutils"
)

// TestQuitGivesTheTerminalBack runs the real client on a terminal, quits it
// with Ctrl-Q, and then checks the terminal it left behind.
//
// The bytes the client sends on its way out must carry the mouse-off codes —
// after the last mouse-on code it sent while starting up; asserting only that
// they appear somewhere would be satisfied by the codes the client sends when it
// turns tracking on. And the settings the client leaves behind must be the
// cooked ones, or the shell's line editing silently stops working.
//
// The terminal is a pty, and the client runs on it under a shell, which is how
// a user meets it: the shell is what outlives the client, and it is what
// reports the terminal afterwards, with the same `stty -a` a user would type.
// The client cannot report its own terminal after the fact — on macOS a pty
// whose session leader has exited refuses to be interrogated at all, and its
// master answers EIO.
func TestQuitGivesTheTerminalBack(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)

	bin := buildDaemonBinary(t)
	cfgDir := tempDir(t)
	rnsDir := tempDir(t)

	// The shell runs the client, then reports the client's exit status and the
	// pty's line settings as the shell sees them.
	script := fmt.Sprintf("%v -t -config %v -rnsconfig %v\n"+
		"printf '\\nPROBE-EXIT %%s\\n' \"$?\"\n"+
		"stty -a\n"+
		"printf 'PROBE-END\\n'\n",
		bin, cfgDir, rnsDir)
	cmd := exec.Command("/bin/sh", "-c", script)
	// A fresh config dir has no identity yet, so the client spends its first
	// seconds creating one; the single-instance guard is bypassed because the
	// developer may well have nomadnet running for parity checks, and the fresh
	// config dir already isolates the lock.
	cmd.Env = append(os.Environ(),
		"GONOMADNET_IGNORE_RUNNING_NOMADNET=1",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		t.Fatalf("starting the client on a pty: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = master.Close()
	})

	out := newTerminalProbe(master)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	// Wait for the client to draw its first screen. The intro splash carries
	// the product name, so it is the earliest thing that proves the client has
	// the terminal.
	if !out.await("Nomad Network", 60*time.Second) {
		t.Fatalf("the client never drew its interface; saw %q", out.History())
	}
	// The intro splash is shown for a second before the main display replaces
	// it, and the quit key has to reach the client that is actually displayed.
	time.Sleep(2 * time.Second)
	out.Drain()

	// Ctrl-Q: the documented quit.
	if _, err := master.Write([]byte{0x11}); err != nil {
		t.Fatalf("sending Ctrl-Q: %v", err)
	}

	select {
	case <-exited:
	case <-time.After(30 * time.Second):
		t.Fatal("the client did not quit, and its shell never reported the terminal back, within 30s of Ctrl-Q")
	}
	// Give the pty's last bytes time to arrive before the reads below.
	time.Sleep(200 * time.Millisecond)

	all := out.History()
	lastEnable := strings.LastIndex(all, "\x1b[?1003h")
	if lastEnable < 0 {
		t.Fatalf("the client never enabled mouse tracking, so this test would pass vacuously; saw %q", all)
	}
	afterQuit := all[lastEnable:]
	// A failure quotes only the tail of it: a full-screen client writes
	// thousands of bytes here, and the point of the message is the last thing
	// the terminal was told before the shell got it back.
	tail := afterQuit
	if len(tail) > 200 {
		tail = tail[len(tail)-200:]
	}
	for _, want := range []string{"\x1b[?1000l", "\x1b[?1002l", "\x1b[?1003l", "\x1b[?1006l", "\x1b[?1049l", "\x1b[?25h"} {
		if !strings.Contains(afterQuit, want) {
			t.Errorf("the terminal never received %q after the client quit; a mouse moving over the "+
				"window would print escape-sequence garbage into the shell. The last bytes the client "+
				"sent after its last mouse-on code: %q", want, tail)
		}
	}

	// The shell's own report, from the terminal the client handed back.
	report, ok := between(all, "PROBE-EXIT", "PROBE-END")
	if !ok {
		t.Fatalf("the shell never reported the terminal the client left behind; saw %q", all)
	}
	status, err := strconv.Atoi(strings.Fields(strings.TrimPrefix(report, "PROBE-EXIT"))[0])
	if err != nil {
		t.Fatalf("the shell's report of the client's exit status is malformed: %q", report)
	}
	if status != 0 {
		t.Errorf("the client exited with status %v, want a clean exit. Report: %q", status, report)
	}
	for _, flag := range []string{"icanon", "echo", "isig"} {
		if !hasSttyFlag(report, flag) {
			t.Errorf("the terminal the client left behind has %v off; the shell's line editing does not "+
				"work there until the user runs `reset`. stty -a said: %q", flag, report)
		}
	}
}

// hasSttyFlag reports whether `stty -a` output lists an option as set. The
// tokens are whitespace-separated and an option that is off is written with a
// leading minus (-icanon), so the comparison is exact: `-echo` must not answer
// for `echo`.
func hasSttyFlag(sttyOutput, flag string) bool {
	for _, field := range strings.Fields(sttyOutput) {
		if strings.TrimRight(field, ":,;") == flag {
			return true
		}
	}
	return false
}

// between returns the text of what the client wrote between the last two
// markers, and whether both were found.
func between(all, from, to string) (string, bool) {
	start := strings.LastIndex(all, from)
	if start < 0 {
		return "", false
	}
	end := strings.Index(all[start:], to)
	if end < 0 {
		return "", false
	}
	return all[start : start+end], true
}

// terminalProbe reads what a client running on a pty has written, from one
// goroutine, so a probe can never hang the test and two probes can never race
// for the same bytes. Everything it reads is also kept, because the assertions
// are about the order of what the terminal received over the whole session —
// the mouse-on codes from start-up and the mouse-off codes from the quit — and
// a read that saw only the bytes arriving after the quit could not tell them
// apart.
type terminalProbe struct {
	chunks chan string

	mu      sync.Mutex
	history strings.Builder
}

// newTerminalProbe starts reading master. The goroutine ends when the read
// fails, which the test's cleanup arranges by closing the pty.
func newTerminalProbe(master *os.File) *terminalProbe {
	p := &terminalProbe{chunks: make(chan string, 256)}
	go func() {
		defer close(p.chunks)
		buf := make([]byte, 8192)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				p.chunks <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return p
}

// History returns everything the terminal has received so far, after taking in
// whatever has arrived since the last read.
func (p *terminalProbe) History() string {
	p.ReadWithin(300 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.history.String()
}

// Drain discards whatever the terminal has already produced, so the next read
// sees only what the client writes from now on.
func (p *terminalProbe) Drain() {
	p.ReadWithin(200 * time.Millisecond)
}

// ReadWithin returns what the terminal has received, giving up after d of
// silence. What it returns is also appended to the probe's history.
func (p *terminalProbe) ReadWithin(d time.Duration) string {
	var got strings.Builder
	deadline := time.After(d)
read:
	for {
		select {
		case chunk, ok := <-p.chunks:
			if !ok {
				break read
			}
			got.WriteString(chunk)
		case <-deadline:
			break read
		}
	}
	p.mu.Lock()
	p.history.WriteString(got.String())
	p.mu.Unlock()
	return got.String()
}

// await waits up to d for the client to write want anywhere in its output.
//
// The client draws incrementally, so the marker can be split across two reads;
// the bytes accumulate in the history rather than in the caller for that
// reason.
func (p *terminalProbe) await(want string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if strings.Contains(p.History(), want) {
			return true
		}
	}
	return false
}
