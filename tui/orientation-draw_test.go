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

	"github.com/gmlewis/tcell/v2"
)

// recordingController is a SidePanelController that records every orientation it
// is told, and touches nothing else. It stands in for the displays the real
// client registers, so a test can assert when the display tree is told about a
// rotation — and, more to the point, when it must not be.
type recordingController struct {
	mu    sync.Mutex
	calls []bool
}

func (r *recordingController) SetPortrait(portrait bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, portrait)
}

func (r *recordingController) orientations() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.calls...)
}

// TestOrientationIsNotAppliedDuringTheDraw pins the invariant: a draw may
// RECORD the terminal's orientation, but it must not tell the displays about it.
//
// A display's SetPortrait changes the tree, and the Guide's also moves focus. The
// frame's DrawFunc runs inside tview's Application.draw, which holds the
// application's write lock for the whole frame, and Application.SetFocus takes
// that same non-reentrant lock — so a focus move from a draw does not fail, it
// blocks the drawing goroutine on itself, forever. The console then stops
// painting and stops answering keys.
//
// It does not run in parallel: it installs the host's pixels probe for the
// package while it runs.
func TestOrientationIsNotAppliedDuringTheDraw(t *testing.T) {
	SetTerminalPixels(func() (int, int, bool) { return 1200, 1920, true })
	t.Cleanup(func() { SetTerminalPixels(nil) })

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	rec := &recordingController{}
	app.Main.RegisterSidePanelController(rec)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()

	// A frame at the tablet's portrait geometry: 100x75 cells on a 1200x1920
	// display, drawn the way tview draws it — the frame's DrawFunc is where the
	// client learns the grid it is being drawn into.
	root := app.Main.Root()
	root.SetRect(0, 0, 100, 75)
	root.Draw(screen)

	if got := rec.orientations(); len(got) != 0 {
		t.Errorf("the draw told the side-panel controllers %v; a draw must not apply the orientation", got)
	}

	app.Main.mu.Lock()
	detected, haveOrientation := app.Main.isPortrait, app.Main.hasOrientation
	app.Main.mu.Unlock()
	if !haveOrientation || !detected {
		t.Error("the draw did not record the portrait orientation it was given")
	}
}

// TestGuideRotationToPortraitInALiveClientDoesNotWedge drives a running client:
// the Guide page in landscape with its topic list focused (the state a normal
// launch leaves it in), then a rotation to portrait.
//
// A display's SetPortrait rebuilds its columns and may move focus (the Guide's
// does, when the topic list it is collapsing holds it). If the rotation were
// applied during a draw, the draw would tell the Guide, the Guide would set
// focus, and the focus call would block on the lock the draw was holding.
// Every assertion here is a barrier on the event loop, so a client that has
// stopped running its loop fails the test by timing out instead of hanging the
// test binary.
//
// It does not run in parallel: it installs the host's pixels probe for the
// package while it runs.
func TestGuideRotationToPortraitInALiveClientDoesNotWedge(t *testing.T) {
	// The display turns with the console, as the appliance's does: held in
	// landscape it is 1920x1080 pixels, stood up it is 1200x1920.
	pixelWidth, pixelHeight := 1920, 1080
	SetTerminalPixels(func() (int, int, bool) { return pixelWidth, pixelHeight, true })
	t.Cleanup(func() { SetTerminalPixels(nil) })

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	guide := NewGuideDisplay(app)
	// The page and the side-panel controller are two registrations, as they are
	// in the client (cmd/gonomadnet/textui.go): the page shows, the display is the
	// one told about a rotation.
	app.Main.SetDisplay("guide", guide.Widget())
	app.Main.RegisterSidePanelController(guide)
	app.SetRoot()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	// SetScreen re-inits the simulation screen, so the size is set after it.
	app.SetScreen(screen)
	screen.SetSize(160, 45) // the tablet in landscape: 160x45 cells of 12x24 pixels
	app.Main.SelectPage("guide")

	app.Main.SetQuitCallback(func() { app.Stop() })
	runErr := make(chan error, 1)
	go func() { runErr <- app.runWithSimScreen() }()

	awaitLoop(t, app, runErr, "the Guide page to be painted in landscape with its topic list focused", func() bool {
		return guide.topics.HasFocus()
	})
	if !guide.TopicsVisible() {
		t.Fatal("the topic list is collapsed in landscape")
	}

	// Rotate to portrait: the display turns with it, so the console is now 100x75
	// cells on a 1200x1920 display — and only the physical size says that grid is
	// standing up.
	pixelWidth, pixelHeight = 1200, 1920
	screen.SetSize(100, 75)
	app.QueueUpdateDraw(func() {}) // the next frame, at the new size

	awaitLoop(t, app, runErr, "the Guide's topic list to collapse in portrait", func() bool {
		return !guide.TopicsVisible()
	})
	awaitLoop(t, app, runErr, "focus to leave the collapsed topic list", func() bool {
		return !guide.topics.HasFocus()
	})
}
