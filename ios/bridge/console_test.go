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
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tempDir returns a temporary directory for a test, on a path short enough for
// a Unix domain socket.
//
// t.TempDir() is unusable for these tests on macOS: it builds a path under
// /var/folders/.../T/ that is far longer than sun_path's 104 bytes, so every
// socket the console binds is refused with EINVAL rather than tested.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ios-bridge-")
	if err != nil {
		t.Fatalf("creating a temporary directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// socketPath returns a Unix socket path short enough to bind, on Darwin and
// elsewhere alike.
func socketPath(t *testing.T) string {
	t.Helper()
	// t.TempDir() would be far too long on macOS, and so would a socket under
	// the iOS sandbox's own $TMPDIR; the name is short for the same reason.
	f, err := os.CreateTemp("/tmp", "iosc-")
	if err != nil {
		t.Fatalf("reserving a socket name: %v", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		t.Fatalf("closing the socket name file: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing the socket name file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

// pipeConnection is one end of an in-memory duplex connection, which is what
// the app's side of a console session looks like to the bridge.
type pipeConnection struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (c *pipeConnection) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *pipeConnection) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c *pipeConnection) Close() error                { return c.w.Close() }

// newPipe returns the bridge's end and the app's end of one duplex stream,
// each of which can be read and written independently.
func newPipe() (host, app *pipeConnection) {
	hr, aw := io.Pipe()
	ar, hw := io.Pipe()
	return &pipeConnection{r: hr, w: hw}, &pipeConnection{r: ar, w: aw}
}

// readFrameWithin reads one frame from r, failing the test if none arrives.
func readFrameWithin(t *testing.T, r io.Reader) Frame {
	t.Helper()
	type result struct {
		f   Frame
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := ReadFrame(r)
		ch <- result{f, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("reading a frame: %v", got.err)
		}
		return got.f
	case <-time.After(2 * time.Second):
		t.Fatal("no frame arrived within 2s")
		return Frame{}
	}
}

// readBytesWithin reads exactly n bytes from r, failing the test if they do not
// arrive.
func readBytesWithin(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	type result struct {
		p   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, n)
		_, err := io.ReadFull(r, buf)
		ch <- result{buf, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("reading %v bytes: %v", n, got.err)
		}
		return got.p
	case <-time.After(2 * time.Second):
		t.Fatalf("no %v bytes arrived within 2s", n)
		return nil
	}
}

func TestConsoleDeliversFrameDataToTheClient(t *testing.T) {
	t.Parallel()
	host, app := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// What the app sends as terminal keystrokes is what the client reads.
	go func() { _ = WriteFrame(app, DataFrame([]byte("hello"))) }()
	if got := readBytesWithin(t, c, 5); string(got) != "hello" {
		t.Errorf("the client read %q, want %q", got, "hello")
	}
}

func TestConsoleWritesClientOutputAsDataFrames(t *testing.T) {
	t.Parallel()
	host, app := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Everything the client draws reaches the app as DATA frames, in order and
	// undivided: the app's parser sees the same byte stream the client wrote.
	go func() {
		_, _ = c.Write([]byte("one "))
		_, _ = c.Write([]byte("two"))
	}()

	if got := readFrameWithin(t, app); got.Type != FrameData || string(got.Data) != "one " {
		t.Errorf("the app received %v, want Data(%q)", got, "one ")
	}
	if got := readFrameWithin(t, app); got.Type != FrameData || string(got.Data) != "two" {
		t.Errorf("the app received %v, want Data(%q)", got, "two")
	}
}

func TestConsoleWindowSizeFollowsResizeFrames(t *testing.T) {
	t.Parallel()
	host, app := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if ws, err := c.WindowSize(); err != nil || ws.Width != 80 || ws.Height != 24 {
		t.Fatalf("WindowSize() = %+v, %v, want 80x24", ws, err)
	}

	resized := make(chan struct{}, 4)
	c.NotifyResize(func() { resized <- struct{}{} })

	if err := WriteFrame(app, ResizeFrame(132, 43)); err != nil {
		t.Fatalf("sending a resize frame: %v", err)
	}
	select {
	case <-resized:
	case <-time.After(2 * time.Second):
		t.Fatal("the resize callback was not called within 2s")
	}

	if ws, err := c.WindowSize(); err != nil || ws.Width != 132 || ws.Height != 43 {
		t.Errorf("WindowSize() after a resize = %+v, %v, want 132x43", ws, err)
	}
}

func TestConsoleEndsWhenTheAppDisconnects(t *testing.T) {
	t.Parallel()
	host, app := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Everything the client draws reaches the app, so the app has to read it
	// before the next write can finish: an in-memory pipe has no buffer.
	drawn := make(chan struct{})
	go func() {
		_, _ = c.Write([]byte("drawn"))
		close(drawn)
	}()
	if got := readFrameWithin(t, app); got.Type != FrameData || string(got.Data) != "drawn" {
		t.Fatalf("the app received % v, want Data(%q)", got, "drawn")
	}
	<-drawn

	// The app going away is the end of the session, and the client's next Read
	// has to say so rather than block forever.
	if err := app.Close(); err != nil {
		t.Fatalf("closing the app's end: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 16)
		_, err := c.Read(buf)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Read returned nil after the app disconnected, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read blocked after the app disconnected")
	}
}

func TestConsoleRefusesAnExitFrameFromTheApp(t *testing.T) {
	t.Parallel()
	host, app := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// EXIT travels from the bridge to the app and never the other way, so one
	// arriving is a peer that is not speaking this protocol.
	if err := WriteFrame(app, ExitFrame(0)); err != nil {
		t.Fatalf("sending an exit frame: %v", err)
	}
	buf := make([]byte, 16)
	if _, err := c.Read(buf); err == nil {
		t.Error("Read returned nil after an exit frame, want an error")
	}
}

func TestConsoleEndsWhenTheFrameStreamLosesItsFraming(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"an unknown frame type", AppendFrame(nil, Frame{Type: 0x42})},
		{"a truncated payload", []byte{0x01, 0, 0, 0, 5, 'h', 'i'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host, app := newPipe()
			c, err := NewConsole(host, 80, 24)
			if err != nil {
				t.Fatalf("NewConsole: %v", err)
			}
			t.Cleanup(func() { _ = c.Close() })

			if _, err := app.Write(tc.wire); err != nil {
				t.Fatalf("writing to the pipe: %v", err)
			}
			// The app going away is what turns a frame that ended early from a
			// wait into a framing error.
			_ = app.Close()
			buf := make([]byte, 16)
			_, err = c.Read(buf)
			if err == nil {
				t.Fatal("Read returned nil after a lost frame stream, want an error")
			}
			if !errors.Is(err, ErrUnknownFrameType) && !errors.Is(err, ErrTruncatedFrame) {
				t.Errorf("Read error = %v, want a framing error", err)
			}
		})
	}
}

func TestConsoleCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	host, _ := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("the first Close returned %v, want nil", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("the second Close returned %v, want nil", err)
	}
}

func TestConsoleStartStopDrainAreSafe(t *testing.T) {
	t.Parallel()
	host, _ := newPipe()
	c, err := NewConsole(host, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// tcell calls Start, Drain and Stop around a session and again around a
	// suspend, so all three have to be safe to call more than once and in any
	// order.
	for _, step := range []struct {
		name string
		fn   func() error
	}{
		{"Start", c.Start},
		{"Drain", c.Drain},
		{"Start", c.Start},
		{"Stop", c.Stop},
		{"Stop", c.Stop},
	} {
		if err := step.fn(); err != nil {
			t.Errorf("%v returned %v, want nil", step.name, err)
		}
	}
}

func TestConsoleRefusesAnEmptyConnection(t *testing.T) {
	t.Parallel()
	if _, err := NewConsole(nil, 80, 24); err == nil {
		t.Error("NewConsole(nil) returned no error, want one")
	}
}

func TestConsoleDefaultsAnUnspecifiedSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		cols, rows uint16
		wantCols   int
		wantRows   int
	}{
		{"both zero", 0, 0, defaultCols, defaultRows},
		{"one zero", 0, 40, defaultCols, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host, _ := newPipe()
			c, err := NewConsole(host, tc.cols, tc.rows)
			if err != nil {
				t.Fatalf("NewConsole: %v", err)
			}
			t.Cleanup(func() { _ = c.Close() })

			ws, err := c.WindowSize()
			if err != nil {
				t.Fatalf("WindowSize: %v", err)
			}
			if ws.Width != tc.wantCols || ws.Height != tc.wantRows {
				t.Errorf("WindowSize() = %vx%v, want %vx%v", ws.Width, ws.Height, tc.wantCols, tc.wantRows)
			}
		})
	}
}

func TestListenerAcceptsTheAppsConnection(t *testing.T) {
	t.Parallel()
	path := socketPath(t)
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	dialed := make(chan *net.UnixConn, 1)
	go func() {
		conn, err := net.Dial("unix", path)
		if err != nil {
			dialed <- nil
			return
		}
		dialed <- conn.(*net.UnixConn)
	}()

	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	app := <-dialed
	if app == nil {
		t.Fatal("the app could not dial the console's socket")
	}
	t.Cleanup(func() { _ = app.Close() })

	// The socket carries the frame protocol, not raw bytes.
	if err := WriteFrame(app, DataFrame([]byte("k"))); err != nil {
		t.Fatalf("writing a frame from the app's end: %v", err)
	}
	c, err := NewConsole(conn, 80, 24)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	if got := readBytesWithin(t, c, 1); !bytes.Equal(got, []byte("k")) {
		t.Errorf("the client read %q, want %q", got, "k")
	}
}

func TestListenRefusesAPathTooLongForASocket(t *testing.T) {
	t.Parallel()
	// A socket path past sun_path's 104 bytes cannot be bound, and on iOS the
	// sandbox's own $TMPDIR is long enough to reach that limit, so the refusal
	// has to be a clear error rather than a truncated name.
	long := filepath.Join(tempDir(t), string(bytes.Repeat([]byte("x"), 120)))
	if _, err := Listen(long); err == nil {
		t.Error("Listen of an over-long path returned no error, want one")
	}
}

func TestListenRefusesAnEmptyPath(t *testing.T) {
	t.Parallel()
	if _, err := Listen(""); err == nil {
		t.Error("Listen(\"\") returned no error, want one")
	}
}
