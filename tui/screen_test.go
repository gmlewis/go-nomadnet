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
	"errors"
	"strings"
	"testing"

	"github.com/gmlewis/tcell/v2"
)

// TestScreenProviderSuppliesTheClientsTerminal checks the seam an appliance uses
// to give the client a terminal it built itself.
//
// The desktop path is the absence of a provider, and it must keep working: a
// client with no provider has to leave tview to open its own terminal, or every
// desktop run would draw into a simulation screen and quietly do nothing.
func TestScreenProviderSuppliesTheClientsTerminal(t *testing.T) {
	// Not parallel: the provider is package state, and every other test in this
	// package creates applications.
	t.Cleanup(func() { SetScreenProvider(nil) })

	if screen, err := newScreen(); err != nil || screen != nil {
		t.Fatalf("newScreen() with no provider = %v, %v, want nil, nil", screen, err)
	}

	provided := tcell.NewSimulationScreen("UTF-8")
	SetScreenProvider(func() (tcell.Screen, error) { return provided, nil })

	got, err := newScreen()
	if err != nil {
		t.Fatalf("newScreen() returned %v", err)
	}
	if got != tcell.Screen(provided) {
		t.Errorf("newScreen() = %v, want the registered screen", got)
	}

	// And the application uses it: a client that asked for a terminal and then
	// opened another one would be rendering somewhere nobody can see. tview does
	// not expose the screen it holds, so the screen itself is asked: a simulation
	// screen is 0x0 until it has been initialized, and tview initializes the one
	// it was given.
	app := NewApp(ThemeDark, "unicode", ColorModeTrue)
	if app.screenErr != nil {
		t.Fatalf("NewApp reported %v", app.screenErr)
	}
	width, height := provided.Size()
	if width != 80 || height != 25 {
		t.Errorf("the registered screen is %vx%v, want the 80x25 it has once it is in use", width, height)
	}
}

// TestScreenProviderFailureIsReportedNotSwallowed checks that a terminal the
// appliance could not build stops the client with the reason, instead of letting
// it fall back to a terminal the device does not have.
func TestScreenProviderFailureIsReportedNotSwallowed(t *testing.T) {
	t.Cleanup(func() { SetScreenProvider(nil) })

	want := errors.New("the console socket is already in use")
	SetScreenProvider(func() (tcell.Screen, error) { return nil, want })

	app := NewApp(ThemeDark, "unicode", ColorModeTrue)
	err := app.Run()
	if err == nil {
		t.Fatal("Run returned nil, want the provider's failure")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("Run returned %v, want the provider's own error", err)
	}
}
