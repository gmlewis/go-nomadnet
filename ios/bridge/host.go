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
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gmlewis/tcell/v2"
)

// Package bridge is the appliance's Go half: the subsystems the iOS app runs in-process,
// the terminal it gives the NomadNet client, and the C entry points the Swift half calls.
//
// On Android the appliance is eight programs and a Kotlin supervisor that starts them. An
// iOS process may not start a program at all, so the same subsystems run here as
// goroutines, and the Swift half reaches them through the exported functions in this file.
// The one thing that does not change is the wire format: the frame protocol in frames.go
// is byte for byte the one Android's console host speaks, because both halves of the
// appliance are tested against one golden table.
//
// This file is the appliance's host lifecycle, and it holds no C.
//
// The C entry points live in bridge.go, which is built only for GOOS=ios, because a file
// that imports "C" makes the compiler hand cgo's own generated sources to every analyzer
// that walks the package. Those generated files are not this repository's code and cannot
// be kept clean by it, so the boundary is exactly one file wide and everything behind it
// is ordinary Go that every gate and every test can see.

// ClientRunner runs the NomadNet client on a terminal and returns when it has
// finished.
//
// It is a seam rather than a call because the client lives in a `package main`
// and Go forbids importing one: the host binary that links this package
// registers its own client here, and everything above this line stays testable
// with a fake runner.
//
// The terminal is a tcell.Tty rather than a tcell.Screen so that the runner owns
// the choice of terminfo entry: the screen a device has is the runner's business,
// and it needs the same tty to ask about it.
type ClientRunner func(ctx context.Context, tty tcell.Tty, configDir, rnsConfigDir string) error

// Host is the appliance's process-wide state: the engine, the console session,
// and the client runner the host binary registered.
//
// It is one value rather than a set of package-level variables so that its
// lifecycle is one lifecycle: an app that is backgrounded and relaunched must be
// able to shut the whole thing down and bring it up again, and every part of it
// has to be released by the same Close.
type Host struct {
	mu sync.Mutex

	// client is the runner the host binary registered. Without it there is no
	// client to run, and a console that accepted a connection would sit there
	// drawing nothing.
	client ClientRunner

	// engine is the running appliance, or nil before Start.
	engine *Engine

	// configDir and rnsConfigDir are where the client's own configuration lives,
	// remembered from Start so a console started later can hand them to the
	// client.
	configDir    string
	rnsConfigDir string

	// feedPort is the sensor feed's port, remembered so a restart uses the same
	// one.
	feedPort int

	// scanEndpoint and scanToken are where the app's camera scanner is listening
	// and what a request for a scan must prove, remembered from Start so the
	// client can be told when it is started. Empty when the app offered no
	// scanner, which is a client whose dialogs have no Scan button — see
	// cmd/gonomadnet/ios-host.go, which puts them in the client's environment.
	scanEndpoint string
	scanToken    string

	// consoleCancel ends the running console session, and consoleDone is closed
	// when its goroutine has finished releasing the socket.
	consoleCancel context.CancelFunc
	consoleDone   chan struct{}

	// closed reports that the host has been shut down, which is what makes a
	// late call a clear error rather than a socket bound after everything else
	// has gone.
	closed bool
}

// SetClientRunner registers the NomadNet client this binary will run in-process.
//
// The host binary calls it from its own init, which is the only point guaranteed
// to run before anything asks for a console.
func (h *Host) SetClientRunner(r ClientRunner) {
	h.mu.Lock()
	h.client = r
	h.mu.Unlock()
}

// Engine is the running appliance, or nil when it is not running.
func (h *Host) Engine() *Engine {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.engine
}

// Start brings up the appliance: the sensor feed first, then the subsystems in
// the order it was given them.
func (h *Host) Start(ctx context.Context, feedPort int, subs ...Subsystem) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return ErrHostShutDown
	}
	if h.engine != nil {
		return nil
	}

	e, err := NewEngine(EngineConfig{FeedPort: feedPort}, subs...)
	if err != nil {
		return err
	}
	if err := e.Start(ctx); err != nil {
		return err
	}
	h.engine = e
	h.feedPort = feedPort
	return nil
}

// SetClientConfigs records where the client's own configuration lives.
//
// The client is started long after the appliance is, when the reader opens the
// console, so the paths it needs are remembered here rather than threaded through
// every call.
func (h *Host) SetClientConfigs(configDir, rnsConfigDir string) {
	h.mu.Lock()
	h.configDir, h.rnsConfigDir = configDir, rnsConfigDir
	h.mu.Unlock()
}

// SetScanEndpoint records where the app's camera scanner is listening, and the token
// a request for a scan must carry.
//
// The client has no camera of its own — an iOS application may not start a program,
// and the camera belongs to the app — so a dialog that would rather read an address
// from a QR code than have it typed asks the app for one over a loopback connection.
// The app binds that listener and names it here before the console exists, because a
// client told an endpoint after it has started is a client whose first Scan button
// does nothing.
//
// An empty endpoint means no scanner at all, which is the right reading of a launch
// where the listener could not be bound: the client then draws no Scan button, and
// addresses are typed, exactly as on a desktop terminal.
func (h *Host) SetScanEndpoint(endpoint, token string) {
	h.mu.Lock()
	h.scanEndpoint, h.scanToken = endpoint, token
	h.mu.Unlock()
}

// ScanEndpoint returns the scanner the app named, or empty strings when it named none.
func (h *Host) ScanEndpoint() (endpoint, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.scanEndpoint, h.scanToken
}

// StartConsole listens for the app to attach to a console and then runs the
// client on it, in this process.
//
// It returns as soon as the listener is bound, because the session ends the way
// every other one does: the client exits, an EXIT frame carries the status over
// the socket, and the connection closes. A caller that wants to know when the
// client is done watches the socket.
//
// A host with no registered client refuses rather than binding a socket that
// would accept a connection and then draw nothing: an appliance whose console is
// silent and reports no error is the least debuggable failure there is.
func (h *Host) StartConsole(ctx context.Context, socketPath string, cols, rows uint16) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrHostShutDown
	}
	if h.client == nil {
		h.mu.Unlock()
		return fmt.Errorf("%w: there is nothing to run on a console", ErrNoClient)
	}
	if h.consoleCancel != nil {
		// A second console replaces the first rather than being refused: the app
		// rebuilds its console when the reader reopens it, and an old session that
		// has already been abandoned must not block the new one.
		h.consoleCancel()
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	h.consoleCancel = cancel
	done := make(chan struct{})
	h.consoleDone = done
	client := h.client
	configDir, rnsConfigDir := h.configDir, h.rnsConfigDir
	h.mu.Unlock()

	listener, err := Listen(socketPath)
	if err != nil {
		cancel()
		h.mu.Lock()
		h.consoleCancel, h.consoleDone = nil, nil
		h.mu.Unlock()
		close(done)
		return err
	}

	go func() {
		defer close(done)
		defer cancel()

		err := serveConsole(sessionCtx, listener, client, configDir, rnsConfigDir, cols, rows)
		// The listener is closed by serveConsole, and this reports nothing: the
		// session ending is how a console ends, and the app has already been told
		// by the EXIT frame. An error here is only reachable when the app is gone.
		_ = err
	}()
	return nil
}

// WaitConsole waits for the console session to finish releasing its socket, for
// a test or a shutdown path that has to know.
func (h *Host) WaitConsole(timeout time.Duration) bool {
	h.mu.Lock()
	done := h.consoleDone
	h.mu.Unlock()
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// serveConsole accepts one connection and runs the client on it.
func serveConsole(
	ctx context.Context,
	listener *Listener,
	client ClientRunner,
	configDir, rnsConfigDir string,
	cols, rows uint16,
) error {
	defer func() { _ = listener.Close() }()

	// The app dials as soon as it has the path, so the accept is not raced
	// against anything; a cancellation that arrives while it is parked is the app
	// going away, and closing the listener is what releases it.
	type accepted struct {
		conn *net.UnixConn
		err  error
	}
	acceptedCh := make(chan accepted, 1)
	go func() {
		conn, err := listener.Accept()
		acceptedCh <- accepted{conn: conn, err: err}
	}()

	var conn *net.UnixConn
	select {
	case a := <-acceptedCh:
		if a.err != nil {
			return a.err
		}
		conn = a.conn
	case <-ctx.Done():
		_ = listener.Close()
		<-acceptedCh
		return ctx.Err()
	}

	console, err := NewConsole(conn, cols, rows)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = console.Close() }()

	if err := client(ctx, console, configDir, rnsConfigDir); err != nil {
		return fmt.Errorf("bridge: running the client: %w", err)
	}
	return nil
}

// Closed reports that the host has been shut down, which is what makes a late
// call a clear error rather than a socket bound after everything else has gone.
func (h *Host) Closed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

// Successor is a host that has not been shut down, carrying over what is registered
// once for the life of the process: the client a console runs.
//
// A Host is one-shot on purpose — shutting one down is what stops a call arriving
// after the shutdown from resurrecting the stack. The appliance is not one-shot: a
// reader's Stop Stack followed by Start Stack, which is what the guide tells them to
// do after pairing a radio, means the same appliance run again, and it gets a host
// that has not been shut down rather than an error.
func (h *Host) Successor() *Host {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Everything that describes the appliance rather than the run is carried over. The
	// scanner is one of those: a Stop Stack followed by Start Stack is the same app with the
	// same camera, and a successor that forgot it would leave the reader with dialogs whose
	// Scan button had gone.
	return &Host{client: h.client, scanEndpoint: h.scanEndpoint, scanToken: h.scanToken}
}

// Close shuts the appliance down: the console first, then the engine, so the
// client stops drawing before the transport it draws from goes away.
//
// The wait for the console is bounded and it is not optional: the socket's path
// outlives the process that bound it, and an app that is killed and relaunched has
// to find the path free. A session that will not come back does not hold the
// shutdown open — it is cancelled either way, and the listener is closed before its
// goroutine returns.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	cancel := h.consoleCancel
	done := h.consoleDone
	h.consoleCancel = nil
	engine := h.engine
	h.engine = nil
	h.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(TimeoutWaitForConsole):
		}
	}
	h.mu.Lock()
	h.consoleDone = nil
	h.mu.Unlock()

	if engine != nil {
		return engine.Close()
	}
	return nil
}
