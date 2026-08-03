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

	"github.com/rivo/tview"
)

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
