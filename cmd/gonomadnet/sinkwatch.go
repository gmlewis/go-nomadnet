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

//go:build !no_tui && !embedded && !pocket_communicator && !pocket_hub

package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// logSinkFailureGrace is how long a file-sink failure must persist before the
// client stops. A transient failure — an antivirus scanner or a backup tool
// holding the log file for a moment — clears in milliseconds; a real one (disk
// full, permissions, a file locked for the session) does not.
var logSinkFailureGrace = 2 * time.Second

// sinkWatch turns the logger's sink-error reports into a stop decision.
//
// The terminal UI must never write a log line to the terminal, so a failed
// file sink silently drops lines (the logger's console fallback is off). That
// is acceptable for a moment and not for a session, so once a failure has
// persisted past the grace period the client stops through its normal graceful
// shutdown, which lets tcell restore the terminal, and then reports the error
// on the restored screen.
//
// A rotation failure is not fatal: the line was written, and the log file
// simply keeps growing.
type sinkWatch struct {
	now   func() time.Time
	grace time.Duration
	path  string

	mu      sync.Mutex
	since   time.Time
	reason  error
	stopped bool
}

// newSinkWatch returns a watch for the log file at path.
func newSinkWatch(path string) *sinkWatch {
	return &sinkWatch{now: time.Now, grace: logSinkFailureGrace, path: path}
}

// observe records one sink report and reports whether the client must stop.
// A nil error clears the streak; a rotation failure is ignored; a repeated
// failure returns true once it has outlived the grace period.
func (w *sinkWatch) observe(op string, err error) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err == nil {
		w.since, w.reason, w.stopped = time.Time{}, nil, false
		return false
	}
	if op == rns.LogSinkRotating {
		return false
	}
	if w.since.IsZero() {
		w.since = w.now()
		w.reason = fmt.Errorf("the log file %q is no longer writable (%v): %w", w.path, op, err)
		return false
	}
	if w.now().Sub(w.since) < w.grace {
		return false
	}
	w.stopped = true
	return true
}

// stopReason returns the reason the client was stopped, or nil when the sink
// never caused a stop.
func (w *sinkWatch) stopReason() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.stopped {
		return nil
	}
	return w.reason
}
