// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// screenText flattens a SimulationScreen's current cell content into a string,
// so a test can assert what is actually painted on screen.
func screenText(screen tcell.SimulationScreen) string {
	cells, _, _ := screen.GetContents()
	var b strings.Builder
	for _, c := range cells {
		if len(c.Runes) > 0 {
			b.WriteRune(c.Runes[0])
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// runWithSimScreen runs the app's event loop the way Application.Run does, but
// against the already-installed SimulationScreen (avoids tview creating a real
// screen).
func (a *App) runWithSimScreen() error {
	return a.Application.Run()
}

// dispatchAndDraw mirrors Application.Run's key dispatch: app input capture
// first, then (if returned) the root InputHandler. Used by tests that need the
// full key-dispatch path without running the real event loop.
func dispatchAndDraw(t *testing.T, app *App, ev *tcell.EventKey) {
	t.Helper()
	got := app.Main.handleInput(ev)
	if got != nil {
		root := app.Main.Root()
		if h := root.InputHandler(); h != nil {
			h(got, func(p tview.Primitive) { app.SetFocus(p) })
		}
	}
}

// TestMenuClickRedrawsPage is the Bug 2 regression: clicking a menu item must
// repaint the screen so the new page is visible immediately. Before the fix the
// menuBar's SetMouseCapture returned 0 (MouseMove) instead of tview.MouseConsumed,
// so tview did not redraw after the click — the page switched internally but the
// screen kept showing the old page until an unrelated async redraw fired, making
// the app appear frozen/unresponsive after a menu click.
//
// Every wait is a loop barrier (awaitLoop), never a sleep. A sleep orders
// nothing, so the read of activePage it preceded was a data race against the
// loop's own write no matter how long the sleep was, and it also left the
// assertion racing the click it was meant to wait for.
func TestMenuClickRedrawsPage(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	app.EnableMouse(true)

	nd := NewNetworkDisplay(app, nil, []NodeEntry{{SourceHash: "aaaa", DisplayName: "Node A"}})
	app.Main.SetDisplay("network", nd.Widget())
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	// SetScreen re-inits the sim screen (resetting it to 80x25), so SetSize
	// MUST come after SetScreen for the menuBar to be laid out full-width.
	app.SetScreen(screen)
	screen.SetSize(135, 32)

	app.Main.SelectPage("network")

	app.Main.SetQuitCallback(func() { app.Stop() })
	runErr := make(chan error, 1)
	go func() { runErr <- app.runWithSimScreen() }()

	// Before the click the screen must show the Network page (e.g. its "Saved
	// Nodes" list title), proving the harness actually paints the active page.
	// screenText walks the simulation screen's live cell buffer, so it is called
	// from inside awaitLoop's predicate — on the loop goroutine, the only
	// goroutine that draws — rather than from here.
	awaitLoop(t, app, runErr, "the Network page to be painted", func() bool {
		return containsSubstr(screenText(screen), "Saved Nodes")
	})

	// Compute the Guide button's x within the full-width menu bar. menuItems and
	// menuWidths are written by redrawMenuBar under md.mu, so they are read under
	// the same lock here.
	app.Main.mu.Lock()
	guideIdx := -1
	for i, it := range app.Main.menuItems {
		if it.Key == "guide" {
			guideIdx = i
		}
	}
	x := 2
	for i := 0; i < guideIdx && i < len(app.Main.menuWidths); i++ {
		x += app.Main.menuWidths[i]
	}
	x += 3
	app.Main.mu.Unlock()

	// Click Guide (down + up → a completed click) and wait for the loop to act
	// on it. The loop runs a whole mouse case — handleClick, and the redraw that
	// case triggers — before it can service the barrier, so a predicate that
	// sees activePage == "guide" is looking at a screen the click has already
	// repainted. The screen text is copied there, on the loop goroutine, and
	// published to this goroutine by the barrier.
	var afterClick string
	screen.InjectMouse(x, 0, tcell.ButtonPrimary, tcell.ModNone)
	screen.InjectMouse(x, 0, tcell.ButtonNone, tcell.ModNone)
	awaitLoop(t, app, runErr, "the Guide click to switch the page", func() bool {
		if app.Main.ActivePage() != "guide" {
			return false
		}
		afterClick = screenText(screen)
		return true
	})

	// The fix: the click must have REDRAWN, so the screen now shows the Guide
	// page (its "Topics" pane title / "Introduction" topic), not the Network
	// page. Before the fix no redraw fired and "Topics" was absent. The wait
	// above deliberately does not force a repaint — forcing one would make this
	// assertion vacuously true.
	if !containsSubstr(afterClick, "Topics") {
		t.Errorf("after click: screen was not redrawn to the Guide page (no 'Topics'); the menu click did not trigger a redraw:\n%v", afterClick)
	}

	// The event loop must still be responsive: Ctrl-Q quits cleanly.
	screen.InjectKey(tcell.KeyCtrlQ, 0, tcell.ModNone)
	select {
	case <-runErr:
	case <-time.After(4 * time.Second):
		app.Stop()
		t.Fatal("event loop did not quit (deadlocked)")
	}
}

// TestGuideEnterDoesNotDeadlock pins that activating the Guide page and pressing
// Enter on the Introduction topic completes without deadlocking — the
// historical SetChangedFunc↔SetCurrentItem recursion regression. It drives the
// full key dispatch + Draw path with a timeout.
func TestGuideEnterDoesNotDeadlock(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{{SourceHash: "aaaa", DisplayName: "Node A"}})
	app.Main.SetDisplay("network", nd.Widget())
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(135, 32)
	app.SetRoot()
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.Root().Draw(screen)

	// Body focus starts on the network page (like the user's pre-click state).
	app.Main.SelectPage("network")
	app.Main.Root().SetRect(0, 0, 135, 32)
	app.Main.Root().Draw(screen)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Switch to Guide (programmatic equivalent of the menu click) and
		// focus the topic list with selection at Introduction (item 0).
		app.Main.SelectPage("guide")
		gd.FocusTopics()
		app.Main.focusRegion = "body"
		app.Main.Root().SetRect(0, 0, 135, 32)
		app.Main.Root().Draw(screen)

		// Press Enter on Introduction.
		enter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
		dispatchAndDraw(t, app, enter)
		app.Main.Root().SetRect(0, 0, 135, 32)
		app.Main.Root().Draw(screen)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Guide Enter + Draw deadlocked (timed out after 5s)")
	}
}
