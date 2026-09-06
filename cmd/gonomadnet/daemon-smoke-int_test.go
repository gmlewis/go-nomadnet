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

//go:build integration

package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/testutils"
)

// lineBuffer accumulates a subprocess's stdout lines so the test goroutine can
// poll for expected markers (Daemon mode active, LXMF Router ready to receive,
// Daemon stopped). Mirrors the harness in rrc/rrc-xprocess_test.go.
type lineBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (lb *lineBuffer) push(line string) {
	lb.mu.Lock()
	lb.lines = append(lb.lines, line)
	lb.mu.Unlock()
}

// waitForLine polls for the first line containing substr (substring match, not
// prefix — the RNS logger prefixes lines with "[date] [Info]     "). It returns
// the full matching line.
func (lb *lineBuffer) waitForLine(t *testing.T, substr string, timeout time.Duration) string {
	t.Helper()
	var out string
	if !testutils.PollUntil(timeout, func() bool {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		for i := len(lb.lines) - 1; i >= 0; i-- {
			if strings.Contains(lb.lines[i], substr) {
				out = lb.lines[i]
				return true
			}
		}
		return false
	}) {
		lb.mu.Lock()
		all := append([]string(nil), lb.lines...)
		lb.mu.Unlock()
		t.Fatalf("timeout waiting for %q; output so far:\n%v", substr, strings.Join(all, "\n"))
	}
	return out
}

// dump returns all captured lines joined by newlines (for failure diagnostics).
func (lb *lineBuffer) dump() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return strings.Join(append([]string(nil), lb.lines...), "\n")
}

// streamLines pipes reader lines into the lineBuffer.
func streamLines(r io.Reader, lb *lineBuffer) {
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			lb.push(scanner.Text())
		}
	}()
}

// buildDaemonBinary compiles the gonomadnet binary into a temp file and returns
// its path. The build uses GOCACHE=/tmp/go-cache (the host convention) so it
// shares the warm cache with the rest of the suite.
func buildDaemonBinary(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(tempDir(t), "gonomadnet")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/gonomadnet")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/go-cache")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build gonomadnet: %v\n%v", err, stderr.String())
	}
	return binPath
}

// repoRoot returns the repository root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %v", dir)
		}
		dir = parent
	}
}

// TestDaemonSmokeEndToEnd verifies the `-daemon` mode end-to-end (TODO):
// build the gonomadnet binary, start it with a fresh temp config + RNS config,
// confirm it registers its LXMF delivery destination and stays alive, then send
// SIGTERM and confirm it shuts down gracefully.
//
// "Registers destinations" is observed via the "LXMF Router ready to receive on
// <hash>" log line (nomadnet/app/app.go:410), which fires only after the LXMF
// router + delivery identity are registered. "Stays alive until SIGTERM" is
// observed by the process still running after that line, and "graceful
// shutdown" by the "Daemon stopped" line + a zero exit code.
func TestDaemonSmokeEndToEnd(t *testing.T) {
	t.Parallel()
	testutils.SkipShortIntegration(t)
	bin := buildDaemonBinary(t)

	cfgDir := tempDir(t)
	rnsDir := tempDir(t)

	cmd := exec.Command(bin,
		"-daemon", "-console",
		"-config", cfgDir,
		"-rnsconfig", rnsDir,
	)
	// The single-instance guard refuses to start gonomadnet if a nomadnet
	// (Python) process is running on the host. During integration tests the
	// developer may well have nomadnet running for parity checks, so bypass
	// that check here (the fresh temp config dir already isolates the lock).
	cmd.Env = append(os.Environ(), "GONOMADNET_IGNORE_RUNNING_NOMADNET=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	// Make sure the process is reaped even if the test fails early.
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		_, _ = cmd.Process.Wait()
	})

	lb := &lineBuffer{}
	// The daemon's own log.Printf lines ("Daemon mode active", "Daemon
	// stopped") go to stderr via the stdlib log package; the RNS logger lines
	// ("LXMF Router ready to receive") go to stdout via -console. Stream both
	// into the same lineBuffer so waitForLine sees every marker regardless of
	// which stream it was written to.
	streamLines(stdout, lb)
	streamLines(stderr, lb)

	// The daemon must report it is active and then register its LXMF delivery
	// destination (the "ready to receive" line proves RegisterDeliveryIdentity
	// succeeded in the async initRNS goroutine).
	lb.waitForLine(t, "Daemon mode active", 10*time.Second)
	readyLine := lb.waitForLine(t, "LXMF Router ready to receive on", 20*time.Second)
	ready := extractAngleHash(readyLine)
	if ready == "" {
		t.Errorf("LXMF ready line %q has no <hash> marker", readyLine)
	}
	t.Logf("daemon registered LXMF destination %v", ready)

	// Confirm the process is still alive a moment after registering (it must
	// stay up until SIGTERM, not exit on its own).
	if !processAlive(cmd.Process) {
		t.Fatalf("daemon exited before SIGTERM; output:\n%v", lb.dump())
	}

	// Graceful shutdown on SIGTERM (Python exits via urwid.ExitMainLoop on Quit;
	// daemon mode exits the signal loop and calls App.Shutdown).
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	// The daemon prints "Shutting down..." then "Daemon stopped" (daemon.go) on a
	// clean shutdown, then exits 0.
	lb.waitForLine(t, "Daemon stopped", 10*time.Second)
	stopped = true
	waitErr := cmd.Wait()
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 0 {
			t.Fatalf("daemon exited %v after SIGTERM; output:\n%v", exitErr.ExitCode(), lb.dump())
		}
	} else if waitErr != nil {
		t.Fatalf("daemon wait: %v", waitErr)
	}
}

// processAlive reports whether the given process still exists. Signal 0 is a
// non-destructive probe: it returns nil if the process exists.
func processAlive(p *os.Process) bool {
	if p == nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// extractAngleHash returns the "<hexhash>" substring of a line, or "" if none.
func extractAngleHash(line string) string {
	start := strings.Index(line, "<")
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start+1:], ">")
	if end < 0 {
		return ""
	}
	return line[start : start+1+end+1]
}
