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
	"sync/atomic"
	"testing"
	"time"

	"github.com/gmlewis/tcell/v2"
)

// TestDrawDoesNotBlockOnMainDisplayMu is the regression test for the freeze the
// operator hit on the tablet: the client stopped painting, every later click was
// ignored, and its background goroutines kept running.
//
// The deadlock is a lock-order inversion around MainDisplay.mu, and it needs
// exactly two participants — both of which one mouse click produces:
//
//   - The event loop, inside a draw. tview's Application.draw holds the
//     application's write lock for the WHOLE frame (gmlewis/tview
//     application.go, draw() → a.Lock()), and the frame's DrawFunc calls
//     checkOrientation, which takes MainDisplay.mu. So inside a draw the order is
//     application lock → MainDisplay.mu, and it is a BLOCKING acquisition.
//
//   - A goroutine holding MainDisplay.mu that reaches back into the application.
//     refreshShortcuts cannot take the lock while a page switch holds it (the
//     SetFocusFunc of the newly focused widget fires during SwitchToPage), so it
//     hands the refresh to a goroutine that takes the lock itself and calls
//     updateShortcutsLocked → the page's registered provider. A provider reads
//     live focus (ConversationsDisplay.GetShortcutText → composerHasFocus →
//     Application.GetFocus → a.RLock). Off the event loop the order is therefore
//     MainDisplay.mu → application lock.
//
// Once the loop is blocked on MainDisplay.mu while holding the application lock,
// and the holder is blocked on the application lock while holding
// MainDisplay.mu, neither can proceed and the client never paints again.
//
// It is a race — the click must leave a goroutine waiting on MainDisplay.mu
// before the click's own redraw runs — which is why it strikes often but not
// always, and why it is not platform-specific: a slower console only widens the
// window.
//
// The test fixes the interleaving instead of racing it. The before-draw hook runs
// inside draw() with the application lock already held, so parking there makes
// "the loop is inside a draw" an observation rather than a guess; the provider
// parks the same way while MainDisplay.mu is held. Each holder is released by the
// test, one step at a time.
func TestDrawDoesNotBlockOnMainDisplayMu(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	app.SetRoot()
	// SetScreen re-inits the sim screen (resetting it to 80x25), so SetSize MUST
	// come after SetScreen and before the loop starts drawing.
	app.SetScreen(screen)
	screen.SetSize(135, 32)

	md := app.Main
	// The provider runs only for the displayed page: updateShortcutsLocked
	// consults the active page's callback alone.
	active := md.ActivePage()

	var armed atomic.Bool
	var inProviderOnce, inDrawOnce, focusReadOnce sync.Once
	providerEntered := make(chan struct{})
	inDraw := make(chan struct{})
	focusTaken := make(chan struct{})
	// release lets the provider reach for the application lock (Application.GetFocus).
	// hookGate lets the parked draw run on, past the hook and into the frame
	// DrawFunc, which is where it wants MainDisplay.mu.
	release := make(chan struct{})
	hookGate := make(chan struct{})

	// The provider stands in for ConversationsDisplay.GetShortcutText: it reads
	// live focus, a read lock on the application. While disarmed it returns at
	// once, because registration itself refreshes the bar.
	md.SetShortcutCallback(active, func() string {
		if !armed.Load() {
			return "seed"
		}
		inProviderOnce.Do(func() { close(providerEntered) })
		<-release
		_ = app.GetFocus() // Application.RLock: blocks while a draw holds the lock
		focusReadOnce.Do(func() { close(focusTaken) })
		return "provider"
	})

	// The before-draw hook runs with the application's write lock held and before
	// the frame's DrawFunc, so a signal from it proves the loop is inside a draw.
	// Parking here holds that lock, which is the state the real freeze starts from.
	app.Application.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		if !armed.Load() {
			return false
		}
		inDrawOnce.Do(func() { close(inDraw) })
		<-hookGate
		return false
	})

	runErr := make(chan error, 1)
	go func() { runErr <- app.Application.Run() }()
	// Let the boot frames finish: they run disarmed, and the barrier proves the
	// loop has already serviced everything they queued.
	onEventLoop(app, func() {})
	armed.Store(true)

	// Step 1: park a draw with the application lock held. drawDone reports whether
	// that frame ever finishes — which is the assertion.
	drawDone := make(chan struct{})
	go func() {
		defer close(drawDone)
		app.Application.Draw()
	}()
	select {
	case <-inDraw:
	case <-time.After(3 * time.Second):
		t.Fatal("no draw was in flight: the event loop is not drawing at all")
	}

	// Step 2: hand MainDisplay.mu to the goroutine refreshShortcuts falls back to
	// when the lock is held (the page-switch case, main-display.go), and let it run
	// the page's provider. The provider reads focus while holding the lock.
	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		md.refreshShortcuts()
	}()
	select {
	case <-providerEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("the refresh provider never ran")
	}

	// Step 3: the provider reaches for the application lock, which the parked draw
	// holds, and the draw is released into checkOrientation, which wants
	// MainDisplay.mu.
	close(release)
	close(hookGate)

	// The loop must still be able to finish that frame. With the inversion it
	// cannot: it holds the application lock while waiting on MainDisplay.mu, and
	// the goroutine holding MainDisplay.mu is waiting on the application lock.
	select {
	case <-drawDone:
	case <-time.After(3 * time.Second):
		// Do NOT call Stop here: it takes the application lock, which the wedged
		// draw holds. Failing the test is the point; the parked goroutines die with
		// the test binary.
		t.Fatal("deadlock: the draw blocked on MainDisplay.mu while the refresh goroutine held it and wanted the application lock")
	}
	select {
	case <-focusTaken:
	case <-time.After(3 * time.Second):
		t.Fatal("the refresh goroutine never got the application lock back")
	}
	<-refreshDone

	app.Application.Stop()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("the event loop exited with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the event loop did not stop")
	}
}
