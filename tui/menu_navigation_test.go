// Copyright 2026 Glenn Lewis. All rights reserved.

package tui

import (
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
	// Enter the Announce Stream and focus its left list (the pileFiller),
	// mirroring the user's post-connect state. focusLeftList SetFocus's the
	// bordered list slot, which cascades down to the pile's list item.
	nd.toggleList()
	nd.focusLeftList()

	app.Main.SetQuitCallback(func() { app.Stop() })
	runErr := make(chan error, 1)
	go func() { runErr <- app.runWithSimScreen() }()
	time.Sleep(100 * time.Millisecond)

	// The pile is [tab bar(0), filter bar(1), list(2)] with focus on the list,
	// so three Ups are required: list→filter bar→tab bar→menu. Focus must stay
	// in the body for the first two and only reach the menu on the third.
	for i := 1; i <= 3; i++ {
		screen.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
		time.Sleep(100 * time.Millisecond)
		region := app.Main.focusRegion
		t.Logf("Up#%d: focus=%T focusRegion=%q", i, app.GetFocus(), region)
		switch i {
		case 1, 2:
			if region != "body" {
				t.Errorf("Up#%d: focusRegion=%q, want body (menu must not be reached before the top of the pile)", i, region)
			}
		case 3:
			if region != "menu" {
				t.Errorf("Up#%d: focusRegion=%q, want menu (Up at the top of the Announce Stream pile must escape to the menu)", i, region)
			}
		}
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
