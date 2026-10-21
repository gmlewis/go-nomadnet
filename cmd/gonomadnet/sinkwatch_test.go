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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
)

// TestSinkWatch drives the stop policy with a fake clock.
func TestSinkWatch(t *testing.T) {
	t.Parallel()

	errBroken := errors.New("locked by another process")
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	advance := func(w *sinkWatch, d time.Duration) { at = at.Add(d) }

	cases := []struct {
		name   string
		steps  []func(w *sinkWatch) bool // each returns observe's result
		want   bool                      // whether any step said stop
		reason bool                      // whether stopReason is non-nil
	}{
		{
			name: "a single failure does not stop the client",
			steps: []func(*sinkWatch) bool{
				func(w *sinkWatch) bool { return w.observe(rns.LogSinkWriting, errBroken) },
			},
		},
		{
			name: "a failure past the grace period stops the client",
			steps: []func(*sinkWatch) bool{
				func(w *sinkWatch) bool { return w.observe(rns.LogSinkWriting, errBroken) },
				func(w *sinkWatch) bool { advance(w, 2*time.Second); return w.observe(rns.LogSinkWriting, errBroken) },
			},
			want:   true,
			reason: true,
		},
		{
			name: "a failure inside the grace period does not stop the client",
			steps: []func(*sinkWatch) bool{
				func(w *sinkWatch) bool { return w.observe(rns.LogSinkOpening, errBroken) },
				func(w *sinkWatch) bool { advance(w, time.Second); return w.observe(rns.LogSinkOpening, errBroken) },
			},
		},
		{
			name: "recovery resets the streak",
			steps: []func(*sinkWatch) bool{
				func(w *sinkWatch) bool { return w.observe(rns.LogSinkWriting, errBroken) },
				func(w *sinkWatch) bool { advance(w, 2*time.Second); return w.observe("", nil) },
				func(w *sinkWatch) bool { return w.observe(rns.LogSinkWriting, errBroken) },
			},
		},
		{
			name: "a rotation failure never stops the client",
			steps: []func(*sinkWatch) bool{
				func(w *sinkWatch) bool { return w.observe(rns.LogSinkRotating, errBroken) },
				func(w *sinkWatch) bool { advance(w, time.Minute); return w.observe(rns.LogSinkRotating, errBroken) },
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
			w := newSinkWatch("/tmp/logfile")
			w.now = func() time.Time { return at }
			var stopped bool
			for i, step := range tc.steps {
				if step(w) {
					stopped = true
					if i != len(tc.steps)-1 {
						t.Fatalf("step %v stopped the client early", i)
					}
				}
			}
			if stopped != tc.want {
				t.Fatalf("stopped = %v, want %v", stopped, tc.want)
			}
			reason := w.stopReason()
			if (reason != nil) != tc.reason {
				t.Fatalf("stopReason() = %v, want non-nil = %v", reason, tc.reason)
			}
			if reason != nil && !strings.Contains(reason.Error(), "/tmp/logfile") {
				t.Fatalf("stopReason() = %v, want it to name the log file", reason)
			}
		})
	}
}
