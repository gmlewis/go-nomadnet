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

package pty

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/term"
)

// childTimeout bounds every wait in this file. It is a safety net against a
// child that never answers, never a delay a passing test waits out.
const childTimeout = 10 * time.Second

// requirePtys skips a test on a platform that has no pseudo-terminals, where
// Open and StartWithSize answer with the pty library's ErrUnsupported.
func requirePtys(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("this platform has no pseudo-terminals")
	}
}

// requireSlaveIsTheMastersPeer fails a test whose slave is not the peer of its
// master, which is the property the whole package exists to preserve.
//
// The window size is what tells the two ends apart: it belongs to the master's
// own terminal, so the slave reports back what was set on the master only when
// the two are the ends of one pty. A slave taken from a recycled /dev/pts name
// belongs to a different pty, whose size this master never sets — and a child
// started on one of those never gets a terminal.
func requireSlaveIsTheMastersPeer(t *testing.T, master, slave *os.File) {
	t.Helper()

	if !term.IsTerminal(int(slave.Fd())) {
		t.Error("the slave is not a terminal")
	}

	want := &Winsize{Rows: 43, Cols: 97, X: 640, Y: 480}
	if err := Setsize(master, want); err != nil {
		t.Fatalf("setting the master's window size: %v", err)
	}
	got, err := GetsizeFull(slave)
	if err != nil {
		t.Fatalf("reading the slave's window size: %v", err)
	}
	if *got != *want {
		t.Errorf("the slave's window size is %+v, want %+v: the two are not ends of one pty", got, want)
	}
}

func TestOpenReturnsTheMastersOwnSlave(t *testing.T) {
	t.Parallel()
	requirePtys(t)

	master, slave, err := Open()
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer func() {
		_ = slave.Close()
		_ = master.Close()
	}()

	requireSlaveIsTheMastersPeer(t, master, slave)

	// A caller that publishes the slave for another process to open hands over
	// its name, so the slave has to have one.
	if name := slave.Name(); name == "" {
		t.Error("the slave has no name to publish")
	}
}

// TestStartWithSizeGivesTheChildATerminal asserts the property every caller
// starts a child for: the child is on a terminal, on every standard descriptor
// the caller left to the pty.
func TestStartWithSizeGivesTheChildATerminal(t *testing.T) {
	t.Parallel()
	requirePtys(t)

	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("this test runs a real child and needs /bin/sh: %v", err)
	}

	tests := []struct {
		name string
		// takeStdin hands the child a standard input of the test's rather than
		// the slave, which leaves the pty on stdout and stderr only.
		takeStdin bool
		script    string
	}{
		{
			name:   "all three descriptors",
			script: `test -t 0 && test -t 1 && test -t 2 && echo ISTTY`,
		},
		{
			name:      "standard input taken",
			takeStdin: true,
			script:    `test ! -t 0 && test -t 1 && test -t 2 && echo ISTTY`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command("/bin/sh", "-c", tt.script)
			if tt.takeStdin {
				devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
				if err != nil {
					t.Fatalf("opening %v: %v", os.DevNull, err)
				}
				defer func() { _ = devNull.Close() }()
				cmd.Stdin = devNull
			}
			master, err := StartWithSize(cmd, &Winsize{Rows: 24, Cols: 80})
			if err != nil {
				t.Fatalf("starting a child on a pty: %v", err)
			}
			defer func() { _ = master.Close() }()

			output := make(chan string, 1)
			go func() {
				var seen bytes.Buffer
				buf := make([]byte, 256)
				for !strings.Contains(seen.String(), "ISTTY") {
					n, err := master.Read(buf)
					seen.Write(buf[:n])
					if err != nil {
						break
					}
				}
				output <- seen.String()
			}()

			select {
			case seen := <-output:
				if !strings.Contains(seen, "ISTTY") {
					t.Errorf("the child did not see a terminal where it should:\n%v", seen)
				}
			case <-time.After(childTimeout):
				t.Fatal("the child never answered")
			}
			_ = cmd.Wait()
		})
	}
}
