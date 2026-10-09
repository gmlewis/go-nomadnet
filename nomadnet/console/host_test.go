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

package console

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The tests below run real, short-lived children on a real PTY, because the
// properties they assert — a controlling terminal, a window size the kernel
// agrees with, a SIGWINCH the child notices — are the kernel's, not this
// package's. They skip rather than fail when the machine lacks a program they
// need, so the package still tests itself on a stripped-down host.

// sessionTimeout bounds every wait in this file. It is a safety net against a
// hung session, never a delay any passing test waits out.
const sessionTimeout = 15 * time.Second

// requireShell skips a test that needs a real child to run.
func requireShell(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("this test runs a real child and needs /bin/sh: %v", err)
	}
}

// requireProgram skips a test that needs a program on the PATH.
func requireProgram(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("this test needs %v on the PATH: %v", name, err)
	}
}

// consoleHarness is the appliance's end of a session: it speaks the frame
// protocol over an in-memory socket and records what the host sends back.
type consoleHarness struct {
	t     *testing.T
	sess  *Session
	local net.Conn

	done chan error

	exitCh chan byte

	// mu guards everything below it. runErr is what Run returned, recorded as
	// soon as it returns so a test that failed earlier can still say why.
	mu       sync.Mutex
	output   bytes.Buffer
	runErr   error
	ran      bool
	consumed bool
}

// startHarness starts a session over net.Pipe, so a test drives exactly what
// the app would.
func startHarness(t *testing.T, cfg Config) *consoleHarness {
	t.Helper()
	local, remote := net.Pipe()
	sess, err := NewSession(cfg, remote)
	if err != nil {
		t.Fatalf("NewSession(%+v) returned an error: %v", cfg, err)
	}
	h := &consoleHarness{
		t:      t,
		sess:   sess,
		local:  local,
		done:   make(chan error, 1),
		exitCh: make(chan byte, 1),
	}
	go func() {
		err := sess.Run(context.Background())
		h.mu.Lock()
		h.runErr = err
		h.mu.Unlock()
		h.done <- err
	}()
	go h.readFrames()

	t.Cleanup(func() {
		_ = local.Close()
		if h.endingAsserted() {
			return
		}
		// The test did not assert how the session ended, so this is the only
		// place that can: a session that fails for an unexpected reason must
		// not look like a silent success.
		if err, ok := h.awaitRun(); !ok {
			t.Error("the session did not return after its connection closed")
		} else if err != nil {
			t.Errorf("the session returned %v, want nil", err)
		}
	})
	return h
}

// endingAsserted reports whether a test has already asserted the session's end.
func (h *consoleHarness) endingAsserted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.consumed
}

// sessionEnd describes how the session ended, for a diagnostic.
func (h *consoleHarness) sessionEnd() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case !h.ran:
		return "the session is still running"
	case h.runErr != nil:
		return fmt.Sprintf("the session had already failed with %v", h.runErr)
	default:
		return "the session had already ended cleanly"
	}
}

// readFrames is the app's reader: it accumulates terminal output and records
// the exit frame.
func (h *consoleHarness) readFrames() {
	for {
		f, err := ReadFrame(h.local)
		if err != nil {
			return
		}
		switch f.Type {
		case FrameData:
			h.mu.Lock()
			h.output.Write(f.Data)
			h.mu.Unlock()
		case FrameExit:
			h.exitCh <- f.Code
			return
		}
	}
}

// send writes one keystroke-or-line to the child as a DATA frame.
func (h *consoleHarness) send(text string) {
	h.t.Helper()
	if err := h.local.SetWriteDeadline(time.Now().Add(sessionTimeout)); err != nil {
		h.t.Fatalf("setting a write deadline: %v", err)
	}
	if err := WriteFrame(h.local, DataFrame([]byte(text))); err != nil {
		h.t.Fatalf("sending %q: %v", text, err)
	}
}

// resize sends a window-size change, as a rotation would.
func (h *consoleHarness) resize(cols, rows uint16) {
	h.t.Helper()
	if err := h.local.SetWriteDeadline(time.Now().Add(sessionTimeout)); err != nil {
		h.t.Fatalf("setting a write deadline: %v", err)
	}
	if err := WriteFrame(h.local, ResizeFrame(cols, rows)); err != nil {
		h.t.Fatalf("sending a %vx%v resize: %v", cols, rows, err)
	}
}

// waitFor waits until the terminal has shown substr and returns everything it
// has shown.
func (h *consoleHarness) waitFor(substr string) string {
	h.t.Helper()
	deadline := time.Now().Add(sessionTimeout)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		seen := h.output.String()
		h.mu.Unlock()
		if strings.Contains(seen, substr) {
			return seen
		}
		time.Sleep(2 * time.Millisecond)
	}
	seen := h.sessionEnd()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.t.Fatalf("the terminal never showed %q; it showed:\n%v\n(%v)", substr, h.output.String(), seen)
	return ""
}

// waitForExit waits for the session's exit frame and returns the code.
func (h *consoleHarness) waitForExit() byte {
	h.t.Helper()
	select {
	case code := <-h.exitCh:
		return code
	case <-time.After(sessionTimeout):
		h.t.Fatalf("the session never sent an exit frame (%v)", h.sessionEnd())
		return 0
	}
}

// awaitRun waits for Run to return, once.
func (h *consoleHarness) awaitRun() (error, bool) {
	select {
	case err := <-h.done:
		h.mu.Lock()
		h.ran, h.consumed = true, true
		h.mu.Unlock()
		return err, true
	case <-time.After(sessionTimeout):
		h.mu.Lock()
		h.consumed = true
		h.mu.Unlock()
		return nil, false
	}
}

// waitForCleanEnd waits for the session to end and asserts that it ended
// without an error.
func (h *consoleHarness) waitForCleanEnd() {
	h.t.Helper()
	if err := h.waitForRun(); err != nil {
		h.t.Errorf("Run returned %v, want nil", err)
	}
}

// waitForRun waits for the session to end, failing the test if it does not.
func (h *consoleHarness) waitForRun() error {
	h.t.Helper()
	err, ok := h.awaitRun()
	if !ok {
		h.t.Fatal("the session did not return")
	}
	return err
}

func TestHostRunsAChildOnAPTYYieldingARealTerminal(t *testing.T) {
	t.Parallel()
	requireShell(t)

	// cmd/gonomadnet takes its terminal-UI path only when its standard input is
	// a terminal, so this is the property the whole design exists to produce.
	h := startHarness(t, Config{
		Command: "/bin/sh",
		Args:    []string{"-c", `test -t 0 && test -t 1 && test -t 2 && echo ISTTY || echo NOTTY`},
	})
	seen := h.waitFor("ISTTY")
	if strings.Contains(seen, "NOTTY") {
		t.Errorf("the child did not see a terminal on stdin, stdout and stderr:\n%v", seen)
	}
	h.waitForCleanEnd()
}

func TestHostReportsTheWindowSizeToTheChild(t *testing.T) {
	t.Parallel()
	requireShell(t)
	requireProgram(t, "stty")

	h := startHarness(t, Config{Command: "/bin/sh", Args: []string{"-c", "stty size"}, Cols: 80, Rows: 24})
	h.waitFor("24 80")
	h.waitForCleanEnd()
}

func TestHostResizesTheChild(t *testing.T) {
	t.Parallel()
	requireShell(t)
	requireProgram(t, "stty")

	h := startHarness(t, Config{Command: "/bin/sh", Cols: 80, Rows: 24})
	h.send("stty size\n")
	h.waitFor("24 80")

	// A rotation is a RESIZE frame, and the kernel turns the ioctl into a
	// SIGWINCH the child gets for free.
	h.resize(100, 40)
	h.send("stty size\n")
	h.waitFor("40 100")
}

func TestHostForwardsInputToTheChild(t *testing.T) {
	t.Parallel()
	requireShell(t)

	// The child must transform what it is given: ordinary tty echo would make a
	// command that was never delivered look delivered.
	h := startHarness(t, Config{
		Command: "/bin/sh",
		Args:    []string{"-c", `read line; echo GOT-$line`},
	})
	h.send("ping\n")
	h.waitFor("GOT-ping")
	h.waitForCleanEnd()
}

func TestHostForwardsOutputFromTheChild(t *testing.T) {
	t.Parallel()
	requireShell(t)

	h := startHarness(t, Config{
		Command: "/bin/sh",
		Args:    []string{"-c", `printf 'MARKER-OUTPUT\n'`},
	})
	h.waitFor("MARKER-OUTPUT")
	h.waitForCleanEnd()
}

func TestHostGivesTheChildTheEnvironmentItWasConfiguredWith(t *testing.T) {
	t.Parallel()
	requireShell(t)

	// The appliance sets TERM, COLORTERM and the client's own home; nothing may
	// be inherited from whatever started the host.
	h := startHarness(t, Config{
		Command: "/bin/sh",
		Args:    []string{"-c", `printf '%s|%s|%s\n' "$TERM" "$COLORTERM" "$HOME"`},
		Env:     []string{"TERM=xterm-256color", "COLORTERM=truecolor", "HOME=/tmp/console-home"},
	})
	h.waitFor("xterm-256color|truecolor|/tmp/console-home")
	h.waitForCleanEnd()
}

func TestHostSendsExitWithTheChildsStatusCode(t *testing.T) {
	t.Parallel()
	requireShell(t)

	for _, code := range []int{0, 1, 3, 42, 255} {
		t.Run(fmt.Sprintf("exit %v", code), func(t *testing.T) {
			t.Parallel()
			h := startHarness(t, Config{
				Command: "/bin/sh",
				Args:    []string{"-c", fmt.Sprintf("exit %v", code)},
			})
			if got := h.waitForExit(); got != byte(code) {
				t.Errorf("the exit frame carried %v, want %v", got, code)
			}
			if err := h.waitForRun(); err != nil {
				t.Errorf("Run returned %v, want nil", err)
			}
		})
	}
}

func TestHostKillsTheChildWhenTheSocketCloses(t *testing.T) {
	t.Parallel()
	requireShell(t)

	h, pid := startSleepingChild(t)
	// The app has gone: a console must never outlive the process that owns it.
	if err := h.local.Close(); err != nil {
		t.Fatalf("closing the app's end of the connection: %v", err)
	}
	if err := h.waitForRun(); err != nil {
		t.Errorf("Run returned %v, want nil", err)
	}
	// A killed pty child is not always destroyed outright: the platform can
	// leave it in its exit path, which is not running and not this session's to
	// finish. What matters is that nothing is left executing.
	waitUntil(t, "the child to stop running", func() bool { return childNotRunning(pid) })
}

func TestHostLeavesNoChildBehind(t *testing.T) {
	t.Parallel()
	requireShell(t)

	// The child exits on its own; after Run returns, the process must be gone
	// and reaped, not sitting in the process table as a zombie.
	h := startHarness(t, Config{
		Command: "/bin/sh",
		Args:    []string{"-c", `echo PID-$$; read line; exit 0`},
	})
	pid := pidFrom(t, h.waitFor("PID-"), "PID-")
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	h.send("go\n")
	if code := h.waitForExit(); code != 0 {
		t.Errorf("the exit frame carried %v, want 0", code)
	}
	if err := h.waitForRun(); err != nil {
		t.Errorf("Run returned %v, want nil", err)
	}
	if !processGone(pid) {
		t.Errorf("child %v survived the session", pid)
	}
}

func TestHostRefusesToRunWithoutAnAbsolutePath(t *testing.T) {
	t.Parallel()

	// This is not pedantry: Android's seccomp policy kills a process on the
	// faccessat2(2) that Go's exec.LookPath issues while resolving an
	// unqualified name, so a bare program name is a crash on the tablet.
	for _, command := range []string{"", "sh", "./sh", "bin/sh", "sh -c"} {
		local, remote := net.Pipe()
		defer func() { _ = local.Close() }()
		sess, err := NewSession(Config{Command: command}, remote)
		if err == nil {
			_ = remote.Close()
			t.Errorf("NewSession accepted the relative command %q", command)
			continue
		}
		if !strings.Contains(err.Error(), "absolute") {
			t.Errorf("the refusal of %q is %q, and must say the path has to be absolute", command, err)
		}
		if sess != nil {
			t.Errorf("NewSession returned a session for the refused command %q", command)
		}
		_ = remote.Close()
	}

	// An absolute path is accepted, and a zero size becomes the conventional one.
	local, remote := net.Pipe()
	defer func() { _ = local.Close() }()
	defer func() { _ = remote.Close() }()
	sess, err := NewSession(Config{Command: "/bin/sh"}, remote)
	if err != nil {
		t.Fatalf("NewSession refused the absolute command /bin/sh: %v", err)
	}
	if sess.cfg.Cols != 80 || sess.cfg.Rows != 24 {
		t.Errorf("a zero size became %vx%v, want 80x24", sess.cfg.Cols, sess.cfg.Rows)
	}
}

// startSleepingChild starts a child that reports its pid and then blocks, so a
// test can end the session and check that the child went with it.
func startSleepingChild(t *testing.T) (*consoleHarness, int) {
	t.Helper()
	h := startHarness(t, Config{
		Command: "/bin/sh",
		Args:    []string{"-c", `echo PID-$$; exec sleep 30`},
	})
	pid := pidFrom(t, h.waitFor("PID-"), "PID-")
	// If the session fails to reap it, the test machine must not be left with a
	// sleeping process for thirty seconds.
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return h, pid
}

// pidFrom reads the first integer after marker in terminal output.
func pidFrom(t *testing.T, output, marker string) int {
	t.Helper()
	at := strings.Index(output, marker)
	if at < 0 {
		t.Fatalf("the terminal never showed %q:\n%v", marker, output)
	}
	rest := output[at+len(marker):]
	if end := strings.IndexAny(rest, "\r\n \t"); end >= 0 {
		rest = rest[:end]
	}
	pid, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		t.Fatalf("the pid after %q is not a number: %q", marker, rest)
	}
	return pid
}

// processGone reports whether a pid has been reaped and no longer exists.
func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// childNotRunning reports whether a pid has stopped executing: either it is
// gone, or the platform is tearing it down.
//
// A killed child is not always destroyed promptly. macOS can leave a pty child
// in the kernel's exit path, where ps reports it as exiting, "E", and the
// parent's wait never returns; such a child holds no user code and no terminal
// work, so "it no longer runs" is the honest property to assert about it. A
// reaped child, which is what the clean-exit path produces, is checked by
// processGone instead.
func childNotRunning(pid int) bool {
	if processGone(pid) {
		return true
	}
	out, err := exec.Command("/bin/ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		// ps cannot see it either, which is the same answer.
		return true
	}
	return strings.ContainsAny(strings.TrimSpace(string(out)), "EZ")
}

func TestChildNotRunningTellsAnExitingChildFromARunningOne(t *testing.T) {
	t.Parallel()
	requireShell(t)

	// A child that has exited and has not been reaped is not running, even
	// though its pid is still in the process table.
	zombie := exec.Command("/bin/sh", "-c", "exit 0")
	if err := zombie.Start(); err != nil {
		t.Fatalf("starting a child that exits at once: %v", err)
	}
	pid := zombie.Process.Pid
	t.Cleanup(func() { _ = zombie.Wait() })
	waitUntil(t, "the child to stop running", func() bool { return childNotRunning(pid) })
	if processGone(pid) {
		t.Fatal("the unreaped child was reported as gone; this test no longer exercises what it is for")
	}

	// A child that is still running is running, whatever the platform says.
	running := exec.Command("/bin/sh", "-c", "exec sleep 30")
	if err := running.Start(); err != nil {
		t.Fatalf("starting a sleeping child: %v", err)
	}
	sleeping := running.Process.Pid
	t.Cleanup(func() {
		_ = running.Process.Kill()
		_ = running.Wait()
	})
	if childNotRunning(sleeping) {
		t.Errorf("child %v is running and was reported as not running", sleeping)
	}
}

// waitUntil polls cond until it holds, and fails the test if it never does.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(sessionTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waited %v for %v, and it never happened", sessionTimeout, what)
}
