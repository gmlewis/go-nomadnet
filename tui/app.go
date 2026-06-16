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
	"github.com/rivo/tview"
)

// App wraps tview.Application with NomadNet configuration.
type App struct {
	*tview.Application
	Main   *MainDisplay
	Theme  int
	Glyphs GlyphSet
}

// NewApp creates a new tview Application with the given theme and glyph set.
func NewApp(theme int, glyphSet string) *App {
	tviewApp := tview.NewApplication()
	tviewApp.EnableMouse(true)
	glyphs := GetGlyphSet(glyphSet)
	if glyphs == nil {
		glyphs = glyphsUnicode
	}

	RegisterThemeStyles(tviewApp, theme)

	a := &App{
		Application: tviewApp,
		Theme:       theme,
		Glyphs:      glyphs,
	}

	a.Main = NewMainDisplay(tviewApp, theme, glyphSet)

	return a
}

// SetRoot sets the root primitive for the application.
func (a *App) SetRoot() {
	a.Application.SetRoot(a.Main.Root(), true)
}

// Run starts the tview application event loop.
func (a *App) Run() error {
	return a.Application.Run()
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
