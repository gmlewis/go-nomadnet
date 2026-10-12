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
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmlewis/tcell/v2"
)

// fakeRunner stands in for the client: it records the terminal it was given and
// stays on it until the context ends, the way the client does.
type fakeRunner struct {
	mu       sync.Mutex
	calls    int
	gotTTY   bool
	gotCols  int
	gotRows  int
	config   string
	rnsDir   string
	ttySeen  chan struct{}
	finished chan struct{}
}

func (f *fakeRunner) run(ctx context.Context, tty tcell.Tty, configDir, rnsConfigDir string) error {
	f.mu.Lock()
	f.calls++
	f.gotTTY = tty != nil
	f.config, f.rnsDir = configDir, rnsConfigDir
	f.ttySeen = make(chan struct{})
	f.finished = make(chan struct{})
	seen, finished := f.ttySeen, f.finished
	f.mu.Unlock()

	if ws, err := tty.WindowSize(); err == nil {
		f.mu.Lock()
		f.gotCols, f.gotRows = ws.Width, ws.Height
		f.mu.Unlock()
	}

	// A client draws, which is what the app sees as DATA frames.
	_, _ = tty.Write([]byte("drawn by the client"))

	// And it reads its terminal: a real client's event loop leaves a blocking read
	// with an error when the app goes away, and that is what ends the session.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		buf := make([]byte, 32)
		for {
			if _, err := tty.Read(buf); err != nil {
				cancel()
				return
			}
		}
	}()

	close(seen)

	<-runCtx.Done()
	close(finished)
	return nil
}

// waitForTTY waits until the client has been given a terminal.
//
// The channel is made by the runner when it is called, so it does not exist until
// then: a test that read it straight away would park on a nil channel and hang.
func (f *fakeRunner) waitForTTY(t *testing.T) {
	t.Helper()
	var ch chan struct{}
	waitFor(t, "the client to be given a terminal", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		ch = f.ttySeen
		return ch != nil
	})
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("the client was given a terminal but never ran on it")
	}
}

// waitFor polls until cond is true, failing the test if it never is.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %v", what)
}

func TestHostRefusesAConsoleWithNoClientRegistered(t *testing.T) {
	t.Parallel()
	// A console with nothing to run on it would accept the app's connection and
	// then draw nothing, which is the least debuggable failure an appliance can
	// produce.
	h := &Host{}
	err := h.StartConsole(context.Background(), socketPath(t), 80, 24)
	if err == nil {
		t.Fatal("StartConsole with no client registered returned no error, want one")
	}
	if !errors.Is(err, ErrNoClient) && !strings.Contains(err.Error(), "no NomadNet client") {
		t.Errorf("StartConsole error = %v, want it to name the missing client", err)
	}
	if h.WaitConsole(time.Second) != true {
		t.Error("a console that never started reported itself as still running")
	}
}

func TestHostRunsTheClientOnTheAppsConsole(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	h := &Host{}
	h.SetClientRunner(runner.run)
	h.SetClientConfigs("/etc/nomadnetwork", "/etc/reticulum")
	t.Cleanup(func() { _ = h.Close() })

	path := socketPath(t)
	if err := h.StartConsole(context.Background(), path, 132, 43); err != nil {
		t.Fatalf("StartConsole: %v", err)
	}

	// The app is the one that dials, exactly as the Android appliance does.
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dialing the console: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// What the client draws reaches the app as the frame protocol.
	frame := readFrameWithin(t, conn)
	if frame.Type != FrameData || string(frame.Data) != "drawn by the client" {
		t.Fatalf("the app received %v, want the client's output", frame)
	}

	runner.waitForTTY(t)

	runner.mu.Lock()
	gotTTY, cols, rows := runner.gotTTY, runner.gotCols, runner.gotRows
	config, rnsDir := runner.config, runner.rnsDir
	runner.mu.Unlock()
	if !gotTTY {
		t.Error("the client was given no terminal")
	}
	if cols != 132 || rows != 43 {
		t.Errorf("the client's terminal is %vx%v, want 132x43", cols, rows)
	}
	if config != "/etc/nomadnetwork" || rnsDir != "/etc/reticulum" {
		t.Errorf("the client was given %q and %q, want its own configuration", config, rnsDir)
	}

	// The app's keystrokes reach the client through the same terminal, which is
	// the other half of the frame protocol.
	if err := WriteFrame(conn, DataFrame([]byte("k"))); err != nil {
		t.Fatalf("writing a keystroke: %v", err)
	}
}

func TestHostClosingEndsTheConsoleSession(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	h := &Host{}
	h.SetClientRunner(runner.run)
	t.Cleanup(func() { _ = h.Close() })

	path := socketPath(t)
	if err := h.StartConsole(context.Background(), path, 80, 24); err != nil {
		t.Fatalf("StartConsole: %v", err)
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dialing the console: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	runner.waitForTTY(t)

	// Shutting the appliance down has to end the client and release the socket: an
	// app that is killed and relaunched must be able to bind the same path.
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !h.WaitConsole(2 * time.Second) {
		t.Fatal("the console session did not release its socket")
	}
	if _, err := net.Dial("unix", path); err == nil {
		t.Error("the console socket still accepts connections after Close")
	}
}

func TestHostEndsTheConsoleWhenTheAppDisconnects(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	h := &Host{}
	h.SetClientRunner(runner.run)
	t.Cleanup(func() { _ = h.Close() })

	path := socketPath(t)
	if err := h.StartConsole(context.Background(), path, 80, 24); err != nil {
		t.Fatalf("StartConsole: %v", err)
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dialing the console: %v", err)
	}

	runner.waitForTTY(t)

	// The app going away ends the session: a client that outlived the app that
	// owns it would hold the terminal open for a reader who has left.
	if err := conn.Close(); err != nil {
		t.Fatalf("closing the app's end: %v", err)
	}
	if !h.WaitConsole(2 * time.Second) {
		t.Fatal("the console session outlived the app")
	}
}

func TestHostRefusesAConsoleSocketPathTooLong(t *testing.T) {
	t.Parallel()
	// iOS's own sandbox temporary directory is long enough for a careless name to
	// pass the kernel's limit, so the refusal is a real one and not a formality.
	runner := &fakeRunner{}
	h := &Host{}
	h.SetClientRunner(runner.run)
	t.Cleanup(func() { _ = h.Close() })

	long := "/tmp/" + strings.Repeat("x", 200)
	if err := h.StartConsole(context.Background(), long, 80, 24); err == nil {
		t.Error("StartConsole accepted an over-long socket path, want an error")
	}
}

func TestHostStartAndStopAreIdempotent(t *testing.T) {
	t.Parallel()
	sub := &fakeSubsystem{name: "transport"}
	h := &Host{}
	t.Cleanup(func() { _ = h.Close() })

	ctx := context.Background()
	if err := h.Start(ctx, freePort(t), sub); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Start(ctx, freePort(t), sub); err != nil {
		t.Fatalf("the second Start: %v", err)
	}
	if sub.starts != 1 {
		t.Errorf("the transport was started %v times, want 1", sub.starts)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("the second Close: %v", err)
	}

	// And a host that has been shut down says so rather than starting again: an app
	// whose launch path runs after its shutdown path must not resurrect the stack.
	if err := h.Start(ctx, freePort(t), sub); !errors.Is(err, ErrHostShutDown) {
		t.Errorf("Start after Close = %v, want %v", err, ErrHostShutDown)
	}
}

// TestTheApplianceStartsAgainAfterAStop is the first thing the device acceptance found.
//
// A reader presses Stop Stack and then Start Stack. It is what the guide tells them to do
// after pairing a radio, and what its troubleshooting tells them to do when the stack did
// not come up. The host that had been shut down was still the one the next start was asked
// for, so the appliance came up once per launch and never twice: the second Start Stack
// failed with "the appliance host has been shut down" and the button stayed on Start Stack.
func TestTheApplianceStartsAgainAfterAStop(t *testing.T) {
	applianceMu.Lock()
	appliance = &Host{}
	applianceMu.Unlock()

	// The client is registered once, by the host binary's init, and it is not part of
	// what a stop ends: the appliance that starts again is the same appliance.
	runner := &fakeRunner{}
	appliance.SetClientRunner(runner.run)

	sub := &fakeSubsystem{name: "transport"}
	ctx := context.Background()

	if err := StartAppliance(ctx, "/etc/nomadnetwork", "/etc/reticulum", freePort(t), sub); err != nil {
		t.Fatalf("StartAppliance: %v", err)
	}
	if err := StopAppliance(); err != nil {
		t.Fatalf("StopAppliance: %v", err)
	}
	if err := StartAppliance(ctx, "/etc/nomadnetwork", "/etc/reticulum", freePort(t), sub); err != nil {
		t.Fatalf("StartAppliance after StopAppliance: %v", err)
	}
	t.Cleanup(func() { _ = StopAppliance() })

	if sub.starts != 2 {
		t.Errorf("the transport started %v times across a stop and a start, want 2", sub.starts)
	}

	// And the console of the appliance that started again still has a client to run: a
	// restart that loses the registered runner is one whose console would be silent.
	path := socketPath(t)
	if err := StartConsoleSession(ctx, path, 80, 24); err != nil {
		t.Fatalf("StartConsoleSession after a restart: %v", err)
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dialing the console of the appliance that started again: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("closing the app's end: %v", err)
	}
	runner.waitForTTY(t)
}

func TestStatusForNamesTheReasonACallFailed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, StatusOK},
		{"no client", ErrNoClient, StatusBadArgument},
		{"bad argument", ErrBadArgument, StatusBadArgument},
		{"not running", ErrNotRunning, StatusNotRunning},
		{"shut down", ErrHostShutDown, StatusShutDown},
		{"anything else", errors.New("the socket was refused"), StatusFailed},
	} {
		if got := statusFor(tc.err); got != tc.want {
			t.Errorf("statusFor(%v) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestApplianceSampleHelpersRefuseAStoppedAppliance(t *testing.T) {
	t.Parallel()
	applianceMu.Lock()
	appliance = &Host{}
	applianceMu.Unlock()
	t.Cleanup(func() { _ = StopAppliance() })

	if err := FeedLocation(LocationSample{Lat: 1, Lng: 2, Valid: true}); !errors.Is(err, ErrNotRunning) {
		t.Errorf("FeedLocation with nothing running = %v, want %v", err, ErrNotRunning)
	}
	if err := FeedHeading(HeadingSample{MagneticDeg: 1, HasMagnetic: true}); !errors.Is(err, ErrNotRunning) {
		t.Errorf("FeedHeading with nothing running = %v, want %v", err, ErrNotRunning)
	}
}
