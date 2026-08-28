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
	"testing"
)

// TestEmbeddedTerminalStopIsIdempotent pins the stop() race fix: stop is
// called from both Close (the UI goroutine) and the child-exit watcher
// goroutine, so concurrent calls raced a check-then-close into
// "close of closed channel" (a pre-existing flake that panicked the whole
// process under -race). Sixty concurrent stops must all succeed.
func TestEmbeddedTerminalStopIsIdempotent(t *testing.T) {
	t.Parallel()

	et := &EmbeddedTerminal{app: newTestApp(), stopCh: make(chan struct{})}

	var wg sync.WaitGroup
	for range 60 {
		wg.Go(et.stop)
	}
	wg.Wait()

	// The channel is closed exactly once: a receive completes without a panic.
	select {
	case <-et.stopCh:
	default:
		t.Error("stopCh was not closed by stop()")
	}
}
