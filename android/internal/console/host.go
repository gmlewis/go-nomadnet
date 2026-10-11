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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty/v2"
)

const (
	// defaultCols and defaultRows are the terminal an unspecified session gets.
	// They are the conventional size, and the client reflows from whatever the
	// app sends after it connects anyway.
	defaultCols = 80
	defaultRows = 24

	// outputChunk is the most the host takes from the pty's master at once.
	outputChunk = 32 * 1024

	// exitCodeUnknown is the status the app is told when a child was killed by a
	// signal and therefore has no exit status of its own.
	exitCodeUnknown = 255

	// exitFlushGrace bounds the wait for a dead child's last output.
	exitFlushGrace = 2 * time.Second

	// reapGrace bounds the wait for a killed child to be reaped. A reaped child
	// arrives in microseconds, so this is only ever reached on a platform that
	// will not finish the child off: macOS can leave a pty child in the kernel's
	// exit path, where wait4 does not return at all.
	reapGrace = 2 * time.Second
)

// child is one program running on its own pseudo-terminal.
type child struct {
	cmd    *exec.Cmd
	master *os.File
}

// exitStatus is what a reaped child reports to the app.
type exitStatus struct {
	code byte
}

// startChild runs cfg's command on a fresh pty.
//
// The child gets a new session and the pty's slave as its controlling terminal,
// which is the whole point: term.IsTerminal(0) is what makes cmd/gonomadnet
// take its terminal-UI path instead of falling back to daemon mode. Because the
// kernel owns the terminal, the child is never signalled by hand — a window
// size change is an ioctl and the kernel delivers SIGWINCH.
func startChild(cfg Config) (*child, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = cfg.Env
	cmd.Dir = cfg.Dir

	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: cfg.Rows, Cols: cfg.Cols})
	if err != nil {
		return nil, fmt.Errorf("console: starting %v on a pty: %w", cfg.Command, err)
	}
	return &child{cmd: cmd, master: master}, nil
}

// pumpInput forwards the app's frames to the child's terminal.
func (s *Session) pumpInput(c *child) error {
	for {
		f, err := ReadFrame(s.conn)
		if err != nil {
			return err
		}
		switch f.Type {
		case FrameData:
			if err := c.writeAll(f.Data); err != nil {
				return fmt.Errorf("console: writing to the child's terminal: %w", err)
			}
		case FrameResize:
			if err := c.resize(f.Cols, f.Rows); err != nil {
				return err
			}
		case FrameExit:
			// EXIT travels from the host to the app and never the other way.
			return errors.New("console: the app sent an exit frame, which only the host sends")
		}
	}
}

// pumpOutput forwards the child's terminal output to the app.
//
// It ends without an error when the pty reports that the child is gone, which
// is a normal end and not a failure to forward anything.
func (s *Session) pumpOutput(c *child) error {
	buf := make([]byte, outputChunk)
	for {
		n, err := c.master.Read(buf)
		if n > 0 {
			if werr := WriteFrame(s.conn, DataFrame(buf[:n])); werr != nil {
				return werr
			}
		}
		if err != nil {
			return nil
		}
	}
}

// writeAll writes p to the child's terminal in full.
func (c *child) writeAll(p []byte) error {
	for len(p) > 0 {
		n, err := c.master.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		p = p[n:]
	}
	return nil
}

// resize applies a new window size to the child's terminal.
func (c *child) resize(cols, rows uint16) error {
	err := pty.Setsize(c.master, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return fmt.Errorf("console: resizing the child's terminal to %vx%v: %w", cols, rows, err)
	}
	return nil
}

// wait reaps the child and returns the status the app should be told.
//
// Wait's own error is the child's non-zero exit or its death by a signal, which
// the exit status reports more precisely, so the two are not reported twice.
func (c *child) wait() exitStatus {
	if err := c.cmd.Wait(); err != nil && c.cmd.ProcessState == nil {
		return exitStatus{code: exitCodeUnknown}
	}
	state := c.cmd.ProcessState
	if state.ExitCode() < 0 {
		// Killed by a signal: there is no exit status to report.
		return exitStatus{code: exitCodeUnknown}
	}
	return exitStatus{code: byte(state.ExitCode())}
}

// kill stops the child.
//
// A child that has already exited is not a failure: the wait that follows is
// what decides how the session ended.
func (c *child) kill() {
	if c.cmd.Process == nil {
		return
	}
	_ = c.cmd.Process.Kill()
}

// close releases the pty's master, which the child has stopped using by then.
func (c *child) close() {
	_ = c.master.Close()
}
