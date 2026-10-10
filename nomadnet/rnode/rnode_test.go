// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package rnode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// logRecorder collects what a server reported, from the goroutine that reported
// it, so that a test can read it from its own.
type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *logRecorder) Logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

// recorded returns what has been reported so far.
func (r *logRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

// within is how long a test waits for bytes the bridge is supposed to be carrying.
//
// Every read a test does on a pty or a socket is bounded by it. An unbounded read turns a
// platform difference into a hang, and this is not hypothetical: the first version of these
// tests waited ten minutes on a CI runner for a byte that never arrived, and reported it as
// a timeout on the whole package rather than as a failure in the test that was wrong. A
// bound makes such a difference fail in milliseconds and names the read that did not come.
const within = 10 * time.Second

// readExactly reads count bytes from r, and fails the test rather than blocking forever.
//
// The read happens on its own goroutine so that a reader which never returns cannot take the
// test with it. A goroutine left behind here is harmless — the test binary exits when the
// package's tests are done — and is very much preferable to a suite that hangs.
func readExactly(t *testing.T, what string, r io.Reader, count int) []byte {
	t.Helper()
	type outcome struct {
		data []byte
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		buf := make([]byte, count)
		_, err := io.ReadFull(r, buf)
		done <- outcome{data: buf, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("reading %v bytes from %v: %v", count, what, got.err)
		}
		return got.data
	case <-time.After(within):
		t.Fatalf("nothing arrived from %v within %v, so the bridge is not carrying bytes", what, within)
		return nil
	}
}

// tempRoot returns a fresh directory under /tmp, which is where a test may
// write and where a socket path is short enough to bind.
func tempRoot(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", prefix)
	if err != nil {
		t.Fatalf("creating a temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestTheBridgePublishesARawSerialPath(t *testing.T) {
	t.Parallel()

	bridge, err := Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	if bridge.SlavePath() == "" {
		t.Fatal("the bridge published an empty slave path")
	}
	slave, err := os.OpenFile(bridge.SlavePath(), os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening the published slave path %v: %v", bridge.SlavePath(), err)
	}
	defer func() { _ = slave.Close() }()

	// A radio's protocol is framed binary data, and a pty in its default state
	// rewrites it: the line discipline turns a carriage return into a newline
	// on the way in, a newline into carriage-return newline on the way out, eats
	// 0x03 as an interrupt, and stops the output at a 0x13. The bridge is a
	// serial cable, so every byte has to arrive as it was written, in both
	// directions.
	outbound := []byte{0x0d, 0x03, 0x13, 0x1a, 0x7f, 0x00, 0xff, 'A'}
	if _, err := bridge.Master().Write(outbound); err != nil {
		t.Fatalf("writing to the pty's master: %v", err)
	}
	if got := readExactly(t, "the slave", slave, len(outbound)); !bytes.Equal(got, outbound) {
		t.Errorf("the transport reads % x, want % x", got, outbound)
	}

	inbound := []byte{0x0a, 0x0d, 0x03, 0x11, 'B'}
	if _, err := slave.Write(inbound); err != nil {
		t.Fatalf("writing to the slave: %v", err)
	}
	if got := readExactly(t, "the master", bridge.Master(), len(inbound)); !bytes.Equal(got, inbound) {
		t.Errorf("the radio's bytes are % x, want % x", got, inbound)
	}
}

func TestTheSlaveStaysUsableWhileNothingHasOpenedIt(t *testing.T) {
	t.Parallel()

	// The transport is configured with the published path and opens it when it
	// starts, which is after the bridge is already running. A pty whose slave has
	// no open file descriptor reports its master as at end of file, so a bridge
	// that let go of the slave would end the moment it was started — and the
	// transport would then find a path that no longer names anything.
	//
	// What is asserted is that invariant and not a timing: a read on the master must
	// not report the end of the stream. Bytes are a platform's own business and are
	// not the end of anything; an error is. An earlier version of this test demanded
	// that the read not return *at all* inside a 50 ms window, which is a timing
	// assertion about the absence of an event — and it is the one that failed on a CI
	// runner whose pty answered immediately, taking the whole package down with it.
	bridge, err := Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	ended := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := bridge.Master().Read(buf)
		ended <- err
	}()

	select {
	case err := <-ended:
		if err != nil {
			t.Fatalf("the master reported %v with no slave open, so the bridge cannot outlive its start", err)
		}
	case <-time.After(250 * time.Millisecond):
	}

	// Whatever that read did, the pair is still a serial path: bytes written to the
	// master reach a slave that is opened now, which is exactly what the transport
	// does when it starts. The reader above cannot take them — it is reading the
	// master, and these are delivered to the slave.
	slave, err := os.OpenFile(bridge.SlavePath(), os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening the slave after the fact: %v", err)
	}
	defer func() { _ = slave.Close() }()
	if _, err := bridge.Master().Write([]byte("still here")); err != nil {
		t.Fatalf("writing to the master: %v", err)
	}
	if got := readExactly(t, "the slave", slave, len("still here")); string(got) != "still here" {
		t.Errorf("the transport reads %q, want %q", got, "still here")
	}
}

func TestPumpCarriesBytesBothWaysAndEndsWhenEitherEndCloses(t *testing.T) {
	t.Parallel()

	before := runtime.NumGoroutine()

	bridge, err := Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	slave, err := os.OpenFile(bridge.SlavePath(), os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening the slave: %v", err)
	}
	defer func() { _ = slave.Close() }()

	// The app's end is a socket in production and a pipe here: what is being
	// tested is the pump, not the transport.
	app, peer := net.Pipe()

	ended := make(chan error, 1)
	go func() { ended <- Pump(bridge.Master(), app) }()

	// The transport writes to the slave; the radio's bytes reach the app.
	if _, err := slave.Write([]byte("to the radio")); err != nil {
		t.Fatalf("writing to the slave: %v", err)
	}
	if got := readExactly(t, "the app's end", peer, len("to the radio")); string(got) != "to the radio" {
		t.Errorf("the app reads %q, want %q", got, "to the radio")
	}

	// And the other way: what the app writes reaches the transport.
	if _, err := peer.Write([]byte("from the radio")); err != nil {
		t.Fatalf("writing to the app's end: %v", err)
	}
	if got := readExactly(t, "the slave", slave, len("from the radio")); string(got) != "from the radio" {
		t.Errorf("the transport reads %q, want %q", got, "from the radio")
	}

	// The app going away ends the pump, releases both ends, and leaves no
	// goroutine behind: a bridge that outlived its app would hold the pty for
	// the life of the install.
	_ = peer.Close()
	select {
	case err := <-ended:
		if err != nil {
			t.Errorf("Pump reported %v after a clean close, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Pump did not return after the app's end was closed")
	}
	waitForGoroutines(t, before)

	// Closing the bridge's own end ends a pump too, which is how a context
	// cancellation releases a radio.
	bridge2, err := Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	app2, peer2 := net.Pipe()
	defer func() { _ = peer2.Close() }()
	ended2 := make(chan error, 1)
	go func() { ended2 <- Pump(bridge2.Master(), app2) }()
	_ = bridge2.Close()
	select {
	case err := <-ended2:
		if err != nil {
			t.Fatalf("Pump returned %v after the bridge was closed, want a clean end", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Pump did not return after the bridge was closed")
	}
	waitForGoroutines(t, before)
}

// waitForGoroutines fails the test if the goroutines running before a test
// started have not come back. It is a settle rather than a delay: the check
// passes on the first look whenever the release was immediate.
func waitForGoroutines(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		now := runtime.NumGoroutine()
		if now <= before {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("the pump leaked goroutines: %v running, %v before", now, before)
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPublishPathWritesTheSlaveWhereTheAppCanFindIt(t *testing.T) {
	t.Parallel()

	dir := tempRoot(t, "rnode-bridge-")
	path := filepath.Join(dir, "rnode", "tty")

	// The directory does not exist yet, which is the ordinary case: the app names
	// a path inside its own private storage and the bridge is the first thing to
	// write there.
	if err := PublishPath(path, "/dev/pts/7"); err != nil {
		t.Fatalf("PublishPath failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the published path: %v", err)
	}
	if string(got) != "/dev/pts/7" {
		t.Errorf("the published path is %q, want %q", got, "/dev/pts/7")
	}

	// A reader must never see a half-written path: the app watches this file and
	// configures the transport with whatever it finds, so an empty or partial
	// read would be a transport pointed at a path that is not one.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the directory: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "tty" {
			t.Errorf("PublishPath left %v behind", entry.Name())
		}
	}

	if err := PublishPath(path, "/dev/pts/8"); err != nil {
		t.Fatalf("republishing: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the republished path: %v", err)
	}
	if string(got) != "/dev/pts/8" {
		t.Errorf("the republished path is %q, want %q", got, "/dev/pts/8")
	}
}

func TestUnpublishPathLeavesNothingForTheNextStartToFind(t *testing.T) {
	t.Parallel()

	dir := tempRoot(t, "rnode-bridge-")
	path := filepath.Join(dir, "tty")
	if err := PublishPath(path, "/dev/pts/7"); err != nil {
		t.Fatalf("PublishPath failed: %v", err)
	}
	if err := UnpublishPath(path); err != nil {
		t.Fatalf("UnpublishPath failed: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the published path is still there after UnpublishPath: %v", err)
	}
	// Removing a path that is not there is what a second cleanup does, and it is
	// not a failure.
	if err := UnpublishPath(path); err != nil {
		t.Errorf("UnpublishPath on a missing path reported %v, want nil", err)
	}
}

func TestTheServerRefusesAPeerFromAnotherUid(t *testing.T) {
	t.Parallel()

	dir := tempRoot(t, "rnode-bridge-")
	socketPath := filepath.Join(dir, "sock")

	bridge, err := Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	ln, err := Listen(socketPath)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	var refusals logRecorder
	// A stranger first, this app second: the stranger must be sent away without a
	// byte of the radio's stream, and the server must keep serving.
	attempts := 0
	srv := &Server{
		Listener: ln,
		Bridge:   bridge,
		OurUID:   1000,
		PeerUID: func(net.Conn) (int, error) {
			attempts++
			if attempts == 1 {
				return 4242, nil
			}
			return 1000, nil
		},
		Logf: refusals.Logf,
	}
	served := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { served <- srv.Serve(ctx) }()

	stranger, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dialing the bridge: %v", err)
	}
	defer func() { _ = stranger.Close() }()

	// The refusal is a close: a read on the stranger's end reports the end of the
	// stream rather than the radio's bytes.
	_ = stranger.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	if _, err := stranger.Read(buf); err == nil {
		t.Fatal("a peer from another uid was served")
	}
	if len(refusals.recorded()) == 0 {
		t.Error("the refusal was not reported, so nothing in the log would explain a radio that never connects")
	}

	// The app itself is still able to connect afterwards, and the connection it is given
	// stays open — that is what being served means. A refused peer is closed, so a read on
	// it ends; a served one is being carried, so a read on it does nothing at all until the
	// radio says something.
	//
	// An earlier version of this test proved "served" by writing a byte to the pty's slave
	// and waiting for it to come out of the socket. Byte-carrying is the pump's own business
	// and is asserted in TestPumpCarriesBytesBothWaysAndEndsWhenEitherEndCloses; here all it
	// did was make the server's accept policy depend on pty timing, and it is where this test
	// failed on CI — the byte written to the slave never reached the master, the pump sat
	// idle, and the test hung for the whole ten-minute package timeout.
	app, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dialing the bridge after a refusal: %v", err)
	}
	defer func() { _ = app.Close() }()

	if err := app.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		t.Fatalf("setting a read deadline on the served peer: %v", err)
	}
	switch _, err := app.Read(make([]byte, 1)); {
	case err == nil:
		t.Fatal("the served peer sent a byte the radio never sent")
	case errors.Is(err, os.ErrDeadlineExceeded):
		// Nothing arrived and the connection did not end: the server is carrying it.
	default:
		t.Fatalf("the served peer's connection is not open: %v", err)
	}

	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve returned %v when its context was cancelled, want a clean stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return when its context was cancelled")
	}
}

func TestTheServerReportsAPeerItCannotIdentify(t *testing.T) {
	t.Parallel()

	dir := tempRoot(t, "rnode-bridge-")
	socketPath := filepath.Join(dir, "sock")

	bridge, err := Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = bridge.Close() }()

	ln, err := Listen(socketPath)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	var reported logRecorder
	srv := &Server{
		Listener: ln,
		Bridge:   bridge,
		OurUID:   os.Getuid(),
		PeerUID: func(conn net.Conn) (int, error) {
			return 0, errors.New("credentials are not readable here")
		},
		Logf: reported.Logf,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	// An unidentifiable peer is refused rather than trusted: the alternative is a
	// process that cannot be told from this app writing into the radio.
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dialing the bridge: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("a peer whose uid could not be read was served")
	}
	lines := reported.recorded()
	if len(lines) == 0 {
		t.Fatal("the unreadable peer was not reported")
	}
	if !strings.Contains(lines[0], "uid") {
		t.Errorf("the report %q does not say what could not be read", lines[0])
	}
}

func TestSocketNameIsUnpredictableAndPinnedToItsPrefix(t *testing.T) {
	t.Parallel()

	first, err := NewSocketEntropy()
	if err != nil {
		t.Fatalf("NewSocketEntropy failed: %v", err)
	}
	second, err := NewSocketEntropy()
	if err != nil {
		t.Fatalf("NewSocketEntropy failed: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two socket names would carry the same entropy")
	}
	if a, b := SocketName(4242, first), SocketName(4242, second); a == b {
		t.Errorf("two names for one pid collided: %v", a)
	}
	if a, b := SocketName(1, first), SocketName(2, first); a == b {
		t.Errorf("two pids produced the same name: %v", a)
	}

	// The prefix is what the Kotlin half builds the same name from, and the two
	// have to agree or the app dials a socket nobody is listening on.
	name := SocketName(4242, first)
	if !strings.HasPrefix(name, SocketNamePrefix) {
		t.Errorf("the socket name %q does not begin with %q", name, SocketNamePrefix)
	}
	if strings.ContainsAny(name, "/\x00") {
		t.Errorf("the socket name %q cannot be an abstract-socket name", name)
	}
}

func TestListenBindsAPathAndAnAbstractName(t *testing.T) {
	t.Parallel()

	dir := tempRoot(t, "rnode-bridge-")

	// A path is bound as a path, which is how the two halves are exercised on a
	// development machine and how the tests here reach a real socket.
	at, err := Listen(filepath.Join(dir, "sock"))
	if err != nil {
		t.Fatalf("Listen on a path failed: %v", err)
	}
	if _, err := net.Dial("unix", filepath.Join(dir, "sock")); err != nil {
		t.Errorf("dialing the bound path: %v", err)
	}
	_ = at.Close()

	// A bare name is bound abstract, which is what the appliance uses: an
	// abstract socket has no filesystem entry, so there is no directory an app
	// would need permission to write a socket into.
	//
	// It is a Linux name. Elsewhere the same spelling binds a socket file in the
	// working directory, which is how a suite that binds in its own package
	// directory once left hundreds of socket files behind, and there is nothing
	// to test there that the path case above has not already covered.
	if runtime.GOOS != "linux" {
		t.Skip("an abstract Unix socket is a Linux name")
	}
	entropy, err := NewSocketEntropy()
	if err != nil {
		t.Fatalf("NewSocketEntropy failed: %v", err)
	}
	name := SocketName(os.Getpid(), entropy)
	abstract, err := Listen(name)
	if err != nil {
		t.Fatalf("Listen on %v failed: %v", name, err)
	}
	defer func() { _ = abstract.Close() }()
	if _, err := net.Dial("unix", "@"+name); err != nil {
		t.Errorf("dialing the abstract name %v: %v", name, err)
	}
}
