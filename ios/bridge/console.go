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

package bridge

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/gmlewis/tcell/v2"
)

const (
	// defaultCols and defaultRows are the terminal an unspecified console gets.
	// They are the conventional size, and the client reflows from whatever the
	// app sends after it connects anyway.
	defaultCols = 80
	defaultRows = 24

	// maxSocketPath is the longest Unix socket path this bridge will bind: the
	// kernel's sun_path is 104 bytes including its terminating NUL, and a longer
	// name is refused rather than silently truncated. It matters on iOS, where
	// the sandbox's own temporary directory is long enough for a careless name to
	// reach the limit.
	maxSocketPath = 103
)

// TerminalName is the terminal the appliance's console claims to be, and the name
// cmd/gonomadnet/ios-host.go looks up to build the client's screen.
//
// It is not looked up on the device, and it must not be: the appliance's terminal
// is the Swift half's parser, and naming one of the entries the client can emit
// exactly is what makes the two agree. iOS has no terminfo database and no infocmp
// to consult, so the entry comes from tcell's own built-in table.
//
// The `-truecolor` suffix is what makes the client draw in 24-bit color: tcell takes
// the xterm-256color entry and adds the direct-color sequences to it, so the
// appliance's terminal emits `38;2;r;g;b` by default and nothing else about the entry
// changes — the same keys, the same mouse encoding, the same alternate screen. The
// Swift half's parser decodes `38;2` and `48;2` into the RGB its renderer draws, which
// is what the appliance's own theme is written in.
const TerminalName = "xterm-256color-truecolor"

// Console is the terminal an in-process client draws on.
//
// iOS gives an application no way to start a program, so there is no child
// process to put on a pseudo-terminal and no /dev/tty for tcell to open. The
// console therefore *is* the terminal: it implements tcell.Tty over the frame
// connection the app holds, so the client renders into the same byte stream the
// app parses. The wire format is the Android appliance's exactly (see
// frames.go), because the Swift half is written against the same golden table.
//
// A Console is a tcell.Tty, and tcell only ever reaches its terminal through
// that interface, so nothing here needs a file descriptor.
type Console struct {
	conn io.ReadWriteCloser

	// mu guards the state the client's goroutine and the reader's goroutine both
	// touch: the size, the notify callback, the buffer left by a short read, and
	// the reason the app's side ended.
	mu      sync.Mutex
	cols    int
	rows    int
	notify  func()
	pending []byte
	err     error

	// writeMu serializes frames, so two writes can never interleave on the
	// connection and leave the app reading a byte stream with no framing.
	writeMu sync.Mutex

	// data carries the app's keystrokes to the client's Read, in order. It is
	// unbuffered on purpose: a client that stops reading must slow the app down
	// rather than let output pile up in memory.
	data chan []byte

	// wake releases a Read that is parked in the client's input loop, which is
	// what Drain exists for. It is buffered so a Drain that arrives while nobody
	// is parked still leaves the next Read awake.
	wake chan struct{}

	closeOnce sync.Once
	done      chan struct{}
}

// NewConsole returns a console that speaks the frame protocol over conn and
// presents a cols-by-rows terminal to the client.
//
// A connection is required: a console without one has no app to draw for, and
// finding that out at the first keystroke rather than here would leave a client
// running with nothing to show.
func NewConsole(conn io.ReadWriteCloser, cols, rows uint16) (*Console, error) {
	if conn == nil {
		return nil, errors.New("bridge: a console needs a connection to the app")
	}
	if cols == 0 {
		cols = defaultCols
	}
	if rows == 0 {
		rows = defaultRows
	}
	c := &Console{
		conn: conn,
		cols: int(cols),
		rows: int(rows),
		data: make(chan []byte),
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	go c.readFrames()
	return c, nil
}

// readFrames turns the app's frames into the client's input.
//
// It is the only reader of the connection, so the frame stream has exactly one
// consumer and a frame can never be decoded twice.
func (c *Console) readFrames() {
	for {
		f, err := ReadFrame(c.conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// The app closed the socket: a normal end of the session.
				err = io.EOF
			}
			c.finish(err)
			return
		}

		switch f.Type {
		case FrameData:
			select {
			case c.data <- f.Data:
			case <-c.done:
				return
			}
		case FrameResize:
			c.mu.Lock()
			c.cols, c.rows = int(f.Cols), int(f.Rows)
			notify := c.notify
			c.mu.Unlock()
			if notify != nil {
				notify()
			}
		case FrameExit:
			c.finish(errors.New("bridge: the app sent an exit frame, which only the bridge sends"))
			return
		}
	}
}

// finish records why the app's side ended and releases the client's reader.
func (c *Console) finish(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
	close(c.data)
}

// readErr returns the reason the app's side ended.
func (c *Console) readErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return io.EOF
	}
	return c.err
}

// Read returns terminal input from the app.
//
// It blocks until the app sends a keystroke, and reports the reason the session
// ended once the app has gone: tcell's input loop only leaves a blocking Read
// when the terminal is being shut down, and a read that returned (0, nil) would
// instead spin.
func (c *Console) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	c.mu.Lock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()

	select {
	case b, ok := <-c.data:
		if !ok {
			return 0, c.readErr()
		}
		n := copy(p, b)
		if n < len(b) {
			c.mu.Lock()
			c.pending = b[n:]
			c.mu.Unlock()
		}
		return n, nil
	case <-c.done:
		return 0, c.readErr()
	case <-c.wake:
		// tcell is stopping the terminal. A read that returns no bytes and no
		// error lets its input loop come round, see the stop it was sent, and
		// leave: a read that stayed parked is a client that never finishes
		// shutting down.
		return 0, nil
	}
}

// Write sends the client's terminal output to the app as one DATA frame.
func (c *Console) Write(p []byte) (int, error) {
	// A copy is not taken: WriteFrame appends to a nil slice of its own, so the
	// frame owns its bytes before they can reach the connection.
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := WriteFrame(c.conn, DataFrame(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WindowSize reports the terminal's size, which follows the app's RESIZE frames.
func (c *Console) WindowSize() (tcell.WindowSize, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return tcell.WindowSize{Width: c.cols, Height: c.rows}, nil
}

// NotifyResize registers the callback tcell wants called when the terminal is
// resized. tcell passes nil to withdraw it.
func (c *Console) NotifyResize(cb func()) {
	c.mu.Lock()
	c.notify = cb
	c.mu.Unlock()
}

// Start activates the terminal. There is no raw mode to set and no state to
// save — the app owns the real terminal — so the only thing it does is forget a
// wake that a previous Stop left behind, because a resumed terminal starts with
// nothing pending.
func (c *Console) Start() error {
	select {
	case <-c.wake:
	default:
	}
	return nil
}

// Stop deactivates the terminal, which like Start has no state to unwind.
func (c *Console) Stop() error { return nil }

// Drain releases a Read that is parked in the client's input loop, which is what
// tcell requires of a terminal before it stops one.
//
// tcell's shutdown closes the stop channel, calls Drain, and then waits for its
// input loop to return — but that loop is parked inside Read, and a parked read
// never sees a closed channel. A Drain that does nothing therefore leaves the
// client unable to finish shutting down, and the whole appliance waiting on it.
// What Read is released by here is the same channel Close uses, so a console that
// is closed and one that is drained both come back.
func (c *Console) Drain() error {
	select {
	case c.wake <- struct{}{}:
	default:
		// A wake is already waiting, which is the same thing to the next Read.
	}
	return nil
}

// Close ends the session and releases the reader. It is safe to call more than
// once: tcell closes its Tty on shutdown, and the engine closes the console when
// the client stops.
func (c *Console) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
	return nil
}

// Listener accepts the appliance's connection to a console.
//
// The app is the client of this socket rather than its server, so session
// lifetime belongs to the bridge: an app that goes away ends the session, and a
// client that outlives it cannot hold the terminal open.
type Listener struct {
	ln   *net.UnixListener
	path string
}

// Listen binds a Unix socket at path for the appliance to dial.
//
// A path longer than the kernel's sun_path is refused rather than truncated:
// the truncated name would simply never match the one the app dials, and on iOS
// the sandbox's own temporary directory is long enough for that to be the first
// thing that goes wrong.
func Listen(path string) (*Listener, error) {
	if path == "" {
		return nil, errors.New("bridge: a console socket needs a path")
	}
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf(
			"bridge: the console socket path is %v bytes, and the kernel allows at most %v",
			len(path), maxSocketPath,
		)
	}

	// A socket left by a previous run would make the bind fail, and an app that
	// crashed is exactly when the next launch has to work.
	_ = os.Remove(path)

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, fmt.Errorf("bridge: resolving the console socket path %q: %w", path, err)
	}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("bridge: binding the console socket at %q: %w", path, err)
	}
	return &Listener{ln: ln, path: path}, nil
}

// Accept waits for the appliance to connect.
func (l *Listener) Accept() (*net.UnixConn, error) {
	conn, err := l.ln.AcceptUnix()
	if err != nil {
		return nil, fmt.Errorf("bridge: accepting the appliance's console connection: %w", err)
	}
	return conn, nil
}

// Path is the socket the appliance dials.
func (l *Listener) Path() string { return l.path }

// Close releases the socket and removes its file. It is safe to call more than
// once.
func (l *Listener) Close() error {
	err := l.ln.Close()
	_ = os.Remove(l.path)
	return err
}

// Console implements the terminal tcell renders through.
var _ tcell.Tty = (*Console)(nil)
