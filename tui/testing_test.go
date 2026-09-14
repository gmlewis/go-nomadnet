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
	"os"
	"testing"
	"time"

	"github.com/rivo/tview"
)

// onEventLoop runs f on the application event loop's own goroutine and returns
// only after the loop has executed it.
//
// This calls tview's Application.QueueUpdate directly rather than the App-level
// QueueUpdateDraw override: that override is deliberately fire-and-forget (it
// hands f to a drainer goroutine and drops it when the queue is full), so it
// orders nothing and is useless as a barrier. tview's QueueUpdate, by contrast,
// hands f to the loop and blocks until the loop has run it, which makes the
// hand-off a real happens-before edge.
//
// The event loop must be running, so callers pair this with a bound on the wait
// rather than calling it blind.
func onEventLoop(app *App, f func()) {
	app.Application.QueueUpdate(f)
}

// awaitLoop waits until cond holds, re-evaluating cond on the event loop's own
// goroutine, and fails the test if the loop exits first or the deadline passes.
//
// cond runs ON the event loop goroutine rather than on the test goroutine, and
// that is the whole point. The loop goroutine is the only writer of UI state, so
// cond may read any of it — widget fields such as pileFiller.focusIndex, the
// screen's live cell buffer — with no locking at all, and it can never observe a
// half-applied event.
//
// Running cond on the test goroutine instead would not be enough, even after a
// barrier: a barrier only orders that goroutine against writes the loop has
// ALREADY made, so the read still races with whatever write the loop is making
// at that very instant. That is precisely how the two tests below used to race.
//
// cond must not itself queue work (no onEventLoop, no QueueUpdateDraw): the loop
// cannot service an update while it is running one. Anything cond leaves behind
// for the test goroutine is published by the barrier's channel hand-off, so it
// is safe to read once awaitLoop returns.
//
// The loop services its update queue and its screen-event queue from a single
// select, so one barrier may be served ahead of an event injected just before
// it; re-evaluating settles that within a couple of iterations. The deadline
// exists to fail a genuine regression with a useful message — it is not a
// timeout to tune, and nothing sleeps through it. On the failure paths the
// helper's goroutine may be left parked on a loop that has gone away; that only
// happens to a test that is already failing.
func awaitLoop(t *testing.T, app *App, runErr <-chan error, what string, cond func() bool) {
	t.Helper()
	const deadline = 5 * time.Second
	settled := make(chan struct{})
	go func() {
		defer close(settled)
		for {
			var ok bool
			onEventLoop(app, func() { ok = cond() })
			if ok {
				return
			}
		}
	}()
	select {
	case <-settled:
	case err := <-runErr:
		t.Fatalf("event loop exited before %v (err=%v)", what, err)
	case <-time.After(deadline):
		t.Fatalf("timed out after %v waiting for %v", deadline, what)
	}
}

// newTestApp returns an *App wired up with an isolated DialogManager,
// StyleRegistry, and kill ring, so parallel tests never share mutable state.
// It does not build a MainDisplay (call NewApp if a test needs the full
// display tree); the fields set here are the ones display constructors and
// dialog/readline helpers reach for.
func newTestApp() *App {
	a := &App{
		Application: tview.NewApplication(),
		Dialogs:     &DialogManager{},
		Styles:      newStyleRegistry(),
		killRing:    &killRing{},
	}
	a.Styles.Register(ThemeDark, ColorModeTrue)
	// Initialize the dialog manager with a stand-in main primitive so display
	// methods that call app.Dialogs.ShowDialog exercise the real overlay path
	// (rather than the nil-app fallback). Tests that need a specific main
	// (e.g. resize tests) re-Init with their own primitive.
	a.Dialogs.Init(a.Application, tview.NewBox())
	return a
}

// TestMain applies the single-line border override once, single-threaded,
// before any parallel test goroutine starts. tview.Borders is a library-global
// that tview reads during Draw; applying it here (idempotently via sync.Once in
// ApplySingleLineBorders) means no test ever writes tview.Borders during the
// parallel phase, eliminating data races between NewApp's
// ApplySingleLineBorders calls and tview's Draw reads of the global.
func TestMain(m *testing.M) {
	ApplySingleLineBorders()
	ApplyDefaultStyles()
	os.Exit(m.Run())
}
