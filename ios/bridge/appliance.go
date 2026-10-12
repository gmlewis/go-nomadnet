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
	"sync"
	"time"
)

// The exported entry points the Swift half calls.
//
// Every one of them is a thin wrapper over an ordinary Go function that a test
// can call, because the alternative — testing the appliance only through a
// simulator and a C compiler — is how a bridge like this ends up with its logic
// in the one place nobody can reach. The C signatures are declared for Swift by
// hand in ios/Sources/GonomadnetBridge/include/bridge.h.
//
// Each returns a status: 0 for success, and a non-zero code whose meaning is
// stated in bridge.h. A failure also records its reason, which GonomadnetLastError
// hands back as a NUL-terminated string the caller must free with
// GonomadnetFreeError. A status alone cannot say why an appliance would not come
// up, and "it did not start" is the least actionable thing a bridge can report.

// Status codes the exported functions return. They are part of the C contract and
// are mirrored in bridge.h.
const (
	// StatusOK reports that the call did what it says.
	StatusOK = 0

	// StatusAlreadyRunning reports a start or stop that was not needed, which is
	// never an error: an app's launch path and its start button can both ask.
	StatusAlreadyRunning = 1

	// StatusNotRunning reports a stop or a sample for an appliance that is not
	// running.
	StatusNotRunning = 2

	// StatusBadArgument reports a configuration the bridge refuses, such as a
	// socket path too long to bind.
	StatusBadArgument = 3

	// StatusFailed reports that the call failed; the reason is in the last error.
	StatusFailed = 4

	// StatusShutDown reports a call against a host that has been shut down.
	StatusShutDown = 5
)

// appliance is the process's one host.
//
// One, because the appliance is a singleton by construction: it binds the pinned
// shared-instance port and the pinned sensor port, and a second one could not
// bind them anyway. Making that explicit here is what lets the exported
// functions be plain Go with no C in them.
var (
	applianceMu sync.Mutex
	appliance   = &Host{}
)

// currentAppliance is the host every exported entry point acts on.
//
// It is read under the lock because the host is replaced when one that has been
// shut down is started again, and a caller that read the pointer without it could
// hold a host that is no longer the appliance.
func currentAppliance() *Host {
	applianceMu.Lock()
	defer applianceMu.Unlock()
	return appliance
}

// statusFor maps a Go error onto a status code.
func statusFor(err error) int {
	switch {
	case err == nil:
		return StatusOK
	case errors.Is(err, ErrHostShutDown):
		return StatusShutDown
	case errors.Is(err, ErrNoClient), errors.Is(err, ErrBadArgument):
		return StatusBadArgument
	case errors.Is(err, ErrNotRunning):
		return StatusNotRunning
	}
	return StatusFailed
}

// StartAppliance brings the appliance up: the sensor feed, then the subsystems.
//
// feedPort is the loopback port the sensor feed binds; zero means the pinned one.
//
// Starting an appliance that has been stopped starts it again rather than reporting a
// shut-down host. A reader's Stop Stack then Start Stack is one appliance run twice, and
// the device acceptance found it was one appliance run once: the second start failed, so
// the stack could not be brought back up after it was taken down, and neither the guide's
// "stop the stack and start it again" nor its "press Restart Stack" could work.
func StartAppliance(ctx context.Context, configDir, rnsConfigDir string, feedPort int, subs ...Subsystem) error {
	applianceMu.Lock()
	h := appliance
	if h.Closed() {
		h = h.Successor()
		appliance = h
	}
	h.SetClientConfigs(configDir, rnsConfigDir)
	applianceMu.Unlock()
	return h.Start(ctx, feedPort, subs...)
}

// StopAppliance shuts the appliance down. Stopping something that is not running
// is not an error.
//
// The lock is held across the shutdown so that a stop and a start cannot interleave:
// bringing the stack back up means the one that is going away is gone first, socket and
// all, which is what the wait inside Close is for.
func StopAppliance() error {
	applianceMu.Lock()
	defer applianceMu.Unlock()
	return appliance.Close()
}

// StartConsoleSession listens for the app and runs the client on the console.
func StartConsoleSession(ctx context.Context, socketPath string, cols, rows uint16) error {
	return currentAppliance().StartConsole(ctx, socketPath, cols, rows)
}

// FeedLocation publishes one GNSS fix.
func FeedLocation(s LocationSample) error {
	engine := currentAppliance().Engine()
	if engine == nil {
		return ErrNotRunning
	}
	return engine.FeedLocation(s)
}

// FeedHeading publishes one orientation reading.
func FeedHeading(s HeadingSample) error {
	engine := currentAppliance().Engine()
	if engine == nil {
		return ErrNotRunning
	}
	return engine.FeedHeading(s)
}

// The reasons a call can fail for a reason of its own, so that statusFor can name
// them without matching the text of an error from somewhere else.
var (
	// ErrNotRunning reports a sample for an appliance that is not running.
	ErrNotRunning = errors.New("bridge: the appliance is not running")

	// ErrNoClient reports a console asked for before a client was registered.
	ErrNoClient = errors.New("bridge: no NomadNet client is registered")

	// ErrHostShutDown reports a call against a host that has been shut down.
	ErrHostShutDown = errors.New("bridge: the appliance host has been shut down")

	// ErrBadArgument reports an argument the bridge refuses.
	ErrBadArgument = errors.New("bridge: the appliance was given an argument it cannot use")
)

// DefaultConsoleSize is the terminal a console gets before the app has said how
// big its window is. The app sends a RESIZE frame as soon as it is attached.
const (
	DefaultConsoleCols = 80
	DefaultConsoleRows = 24
)

// TimeoutWaitForConsole is how long WaitStop waits for a console session to
// release its socket.
const TimeoutWaitForConsole = 5 * time.Second
