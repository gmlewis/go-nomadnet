// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TestSavedNodesUpToMenu verifies the dispatcher's up-at-top→menu transition
// for the Saved Nodes left list (a bare *IndicativeListBox at item 0): the
// app-level input capture's bodyListAtTop must recognize it and move focus to
// the menu. (The Announce Stream variant — a *pileFiller — is covered by
// TestAnnounceStreamUpToMenu, which drives the real event loop because the
// escape is handled by the pile's own InputHandler, not the dispatcher.)
func TestSavedNodesUpToMenu(t *testing.T) {
	app := newTestApp()
	md := NewMainDisplay(app, ThemeDark, GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "aaaa", DisplayName: "Node A"},
		{SourceHash: "bbbb", DisplayName: "Node B"},
	})

	md.SetDisplay("network", nd.Widget())
	md.SelectPage("network")

	// Saved Nodes mode (the default): focusLeftList SetFocus's the bordered
	// list slot, which cascades down to the IndicativeListBox.
	md.focusRegion = "body"
	nd.focusLeftList()

	if !md.bodyListAtTop() {
		t.Fatalf("bodyListAtTop=false for focused IndicativeListBox at item 0; want true")
	}

	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	if res := md.handleInput(up); res != nil {
		t.Errorf("handleInput(Up) returned %v, want nil (consumed)", res)
	}
	if md.focusRegion != "menu" {
		t.Errorf("focusRegion=%q, want menu (Up-at-top of Saved Nodes should reach the menu)", md.focusRegion)
	}
}

// TestAnnounceStreamUpToMenu drives the REAL event loop with the Announce
// Stream focused and verifies that repeated Up escapes to the main menu — the
// Bug 1 regression. The Announce Stream left panel is a pileFiller of
// [tab bar, filter bar, list]; Up must traverse list→filter bar→tab bar and
// then, from the top of the pile (the tab bar), escape to the menu (matching
// urwid's MainFrame: Up at the top of the body moves focus to the header).
// Before the fix, Up dead-ended on the tab bar and never reached the menu.
//
// Each key press is awaited through the pile's own focus index instead of a
// sleep. That makes the wait a barrier AND an assertion: it proves the press
// was handled before the focus region is inspected (a sleep only hoped so, and
// raced the loop's write of focusRegion while doing it), and it makes the
// traversal's intermediate states — the very states Bug 1 broke — observable
// rather than assumed.
func TestAnnounceStreamUpToMenu(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, []NodeEntry{
		{SourceHash: "aaaa", DisplayName: "Node A"},
		{SourceHash: "bbbb", DisplayName: "Node B"},
	})
	app.Main.SetDisplay("network", nd.Widget())
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
	// Enter the Announce Stream, mirroring the user's post-Ctrl-L state.
	// Python's AnnounceStream Pile defaults to index 0, so toggleList lands
	// focus on the tab bar (not the list).
	nd.toggleList()

	app.Main.SetQuitCallback(func() { app.Stop() })
	runErr := make(chan error, 1)
	go func() { runErr <- app.runWithSimScreen() }()

	// The pile is [tab bar(0), filter bar(1), list(2)]. FocusIndex is written by
	// the loop as it dispatches keys and is not lock-protected, so it is read
	// only inside awaitLoop's conditions, which run on the loop goroutine that
	// writes it.
	pile := nd.announceStream.pile
	awaitLoop(t, app, runErr, "the Announce Stream pile to settle on its tab bar", func() bool {
		return pile.FocusIndex() == 0
	})

	// Reach the list the way a user does — two Downs: tab bar → filter bar →
	// list (index 0 → 1 → 2).
	for _, want := range []int{1, 2} {
		screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
		awaitLoop(t, app, runErr, fmt.Sprintf("Down to reach pile index %v", want), func() bool {
			return pile.FocusIndex() == want
		})
	}

	// With focus on the list, three Ups are required: list→filter bar→tab
	// bar→menu. Focus must stay in the body for the first two — reaching the
	// menu early is precisely the Bug 1 failure mode — and only the third Up,
	// taken at the top of the pile, may escape to the menu.
	for i, want := range []int{1, 0} {
		screen.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
		awaitLoop(t, app, runErr, fmt.Sprintf("Up to reach pile index %v", want), func() bool {
			return pile.FocusIndex() == want
		})
		if region := app.Main.FocusRegion(); region != "body" {
			t.Errorf("Up#%v: focusRegion=%q, want body (menu must not be reached before the top of the pile)", i+1, region)
		}
	}
	screen.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
	awaitLoop(t, app, runErr, "the third Up to escape to the menu", func() bool {
		return app.Main.FocusRegion() == "menu"
	})
	t.Logf("Up#3: focus=%T focusRegion=%q", app.GetFocus(), app.Main.FocusRegion())

	// The event loop must still be responsive: Ctrl-Q quits cleanly.
	screen.InjectKey(tcell.KeyCtrlQ, 0, tcell.ModNone)
	select {
	case <-runErr:
	case <-time.After(4 * time.Second):
		app.Stop()
		t.Fatal("event loop did not quit (deadlocked)")
	}
}
