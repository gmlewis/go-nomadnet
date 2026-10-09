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

// Package console gives the gonomadnet client a real terminal on Android,
// where the platform provides none.
//
// The appliance runs the client as a bundled executable out of its own
// nativeLibraryDir, but a tview program needs a controlling terminal and a
// window size, and Android has neither. This package supplies them: it opens a
// PTY, starts the client on the slave so that term.IsTerminal reports true, and
// bridges the PTY master to the Android application over an abstract Unix
// socket.
//
// The application is the server and this host dials it, so session lifetime
// belongs to the app rather than to a process the app cannot see. Every byte
// crosses the socket as a frame (see frames.go): raw terminal bytes one way,
// window-size changes and the client's exit code the other.
//
// The Android side implements the same wire format in Kotlin, against the same
// golden table of bytes, because a codec disagreement between the two halves
// produces a console that renders nothing and reports no error.
package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Config describes the child a console session runs and the terminal it runs it
// on.
type Config struct {
	// Command is the absolute path of the program to run. A relative path is
	// refused: Android's seccomp policy kills a process on the faccessat2(2)
	// that Go's exec.LookPath issues while resolving an unqualified name.
	Command string

	// Args are the program's arguments.
	Args []string

	// Env is the child's entire environment, as KEY=VALUE pairs. A nil Env
	// inherits this process's environment, which is never what an appliance
	// wants; the console host passes the client exactly what it should see.
	Env []string

	// Dir is the child's working directory. Empty means this process's.
	Dir string

	// Cols and Rows are the terminal's initial size. A zero becomes 80 by 24.
	Cols uint16
	Rows uint16
}

// Session bridges one child process on a PTY to one console connection.
//
// A Session is used once: Run starts the child, pumps in both directions until
// the child exits or the connection ends, and returns.
type Session struct {
	cfg  Config
	conn io.ReadWriteCloser
}

// NewSession returns a session that will run cfg's command and speak the frame
// protocol over conn.
//
// The command is validated here rather than at start, because a relative path
// is a configuration error that must be caught before anything is executed, and
// on Android it is fatal rather than merely wrong.
func NewSession(cfg Config, conn io.ReadWriteCloser) (*Session, error) {
	if !filepath.IsAbs(cfg.Command) {
		return nil, fmt.Errorf(
			"console: the command %q must be an absolute path: Android's seccomp policy kills a process that resolves an unqualified name",
			cfg.Command,
		)
	}
	if conn == nil {
		return nil, errors.New("console: a session needs a connection")
	}
	if cfg.Cols == 0 {
		cfg.Cols = defaultCols
	}
	if cfg.Rows == 0 {
		cfg.Rows = defaultRows
	}
	return &Session{cfg: cfg, conn: conn}, nil
}

// Run runs the session to its end and returns when the child is done with and
// the connection is closed.
//
// The session ends in one of three ways, and only the first of them reports an
// exit code:
//
//   - The child exits. Its remaining output is forwarded, an EXIT frame carries
//     the status, and the connection is closed.
//   - The connection ends, because the app closed it or the context was
//     cancelled. The child is killed and reaped: a console must never outlive
//     the app that owns it. The wait for that reap is bounded, because a killed
//     child is not always reaped — a platform can leave it in its exit path and
//     then never return from the wait — and an app that has gone must be
//     released either way.
//   - The frame stream loses its framing. That is reported as an error, and the
//     child is killed for the same reason.
func (s *Session) Run(ctx context.Context) error {
	c, err := startChild(s.cfg)
	if err != nil {
		return err
	}
	defer c.close()

	// The child's death is observed by one goroutine that also reaps it, so no
	// path out of Run can leave a process behind.
	exited := make(chan exitStatus, 1)
	go func() { exited <- c.wait() }()

	inbound := make(chan error, 1)
	go func() { inbound <- s.pumpInput(c) }()

	outbound := make(chan error, 1)
	go func() { outbound <- s.pumpOutput(c) }()

	var cause error
	select {
	case status := <-exited:
		// Everything the child wrote is still in the pty's buffer, so the exit
		// frame goes out only after the reader has drained it.
		drained := drainOutput(outbound)
		if err := WriteFrame(s.conn, ExitFrame(status.code)); err != nil {
			if drained != nil {
				return fmt.Errorf("console: forwarding the child's output: %w", drained)
			}
			return fmt.Errorf("console: sending the exit frame: %w", err)
		}
		// Closing the connection is how the app learns the session is over, and
		// it releases the goroutine still reading frames from it.
		_ = s.conn.Close()
		return nil
	case <-ctx.Done():
		cause = ctx.Err()
	case err := <-inbound:
		cause = err
	}

	c.kill()
	// A killed child is not always reaped. macOS can leave a pty child in the
	// kernel's exit path, where it is reported as exiting and wait4 never
	// returns, so the wait is bounded: the wait for a reaped child, and the
	// close of the pty's master that follows this function's return, are what
	// release the child, and neither may hang the app.
	if _, reaped := waitForReap(exited, reapGrace); reaped {
		// Everything the child wrote is still in the pty's buffer, so it is
		// forwarded before the connection goes. An error from it is the app
		// having gone, which is what brought the session here.
		_ = drainOutput(outbound)
	}
	_ = s.conn.Close()
	return cleanEnd(cause)
}

// cleanEnd turns the reason a session's input side ended into the error Run
// reports: a closed connection is a normal end, a lost frame stream is not.
func cleanEnd(err error) error {
	if err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrClosed) ||
		errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// waitForReap waits for a killed child to be reaped, and reports whether it was.
//
// An abandoned reap is not a failure: the child is past running by then, and
// the caller closes the pty's master as it returns, which is what lets the
// kernel finish the child off.
func waitForReap(exited <-chan exitStatus, grace time.Duration) (exitStatus, bool) {
	select {
	case status := <-exited:
		return status, true
	case <-time.After(grace):
		return exitStatus{code: exitCodeUnknown}, false
	}
}

// drainOutput waits for the pty reader to finish, so that no terminal output is
// dropped behind the exit frame.
//
// The wait is bounded: a grandchild holding the pty open must not keep the
// session alive once the client itself is gone.
func drainOutput(outbound <-chan error) error {
	select {
	case err := <-outbound:
		return err
	case <-time.After(exitFlushGrace):
		return nil
	}
}
