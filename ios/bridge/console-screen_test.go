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
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/tcell/v2"
)

// This file is the join between the console and the client, and it is the one seam
// the appliance turns on.
//
// console.go gives the client a tcell.Tty; cmd/gonomadnet/ios-host.go builds a real
// tcell screen on top of it and runs the NomadNet client through that. Each half is
// tested and the join was not: a console that satisfied every test here and a screen
// built on one were never put together, so an appliance that opened a console and
// drew nothing would have passed everything. These tests build the screen exactly
// the way the host binary builds it — the same terminfo lookup, the same
// constructor, the same order — over a console bound to a real socket, and then
// assert the four things the join has to do: render, take a keystroke, follow the
// app's resize, and end when the app goes away.

// screenBuildMu serializes building a client terminal.
//
// tcell's terminfo entries are process-global, and building a screen writes to the
// entry it is handed: prepareKeys marks an xterm-like terminal on the entry itself.
// The appliance builds one screen at a time, because there is one console, so the
// device cannot have this race; these tests build several at once, and they take
// turns rather than reporting tcell's shared entry as a race of their own.
var screenBuildMu sync.Mutex

// appSide is the app's end of a console session: it drains the frames the bridge
// sends, which is what an app that stopped reading would otherwise make the client
// block on.
type appSide struct {
	conn *net.UnixConn

	mu     sync.Mutex
	data   []byte
	frames []Frame
	err    error

	done chan struct{}
}

// newAppSide starts draining conn, which is the app's end of the session.
func newAppSide(t *testing.T, conn *net.UnixConn) *appSide {
	t.Helper()
	a := &appSide{conn: conn, done: make(chan struct{})}
	go a.drain()
	t.Cleanup(func() { _ = a.conn.Close() })
	return a
}

// drain reads every frame the bridge sends, in order, until the session ends.
func (a *appSide) drain() {
	defer close(a.done)
	for {
		f, err := ReadFrame(a.conn)
		if err != nil {
			a.mu.Lock()
			a.err = err
			a.mu.Unlock()
			return
		}
		a.mu.Lock()
		a.frames = append(a.frames, f)
		if f.Type == FrameData {
			a.data = append(a.data, f.Data...)
		}
		a.mu.Unlock()
	}
}

// terminalOutput is everything the client has drawn so far, concatenated: the
// app's parser sees one byte stream, and so does this.
func (a *appSide) terminalOutput() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return bytes.Clone(a.data)
}

// waitForOutput waits, up to a bounded time, for the client's output to contain
// want, and fails with what it did contain when it never does.
func (a *appSide) waitForOutput(t *testing.T, what string, want []byte) []byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := a.terminalOutput()
		if bytes.Contains(got, want) {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("the app never received %v within 2s; the client drew %q", what, got)
		}
		time.Sleep(time.Millisecond)
	}
}

// close ends the app's side of the session, which is what an app that has gone
// away looks like from the bridge.
func (a *appSide) close() {
	_ = a.conn.Close()
	<-a.done
}

// openConsoleSession brings up a console and the client's terminal on it, in the
// order the appliance does: the bridge binds the socket, the app dials it, the
// console is built on the connection, and the client's screen is built on the
// console and initialized.
func openConsoleSession(t *testing.T, cols, rows uint16) (tcell.Screen, *appSide) {
	t.Helper()

	path := socketPath(t)
	listener, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	// The app dials as soon as it has the path, and the bridge is waiting for it.
	app := dialInBackground(t, path)

	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	console, err := NewConsole(conn, cols, rows)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	t.Cleanup(func() { _ = console.Close() })

	// The client's terminal, built the way the host binary builds it: the same
	// terminfo lookup, then the same constructor. tcell reaches its terminal
	// only through the Tty interface, so nothing below this line wants a file
	// descriptor.
	screen := buildClientScreen(t, console)

	// The app's end drains before the screen is initialized: initialization
	// writes the terminal's setup to the console, and an app that is not reading
	// is a client that blocks inside its own Init.
	side := newAppSide(t, app)

	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)

	return screen, side
}

// buildClientScreen returns the client's terminal on a console, built the way
// cmd/gonomadnet/ios-host.go builds it: the same terminfo lookup, then the same
// constructor. tcell reaches its terminal only through the Tty interface, so
// nothing here wants a file descriptor and nothing here needs a device.
func buildClientScreen(t *testing.T, console *Console) tcell.Screen {
	t.Helper()

	screenBuildMu.Lock()
	defer screenBuildMu.Unlock()

	ti, err := tcell.LookupTerminfo(TerminalName)
	if err != nil {
		t.Fatalf("LookupTerminfo(%v): %v", TerminalName, err)
	}
	screen, err := tcell.NewTerminfoScreenFromTtyTerminfo(console, ti)
	if err != nil {
		t.Fatalf("NewTerminfoScreenFromTtyTerminfo(%v): %v", TerminalName, err)
	}
	return screen
}

// dialInBackground connects to the console's socket, which the bridge is about to
// accept.
func dialInBackground(t *testing.T, path string) *net.UnixConn {
	t.Helper()
	type dialed struct {
		conn *net.UnixConn
		err  error
	}
	ch := make(chan dialed, 1)
	go func() {
		c, err := net.Dial("unix", path)
		if err != nil {
			ch <- dialed{nil, err}
			return
		}
		ch <- dialed{c.(*net.UnixConn), nil}
	}()

	select {
	case d := <-ch:
		if d.err != nil {
			t.Fatalf("the app dialing the console socket: %v", d.err)
		}
		return d.conn
	case <-time.After(2 * time.Second):
		t.Fatal("the app could not dial the console socket within 2s")
		return nil
	}
}

// pollEventWithin returns the next event the client's terminal produces.
func pollEventWithin(t *testing.T, screen tcell.Screen) tcell.Event {
	t.Helper()
	ch := make(chan tcell.Event, 1)
	go func() { ch <- screen.PollEvent() }()

	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("the client's terminal produced no event within 2s")
		return nil
	}
}

// pollEventTypeWithin returns the next event the client's terminal produces that
// is of the wanted type, ignoring the others.
func pollEventTypeWithin[T tcell.Event](t *testing.T, screen tcell.Screen) T {
	t.Helper()
	var zero T
	deadline := time.Now().Add(2 * time.Second)
	for {
		ev := pollEventWithin(t, screen)
		if typed, ok := ev.(T); ok {
			return typed
		}
		if time.Now().After(deadline) {
			t.Fatalf("the client's terminal produced no %T within 2s; it produced %T", zero, ev)
		}
	}
}

// waitForScreenSize waits, up to a bounded time, for the client's terminal to
// report the size the app asked for.
func waitForScreenSize(t *testing.T, screen tcell.Screen, cols, rows int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		w, h := screen.Size()
		if w == cols && h == rows {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the client's terminal is %vx%v after 2s, wanted %vx%v", w, h, cols, rows)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestTheAppliancesTerminalDrawsDirectColor asserts the client's terminal is a
// 24-bit one.
//
// The appliance's theme is written in direct color, and a terminal that has no
// direct-color capability makes tcell degrade every one of those colors to the
// 256-color cube before the app ever sees them — the device would draw a palette it
// was never designed for, and the Swift half's own RGB path would never be used.
func TestTheAppliancesTerminalDrawsDirectColor(t *testing.T) {
	t.Parallel()

	screenBuildMu.Lock()
	defer screenBuildMu.Unlock()

	ti, err := tcell.LookupTerminfo(TerminalName)
	if err != nil {
		t.Fatalf("LookupTerminfo(%v): %v", TerminalName, err)
	}
	if ti.SetFgRGB == "" || ti.SetBgRGB == "" {
		t.Fatalf("%v has no direct-color capability: the client would draw in 256 colors", TerminalName)
	}
	for _, want := range []string{"38;2;", "48;2;"} {
		if !strings.Contains(ti.SetFgRGB+ti.SetBgRGB, want) {
			t.Errorf("%v does not emit %v", TerminalName, want)
		}
	}
}

// TestConsoleCarriesDirectColorToTheApp asserts the whole path the appliance's
// theme takes: a cell the client draws in a direct color arrives at the app as the
// 24-bit sequence, which is what the Swift half's parser decodes.
func TestConsoleCarriesDirectColorToTheApp(t *testing.T) {
	t.Parallel()
	screen, app := openConsoleSession(t, 80, 24)

	// A color that is not in the 256-color cube, so a terminal without direct
	// color would have to replace it with one that is.
	direct := tcell.NewRGBColor(0x12, 0x34, 0x56)
	screen.SetContent(0, 0, 'X', nil, tcell.StyleDefault.Foreground(direct))
	screen.Show()

	app.waitForOutput(t, "the cell's direct color", []byte("\x1b[38;2;18;52;86m"))
}

// TestConsoleRendersTheClientsTerminal asserts the client is genuinely drawing
// into the console: the terminal's setup arrives at the app as DATA frames, and a
// cell the client draws arrives the same way.
func TestConsoleRendersTheClientsTerminal(t *testing.T) {
	t.Parallel()
	screen, app := openConsoleSession(t, 80, 24)

	// Initializing the screen writes the xterm-256color entry's own setup to the
	// console, and the alternate-screen sequence is the one that says the
	// client's terminal — not the app's — decided what to emit.
	app.waitForOutput(t, "the terminal's setup", []byte("\x1b[?1049h"))

	// And what the client draws reaches the app: this is the path every frame of
	// the client's interface takes, and the only one the reader ever sees.
	screen.SetContent(0, 0, 'X', nil, tcell.StyleDefault)
	screen.Show()
	app.waitForOutput(t, "a cell the client drew", []byte("X"))
}

// TestConsoleDeliversAKeystrokeToTheClientScreen asserts the other direction: a
// keystroke the app sends as a DATA frame arrives at the client as a key event.
func TestConsoleDeliversAKeystrokeToTheClientScreen(t *testing.T) {
	t.Parallel()
	screen, app := openConsoleSession(t, 80, 24)

	// A keystroke travels as a DATA frame, which is the frame the client's own
	// output uses in the other direction.
	if err := WriteFrame(app.conn, DataFrame([]byte("q"))); err != nil {
		t.Fatalf("sending a keystroke from the app: %v", err)
	}

	key := pollEventTypeWithin[*tcell.EventKey](t, screen)
	if key.Rune() != 'q' {
		t.Errorf("the client's terminal read the key %q, wanted %q", key.Rune(), 'q')
	}
}

// TestConsoleResizeReachesTheClientScreen asserts the app's RESIZE frame is what
// the client's terminal believes its window to be, which is what makes the client
// reflow for a rotation instead of drawing for the grid it started with.
func TestConsoleResizeReachesTheClientScreen(t *testing.T) {
	t.Parallel()
	screen, app := openConsoleSession(t, 80, 24)

	if w, h := screen.Size(); w != 80 || h != 24 {
		t.Fatalf("the client's terminal started at %vx%v, wanted 80x24", w, h)
	}

	if err := WriteFrame(app.conn, ResizeFrame(132, 43, 1920, 1080)); err != nil {
		t.Fatalf("sending a resize from the app: %v", err)
	}

	// The console hands the new size to tcell through the callback tcell
	// registered, and tcell re-reads the size on its next resize pass.
	waitForScreenSize(t, screen, 132, 43)
}

// TestConsoleSessionEndsWhenTheAppDisconnects asserts the app going away ends the
// session: the client's terminal is told, its shutdown does not panic, and the
// goroutines the session started are gone.
//
// It is deliberately not parallel: it counts goroutines, and a session running
// beside it would be counted as one this session leaked.
func TestConsoleSessionEndsWhenTheAppDisconnects(t *testing.T) {
	before := runtime.NumGoroutine()

	screen, app := openConsoleSession(t, 80, 24)

	app.close()

	// A client blocked on a keystroke has to be told the session ended, or the
	// reader closes the console and the client is still there waiting.
	ev := pollEventTypeWithin[*tcell.EventError](t, screen)
	if ev.Error() == "" {
		t.Error("the client's terminal reported the session ended without a reason")
	}

	// Fini is what the client calls on its way out, and it must not panic on a
	// console whose app has gone.
	screen.Fini()

	waitForGoroutines(t, before)
}

// waitForGoroutines waits, briefly, for the goroutines a session started to go
// away. A session that leaks one per console is a session that leaks one every
// time the reader reopens the console.
func waitForGoroutines(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		now := runtime.NumGoroutine()
		if now <= before {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the session left %v goroutines behind after 2s, and there were %v before it", now, before)
		}
		time.Sleep(time.Millisecond)
	}
}
