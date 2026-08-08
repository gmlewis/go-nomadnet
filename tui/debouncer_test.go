// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDebouncedCallCoalescesBurst verifies the core fix for the "UI appears
// hung under an announce burst" symptom: a rapid burst of Trigger calls (one
// per incoming announce/message) must collapse into a SINGLE fn invocation
// once the burst subsides, rather than queuing one fn per call. Without
// coalescing, N announces would queue N full UI refreshes (each doing disk
// I/O and a full redraw) and starve the tview event loop's key handling.
func TestDebouncedCallCoalescesBurst(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	d := NewDebouncedCall(20*time.Millisecond, func() { calls.Add(1) })

	// Fire 200 times with no delay between calls, simulating a tight burst of
	// 200 incoming announces all firing UIChangeCallback within the coalesce
	// window.
	for i := 0; i < 200; i++ {
		d.Trigger()
	}

	// Wait long enough for the (single) coalesced fire to run.
	time.Sleep(80 * time.Millisecond)

	if got := calls.Load(); got != 1 {
		t.Errorf("burst of 200 triggers produced %v fn calls, want 1 (coalesced)", got)
	}
}

// TestDebouncedCallFiresAfterWindow verifies a single Trigger runs fn exactly
// once after the window elapses.
func TestDebouncedCallFiresAfterWindow(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	d := NewDebouncedCall(15*time.Millisecond, func() { calls.Add(1) })

	d.Trigger()
	if got := calls.Load(); got != 0 {
		t.Errorf("fn ran before window elapsed: %v calls, want 0", got)
	}
	time.Sleep(60 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("after window: %v calls, want 1", got)
	}
}

// TestDebouncedCallRetriggerAfterFire verifies the timer can be re-armed after
// it has already fired (a second, later burst produces a second fn call).
func TestDebouncedCallRetriggerAfterFire(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	d := NewDebouncedCall(15*time.Millisecond, func() { calls.Add(1) })

	d.Trigger()
	time.Sleep(60 * time.Millisecond) // first fire
	if got := calls.Load(); got != 1 {
		t.Fatalf("first fire: %v calls, want 1", got)
	}

	// A second burst well after the first fire must re-arm the same timer and
	// produce exactly one more call.
	for i := 0; i < 50; i++ {
		d.Trigger()
	}
	time.Sleep(60 * time.Millisecond)
	if got := calls.Load(); got != 2 {
		t.Errorf("after retrigger: %v total calls, want 2", got)
	}
}

// TestDebouncedCallConcurrentTriggers verifies coalescing holds when Trigger
// is called concurrently from many goroutines (as it is from transport
// goroutines processing announces in parallel). The fn call count must be
// bounded by the number of distinct window boundaries crossed, NOT by the
// number of Trigger calls.
func TestDebouncedCallConcurrentTriggers(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	d := NewDebouncedCall(20*time.Millisecond, func() { calls.Add(1) })

	var wg sync.WaitGroup
	n := 500
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			d.Trigger()
		}()
	}
	wg.Wait()
	time.Sleep(80 * time.Millisecond)

	if got := calls.Load(); got != 1 {
		t.Errorf("concurrent burst of %v triggers produced %v fn calls, want 1", n, got)
	}
}
