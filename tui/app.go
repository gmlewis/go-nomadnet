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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// applyDefaultStylesOnce guards the write to tview's library-global tview.Styles
// (mirroring applyBordersOnce): the values are constants, so applying them
// more than once is a no-op, and the sync.Once makes ApplyDefaultStyles
// idempotent and safe to call from concurrent goroutines (e.g. parallel tests
// each calling NewApp). TestMain fires it once before any parallel test runs.
var applyDefaultStylesOnce sync.Once

// App wraps tview.Application with NomadNet configuration.
type App struct {
	*tview.Application
	Main      *MainDisplay
	Theme     int
	ColorMode int
	Glyphs    GlyphSet
	GlyphSet  string
	Dialogs   *DialogManager
	Styles    *StyleRegistry
	killRing  *killRing
}

// NewApp creates a new tview Application with the given theme, color depth,
// and glyph set. colorMode selects the palette variant (mono/16/256/true);
// pass ColorModeTrue for the shipped 24-bit default.
func NewApp(theme int, glyphSet string, colorMode int) *App {
	tviewApp := tview.NewApplication()
	tviewApp.EnableMouse(true)
	glyphs := GetGlyphSet(glyphSet)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	a := &App{
		Application: tviewApp,
		Theme:       theme,
		ColorMode:   colorMode,
		Glyphs:      glyphs,
		GlyphSet:    glyphSet,
		Dialogs:     &DialogManager{},
		Styles:      newStyleRegistry(),
		killRing:    &killRing{},
	}
	a.Styles.Register(theme, colorMode)
	ApplySingleLineBorders()
	ApplyDefaultStyles()

	a.Main = NewMainDisplay(a, theme, glyphSet)

	return a
}

// ApplyDefaultStyles overrides tview's library-global Styles so that pane
// backgrounds, border cells, border titles, and graphics use the TERMINAL
// DEFAULT color (transparent), matching the Python original. The Python
// urwid LineBox and pane backgrounds are emitted with `\x1b[0;39;49m`
// (default fg, default bg) — never a forced black background or white border.
//
// tview's stock Styles default to `PrimitiveBackgroundColor=ColorBlack` and
// `BorderColor=TitleColor=GraphicsColor=ColorWhite`, which box.go combines into
// white-on-black border cells (`\x1b[38;2;255;255;255;48;2;0;0;0`). On a light
// terminal that is a hard black box where Python is transparent.
//
// Idempotent and safe to call repeatedly: this only ever assigns the constant
// ColorDefault to these four fields. Widgets that need a specific fill (the
// menubar/shortcutbar at `menubar_bg`, msg headers, etc.) set their own
// SetBackgroundColor explicitly and are unaffected.
func ApplyDefaultStyles() {
	applyDefaultStylesOnce.Do(func() {
		tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
		tview.Styles.BorderColor = tcell.ColorDefault
		tview.Styles.TitleColor = tcell.ColorDefault
		tview.Styles.GraphicsColor = tcell.ColorDefault
	})
}

// SetRoot sets the root primitive for the application. The main display is
// mounted as the bottom page of a tview.Pages root owned by the dialog
// manager, so modal dialogs can overlay it without destroying the underlying
// screen.
func (a *App) SetRoot() {
	root := a.Dialogs.Init(a.Application, a.Main.Root())
	a.Application.SetRoot(root, true)
}

// Run starts the tview application event loop.
func (a *App) Run() error {
	return a.Application.Run()
}

// ShowIntro displays the intro/splash widget as the root for the given number
// of seconds, then swaps to the main display root (SetRoot), matching Python's
// TextUI.py:223-232 (intro_display shown for intro_time, then display_main).
// A non-positive seconds shows the main display immediately.
func (a *App) ShowIntro(intro tview.Primitive, seconds float64) {
	if seconds <= 0 {
		a.SetRoot()
		return
	}
	a.Application.SetRoot(intro, true)
	duration := time.Duration(seconds * float64(time.Second))
	time.AfterFunc(duration, func() {
		a.QueueUpdateDraw(func() {
			a.SetRoot()
		})
	})
}

// Stop stops the tview application event loop.
func (a *App) Stop() {
	a.Application.Stop()
}

// QueueUpdateDraw queues f to be executed on the application event loop
// non-blockingly, avoiding deadlocks when invoked from the main thread or during shutdown.
func (a *App) QueueUpdateDraw(f func()) {
	if a == nil || a.Application == nil || f == nil {
		return
	}
	go a.Application.QueueUpdateDraw(f)
}

// SetQuitCallback sets the callback invoked when the user quits.
func (a *App) SetQuitCallback(fn func()) {
	if a.Main != nil {
		a.Main.SetQuitCallback(fn)
	}
}
