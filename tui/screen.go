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

	"github.com/gmlewis/tcell/v2"
)

// ScreenProvider returns the terminal the client draws into.
//
// tcell reaches its terminal two ways: by opening the process's controlling
// terminal, or through the fully exported tcell.Tty interface. A desktop client
// wants the first, and gets it by leaving this unset. An appliance that has a
// terminal but no /dev/tty — the iOS appliance, whose client runs in-process and
// is handed a synthetic tcell.Tty by its own bridge — wants the second, and
// registers a provider here.
//
// The provider is asked once, when the application is created, because a screen
// belongs to an application for its whole life: tview swaps screens only while
// it is running, and this seam exists to avoid ever opening the default one.
type ScreenProvider func() (tcell.Screen, error)

var (
	// screenProviderMu guards the provider. It is read on every application
	// creation and written once at start-up, so it is a mutex rather than an
	// atomic pointer: a provider that is a closure over the caller's state must
	// be published to the goroutine that runs the event loop, and a mutex is the
	// cheapest way to say that out loud.
	screenProviderMu sync.Mutex
	screenProvider   ScreenProvider
)

// SetScreenProvider registers the terminal the client draws into, or clears it
// when p is nil.
//
// It must be called before the application is created. The appliance's host
// binary calls it at start-up, from its own init, which is the only point that
// is guaranteed to run before anything asks for a screen.
func SetScreenProvider(p ScreenProvider) {
	screenProviderMu.Lock()
	screenProvider = p
	screenProviderMu.Unlock()
}

// newScreen returns the registered terminal, or nil when the client should open
// its own.
//
// A provider that fails is not silently ignored: its error is returned and
// reported by App.Run, because an appliance whose terminal could not be built
// must say so rather than fall back to a default terminal that does not exist on
// the device.
func newScreen() (tcell.Screen, error) {
	screenProviderMu.Lock()
	p := screenProvider
	screenProviderMu.Unlock()
	if p == nil {
		return nil, nil
	}
	return p()
}
