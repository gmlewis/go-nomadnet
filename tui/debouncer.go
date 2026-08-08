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
// Trigger is safe for concurrent use. fn runs on a time.AfterFunc goroutine; it
// is the caller's responsibility to marshal any tview mutations inside fn onto
// the event loop (e.g. via App.QueueUpdateDraw).
type debouncedCall struct {
	fn     func()
	window time.Duration

	mu    sync.Mutex
	timer *time.Timer
}

// NewDebouncedCall creates a debouncedCall that runs fn at most once per window
// no matter how rapidly Trigger is called.
func NewDebouncedCall(window time.Duration, fn func()) *debouncedCall {
	return &debouncedCall{fn: fn, window: window}
}

// Trigger schedules (or reschedules) the debounced fn call. A burst of Trigger
// calls within window results in a single fn invocation once the burst subsides.
func (d *debouncedCall) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer == nil {
		d.timer = time.AfterFunc(d.window, d.fn)
		return
	}
	d.timer.Reset(d.window)
}
