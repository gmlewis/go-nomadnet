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

package tui

import (
	"sync"
	"time"
)

// timer is the handle returned by a clock's AfterFunc. It mirrors the subset
// of *time.Timer the debouncer needs (Reset to reschedule a pending call).
type timer interface {
	Reset(d time.Duration) bool
	Stop() bool
}

// clock schedules fn to run after a duration, as time.AfterFunc does. It is
// abstracted so the debouncer can be tested with a deterministic fake clock
// instead of real wall-clock time: the tests advance the clock explicitly, so
// they never depend on the scheduler and cannot flake under load.
type clock interface {
	AfterFunc(d time.Duration, fn func()) timer
}

// realClock wraps time.AfterFunc. *time.Timer satisfies timer.
type realClock struct{}

func (realClock) AfterFunc(d time.Duration, fn func()) timer {
	return time.AfterFunc(d, fn)
}

// debouncedCall coalesces a burst of Trigger calls into a single invocation of
// fn. The first Trigger schedules fn to run after window; each subsequent
// Trigger before the window elapses resets the timer, so a burst of N calls
// within the window collapses to one fn invocation. This is used to keep the
// tview event loop responsive when a callback fires once per incoming network
// event (e.g. announces/messages): without coalescing, a burst of N announces
// would queue N full UI refreshes (each doing disk I/O and a full redraw),
// starving key handling and making the UI appear hung. Python's per-announce
// directory_change_callback (Network.py:1744) avoids this by doing only cheap
// in-memory widget rebuilds; the Go refresh is heavier, so coalescing is the
// parity-preserving fix.
//
// Trigger is safe for concurrent use. fn runs on the clock's AfterFunc
// goroutine; it is the caller's responsibility to marshal any tview mutations
// inside fn onto the event loop (e.g. via App.QueueUpdateDraw).
type debouncedCall struct {
	fn     func()
	window time.Duration
	clock  clock

	mu    sync.Mutex
	timer timer
}

// NewDebouncedCall creates a debouncedCall that runs fn at most once per window
// no matter how rapidly Trigger is called. It uses the real wall-clock.
func NewDebouncedCall(window time.Duration, fn func()) *debouncedCall {
	return &debouncedCall{fn: fn, window: window, clock: realClock{}}
}

// newDebouncedCallWithClock is a test constructor that injects a deterministic
// clock so tests can advance time explicitly instead of sleeping.
func newDebouncedCallWithClock(window time.Duration, fn func(), c clock) *debouncedCall {
	return &debouncedCall{fn: fn, window: window, clock: c}
}

// Trigger schedules (or reschedules) the debounced fn call. A burst of Trigger
// calls within window results in a single fn invocation once the burst subsides.
func (d *debouncedCall) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer == nil {
		d.timer = d.clock.AfterFunc(d.window, d.fn)
		return
	}
	d.timer.Reset(d.window)
}
