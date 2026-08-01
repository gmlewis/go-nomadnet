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
	"time"

	"github.com/rivo/tview"
)

// App wraps tview.Application with NomadNet configuration.
type App struct {
	*tview.Application
	Main      *MainDisplay
	Theme     int
	ColorMode int
	Glyphs    GlyphSet
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
		Dialogs:     &DialogManager{},
		Styles:      newStyleRegistry(),
		killRing:    &killRing{},
	}
	a.Styles.Register(theme, colorMode)
	ApplySingleLineBorders()

	a.Main = NewMainDisplay(a, theme, glyphSet)

	return a
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

// SetQuitCallback sets the callback invoked when the user quits.
func (a *App) SetQuitCallback(fn func()) {
	if a.Main != nil {
		a.Main.SetQuitCallback(fn)
	}
}
