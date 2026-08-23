// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a deterministic, manually-advanced clock for debouncer tests.
// It records scheduled timers and fires them when advance moves the clock to
// or past their deadline. Because tests drive time explicitly, they never wait
// on the real scheduler and cannot flake under load.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Duration
	timers []*fakeTimer
}

// fakeTimer is a timer scheduled on a fakeClock.
type fakeTimer struct {
	clock    *fakeClock
	deadline time.Duration
	fn       func()
	inList   bool // whether the timer is currently in clock.timers
	stopped  bool
}

func (c *fakeClock) AfterFunc(d time.Duration, fn func()) timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, deadline: c.now + d, fn: fn, inList: true}
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	active := t.inList && !t.stopped && t.deadline > t.clock.now
	t.deadline = t.clock.now + d
	t.stopped = false
	if !t.inList {
		t.clock.timers = append(t.clock.timers, t)
		t.inList = true
	}
	return active
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	active := t.inList && !t.stopped && t.deadline > t.clock.now
	t.stopped = true
	return active
}

// advance moves the clock forward by d and fires every non-stopped timer whose
// deadline has been reached, in deadline order. fired timers are removed from
// the clock. fn callbacks run outside the clock mutex so a callback that
// re-arms a timer (via Trigger) does not deadlock.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now += d
	// Collect due timers (deadline <= now) and drop stopped ones, in place.
	var due []*fakeTimer
	remaining := c.timers[:0]
	for _, t := range c.timers {
		if t.stopped {
			t.inList = false
			continue
		}
		if t.deadline <= c.now {
			t.inList = false
			due = append(due, t)
		} else {
			remaining = append(remaining, t)
		}
	}
	c.timers = remaining
	c.mu.Unlock()
	// Fire by deadline order for determinism.
	sortDueByDeadline(due)
	for _, t := range due {
		t.fn()
	}
}

func sortDueByDeadline(due []*fakeTimer) {
	for i := 1; i < len(due); i++ {
		for j := i; j > 0 && due[j].deadline < due[j-1].deadline; j-- {
			due[j], due[j-1] = due[j-1], due[j]
		}
	}
}

// TestDebouncedCallCoalescesBurst verifies the core fix for the "UI appears
// hung under an announce burst" symptom: a rapid burst of Trigger calls (one
// per incoming announce/message) must collapse into a SINGLE fn invocation
// once the burst subsides, rather than queuing one fn per call. Without
// coalescing, N announces would queue N full UI refreshes (each doing disk
// I/O and a full redraw) and starve the tview event loop's key handling.
func TestDebouncedCallCoalescesBurst(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	fc := &fakeClock{}
	d := newDebouncedCallWithClock(20*time.Millisecond, func() { calls.Add(1) }, fc)

	// Fire 200 times with no delay between calls, simulating a tight burst of
	// 200 incoming announces all firing UIChangeCallback within the coalesce
	// window.
	for range 200 {
		d.Trigger()
	}

	// Before the window elapses, nothing has fired.
	if got := calls.Load(); got != 0 {
		t.Fatalf("before window: %v calls, want 0", got)
	}

	// Advance past the window: the 200 triggers must collapse to one fire.
	fc.advance(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("burst of 200 triggers produced %v fn calls, want 1 (coalesced)", got)
	}
}

// TestDebouncedCallFiresAfterWindow verifies a single Trigger runs fn exactly
// once after the window elapses.
func TestDebouncedCallFiresAfterWindow(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	fc := &fakeClock{}
	d := newDebouncedCallWithClock(15*time.Millisecond, func() { calls.Add(1) }, fc)

	d.Trigger()
	// Short of the window, fn must not have run.
	fc.advance(14 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("before window elapsed: %v calls, want 0", got)
	}
	// At the window, fn fires exactly once.
	fc.advance(1 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("after window: %v calls, want 1", got)
	}
}

// TestDebouncedCallRetriggerAfterFire verifies the timer can be re-armed after
// it has already fired (a second, later burst produces a second fn call).
func TestDebouncedCallRetriggerAfterFire(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	fc := &fakeClock{}
	d := newDebouncedCallWithClock(15*time.Millisecond, func() { calls.Add(1) }, fc)

	d.Trigger()
	fc.advance(15 * time.Millisecond) // first fire
	if got := calls.Load(); got != 1 {
		t.Fatalf("first fire: %v calls, want 1", got)
	}

	// A second burst well after the first fire must re-arm the same timer and
	// produce exactly one more call.
	for range 50 {
		d.Trigger()
	}
	fc.advance(15 * time.Millisecond) // second fire
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
	fc := &fakeClock{}
	d := newDebouncedCallWithClock(20*time.Millisecond, func() { calls.Add(1) }, fc)

	var wg sync.WaitGroup
	n := 500
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			d.Trigger()
		}()
	}
	wg.Wait()

	// All goroutines coalesced into one pending timer; advancing past the
	// window fires it exactly once.
	fc.advance(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("concurrent burst of %v triggers produced %v fn calls, want 1", n, got)
	}
}
